package cmd

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/spf13/cobra"

	"github.com/gopherust-io/goalign/internal/analyzer"
	"github.com/gopherust-io/goalign/internal/bytesconv"
	"github.com/gopherust-io/goalign/internal/config"
	"github.com/gopherust-io/goalign/internal/layout"
	"github.com/gopherust-io/goalign/internal/pkgscan"
)

const maxAnalyzeWorkers = 8

var (
	recursive      bool
	exclude        []string
	arch           string
	minWaste       int
	jobs           int
	policyName     string
	usePackages    bool
	skipGenerated  bool
	ignoreGlobs    []string
	generatedGlobs []string
	arches         []string
	cacheguard     bool
	cacheLineSize  int

	// Shared with analyze/fix; declared here so applyConfig can set them.
	// failOnFindings is also bound in analyze.go; rewriteBools in fix.go.
)

// Default directory names skipped during recursive walks (in addition to -e).
var defaultSkipDirs = map[string]struct{}{
	"vendor":       {},
	".git":         {},
	"node_modules": {},
	"bin":          {},
}

var defaultGeneratedGlobs = []string{"*.pb.go", "*.gen.go"}

func addScanFlags(cmd *cobra.Command) {
	cmd.Flags().BoolVarP(&recursive, "recursive", "r", false, "analyze files recursively")
	cmd.Flags().StringSliceVarP(&exclude, "exclude", "e", []string{}, "exclude path substrings (e.g., vendor/,testdata/)")
	cmd.Flags().StringVar(&arch, "arch", "", "target GOARCH (amd64, arm64, 386, arm, mips, riscv64, wasm, …); default: host")
	cmd.Flags().IntVar(&minWaste, "min-waste", 0, "only report/fix issues with wasted bytes >= N")
	cmd.Flags().IntVarP(&jobs, "jobs", "j", 0, "max parallel file analyzes (default: min(GOMAXPROCS, 8))")
	cmd.Flags().StringVar(&policyName, "policy", "atomics", "suggest policy: atomics, density, stable")
	cmd.Flags().BoolVar(&usePackages, "packages", false, "resolve imported type sizes via go/packages (slower, more accurate)")
	cmd.Flags().BoolVar(&skipGenerated, "skip-generated", true, "skip files with Code generated DO NOT EDIT markers")
	cmd.Flags().StringSliceVar(&ignoreGlobs, "ignore", nil, "ignore path globs (e.g. '**/*.pb.go')")
	cmd.Flags().StringSliceVar(&generatedGlobs, "generated", nil, "extra generated-file globs (with --skip-generated)")
	cmd.Flags().StringSliceVar(&arches, "arches", nil, "multi-arch report (e.g. amd64,arm64,386); analyze only")
	cmd.Flags().BoolVar(&cacheguard, "cacheguard", false, "suggest/fix cache-line pads to isolate contended fields (false-share)")
	cmd.Flags().IntVar(&cacheLineSize, "cache-line", layout.DefaultCacheLine, "cache line size in bytes for Cacheguard (default 64)")
}

func applyConfig(cmd *cobra.Command, path string) error {
	cfg, found, err := config.Load(path)
	if err != nil {
		return err
	}
	if found == "" {
		return nil
	}
	if verbose {
		fmt.Printf("Loaded config %s\n", found)
	}
	flag := func(name string) bool {
		if cmd == nil {
			return false
		}
		f := cmd.Flags().Lookup(name)
		if f == nil {
			f = cmd.PersistentFlags().Lookup(name)
		}
		return f != nil && f.Changed
	}
	if !flag("arch") && cfg.Arch != "" {
		arch = cfg.Arch
	}
	if !flag("min-waste") && cfg.MinWaste != nil {
		minWaste = *cfg.MinWaste
	}
	if !flag("exclude") && len(cfg.Exclude) > 0 {
		exclude = append([]string{}, cfg.Exclude...)
	}
	if !flag("jobs") && cfg.Jobs != nil {
		jobs = *cfg.Jobs
	}
	if !flag("format") && cfg.Format != "" {
		format = cfg.Format
	}
	if !flag("recursive") && cfg.Recursive != nil {
		recursive = *cfg.Recursive
	}
	if !flag("policy") && cfg.Policy != "" {
		policyName = cfg.Policy
	}
	if !flag("packages") && cfg.Packages != nil {
		usePackages = *cfg.Packages
	}
	if !flag("skip-generated") && cfg.SkipGenerated != nil {
		skipGenerated = *cfg.SkipGenerated
	}
	if !flag("ignore") && len(cfg.IgnoreGlobs) > 0 {
		ignoreGlobs = append([]string{}, cfg.IgnoreGlobs...)
	}
	if !flag("generated") && len(cfg.GeneratedGlobs) > 0 {
		generatedGlobs = append([]string{}, cfg.GeneratedGlobs...)
	}
	if !flag("arches") && len(cfg.Arches) > 0 {
		arches = append([]string{}, cfg.Arches...)
	}
	if !flag("fail-on-findings") && cfg.FailOnFindings != nil {
		failOnFindings = *cfg.FailOnFindings
	}
	if !flag("rewrite-bools") && cfg.RewriteBools != nil {
		rewriteBools = *cfg.RewriteBools
	}
	if !flag("cacheguard") && cfg.Cacheguard != nil {
		cacheguard = *cfg.Cacheguard
	}
	if !flag("cache-line") && cfg.CacheLine != nil {
		cacheLineSize = *cfg.CacheLine
	}
	return nil
}

func resolvePath(args []string) string {
	if len(args) > 0 {
		return args[0]
	}
	return "."
}

func resolveSizer() (layout.Sizer, error) {
	if bytesconv.IsEmpty(arch) {
		return layout.DefaultSizer(), nil
	}
	if !layout.ValidArch(arch) {
		return layout.Sizer{}, fmt.Errorf("unknown --arch %q", arch)
	}
	return layout.SizerFor(arch), nil
}

func resolvePolicy() (layout.Policy, error) {
	return layout.ParsePolicy(policyName)
}

// collectResults scans path and returns filtered analysis results.
// nFiles is 0 when no Go files matched.
func collectResults(cmd *cobra.Command, path string) ([]analyzer.Result, int, int, error) {
	if err := applyConfig(cmd, path); err != nil {
		return nil, 0, 0, err
	}
	return collectResultsConfigured(path)
}

// collectResultsConfigured assumes applyConfig has already run (or defaults apply).
func collectResultsConfigured(path string) ([]analyzer.Result, int, int, error) {
	if _, statErr := os.Stat(path); os.IsNotExist(statErr) {
		return nil, 0, 0, fmt.Errorf("path '%s' does not exist", path)
	}

	if len(arches) > 0 {
		return collectMultiArch(path)
	}

	sizer, err := resolveSizer()
	if err != nil {
		return nil, 0, 0, err
	}
	policy, err := resolvePolicy()
	if err != nil {
		return nil, 0, 0, err
	}

	goFiles, err := findGoFiles(path, recursive)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("finding Go files: %w", err)
	}
	if len(goFiles) == 0 {
		return nil, 0, 0, nil
	}

	if verbose {
		fmt.Printf("Found %d Go files to analyze\n", len(goFiles))
	}

	opts := analyzer.Options{
		Sizer:      sizer,
		Policy:     policy,
		CacheLine:  cacheLineSize,
		Cacheguard: cacheguard,
	}
	if usePackages {
		sizes, err := pkgscan.LoadTypeSizes(packagePatterns(path), arch)
		if err != nil {
			return nil, 0, 0, fmt.Errorf("--packages: %w", err)
		}
		opts.TypeSizes = sizes
		if verbose {
			fmt.Printf("Resolved %d types via go/packages\n", len(sizes))
		}
	}

	results, fileErrs := analyzeParallel(goFiles, opts)
	return filterMinWaste(results, minWaste), len(goFiles), fileErrs, nil
}

func collectMultiArch(path string) ([]analyzer.Result, int, int, error) {
	policy, err := resolvePolicy()
	if err != nil {
		return nil, 0, 0, err
	}
	goFiles, err := findGoFiles(path, recursive)
	if err != nil {
		return nil, 0, 0, err
	}
	if len(goFiles) == 0 {
		return nil, 0, 0, nil
	}

	type archRow struct {
		arch   string
		wasted int
		saved  int
		issues int
	}
	rows := make([]archRow, 0, len(arches))
	var merged []analyzer.Result
	fileErrs := 0

	for _, a := range arches {
		a = strings.TrimSpace(a)
		if a == "" {
			continue
		}
		if !layout.ValidArch(a) {
			return nil, 0, 0, fmt.Errorf("unknown --arches entry %q", a)
		}
		opts := analyzer.Options{
			Sizer:      layout.SizerFor(a),
			Policy:     policy,
			CacheLine:  cacheLineSize,
			Cacheguard: cacheguard,
		}
		if usePackages {
			sizes, err := pkgscan.LoadTypeSizes(packagePatterns(path), a)
			if err != nil {
				return nil, 0, 0, fmt.Errorf("--packages/--arches %s: %w", a, err)
			}
			opts.TypeSizes = sizes
			if verbose {
				fmt.Printf("Resolved %d types via go/packages for arch %s\n", len(sizes), a)
			}
		}
		results, errs := analyzeParallel(goFiles, opts)
		results = filterMinWaste(results, minWaste)
		fileErrs += errs
		row := archRow{arch: a}
		for _, r := range results {
			for _, iss := range r.Issues {
				row.wasted += iss.Wasted
				row.saved += iss.Saved
				row.issues++
			}
		}
		rows = append(rows, row)
		// Keep host/first arch details for formatter when single-arch detail wanted.
		if len(merged) == 0 {
			merged = results
		}
	}

	fmt.Println("Multi-arch summary")
	fmt.Println("ARCH     ISSUES  WASTED  SAVABLE")
	for _, r := range rows {
		fmt.Printf("%-8s %6d  %6d  %7d\n", r.arch, r.issues, r.wasted, r.saved)
	}
	return merged, len(goFiles), fileErrs, nil
}

func packagePatterns(path string) []string {
	info, err := os.Stat(path)
	if err == nil && !info.IsDir() {
		return []string{filepath.Dir(path)}
	}
	if recursive {
		return []string{path + "/..."}
	}
	return []string{path}
}

func filterMinWaste(results []analyzer.Result, min int) []analyzer.Result {
	if min <= 0 {
		return results
	}
	out := make([]analyzer.Result, 0, len(results))
	for _, r := range results {
		kept := make([]analyzer.Issue, 0, len(r.Issues))
		for _, iss := range r.Issues {
			if keepDespiteMinWaste(iss, min) {
				kept = append(kept, iss)
			}
		}
		if len(kept) > 0 {
			r.Issues = kept
			out = append(out, r)
		}
	}
	return out
}

// keepDespiteMinWaste reports whether an issue should survive --min-waste.
// False-share / cacheguard findings often have Wasted==0 but still matter.
func keepDespiteMinWaste(iss analyzer.Issue, min int) bool {
	if iss.Wasted >= min {
		return true
	}
	for _, n := range iss.Notes {
		if strings.HasPrefix(n, "false-share") || strings.HasPrefix(n, "cacheguard") {
			return true
		}
	}
	return false
}

func countIssues(results []analyzer.Result) int {
	n := 0
	for _, r := range results {
		n += len(r.Issues)
	}
	return n
}

func analyzeParallel(goFiles []string, opts analyzer.Options) ([]analyzer.Result, int) {
	workers := runtime.GOMAXPROCS(0)
	if workers > maxAnalyzeWorkers {
		workers = maxAnalyzeWorkers
	}
	if jobs > 0 {
		workers = jobs
		if workers > maxAnalyzeWorkers*32 {
			workers = maxAnalyzeWorkers * 32 // hard cap (256 with default maxAnalyzeWorkers)
		}
	}
	if workers < 1 {
		workers = 1
	}
	if workers > len(goFiles) {
		workers = len(goFiles)
	}

	type indexed struct {
		err    error
		result analyzer.Result
		i      int
	}

	jobsCh := make(chan int, workers)
	out := make(chan indexed, workers)

	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobsCh {
				file := goFiles[i]
				if verbose {
					fmt.Printf("Analyzing: %s\n", file)
				}
				result, err := analyzer.AnalyzeFileWithOptions(file, opts)
				out <- indexed{i: i, result: result, err: err}
			}
		}()
	}

	go func() {
		for i := range goFiles {
			jobsCh <- i
		}
		close(jobsCh)
		wg.Wait()
		close(out)
	}()

	byIndex := make([]analyzer.Result, len(goFiles))
	hasIssue := make([]bool, len(goFiles))
	fileErrs := 0
	for item := range out {
		if item.err != nil {
			fmt.Fprintf(os.Stderr, "Error analyzing %s: %v\n", goFiles[item.i], item.err)
			fileErrs++
			continue
		}
		if len(item.result.Issues) > 0 {
			byIndex[item.i] = item.result
			hasIssue[item.i] = true
		}
	}

	results := make([]analyzer.Result, 0)
	for i, ok := range hasIssue {
		if ok {
			results = append(results, byIndex[i])
		}
	}
	return results, fileErrs
}

func findGoFiles(path string, recursive bool) ([]string, error) {
	var files []string
	root, err := filepath.Abs(path)
	if err != nil {
		root = path
	}

	if !recursive {
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			if strings.HasSuffix(path, ".go") {
				if shouldExclude(path) || shouldIgnoreGlob(path) || shouldSkipGeneratedFile(path) {
					return nil, nil
				}
				return []string{path}, nil
			}
			return nil, fmt.Errorf("file is not a Go file")
		}

		entries, err := os.ReadDir(path)
		if err != nil {
			return nil, err
		}

		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			if !strings.HasSuffix(entry.Name(), ".go") {
				continue
			}
			fp := filepath.Join(path, entry.Name())
			if shouldExclude(fp) || shouldIgnoreGlob(fp) || shouldSkipGeneratedFile(fp) {
				continue
			}
			files = append(files, fp)
		}
		return files, nil
	}

	err = filepath.WalkDir(path, func(filePath string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			abs, aerr := filepath.Abs(filePath)
			if aerr != nil {
				abs = filePath
			}
			// Never SkipDir the walk root (e.g. analyze -r vendor/).
			if abs != root {
				name := d.Name()
				if _, skip := defaultSkipDirs[name]; skip {
					return filepath.SkipDir
				}
				if shouldExclude(filePath) || shouldExclude(name+string(filepath.Separator)) {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if !strings.HasSuffix(filePath, ".go") {
			return nil
		}
		if shouldExclude(filePath) || shouldIgnoreGlob(filePath) || shouldSkipGeneratedFile(filePath) {
			return nil
		}
		files = append(files, filePath)
		return nil
	})
	if err != nil {
		return nil, err
	}

	return files, nil
}

func shouldExclude(file string) bool {
	for _, pattern := range exclude {
		if bytesconv.IsEmpty(pattern) {
			continue
		}
		if strings.Contains(file, pattern) {
			return true
		}
	}
	return false
}

func shouldIgnoreGlob(file string) bool {
	for _, g := range ignoreGlobs {
		if analyzer.MatchGlob(g, file) {
			return true
		}
	}
	return false
}

func shouldSkipGeneratedFile(file string) bool {
	globs := generatedGlobs
	if len(globs) == 0 {
		globs = defaultGeneratedGlobs
	}
	for _, g := range globs {
		if analyzer.MatchGlob(g, file) {
			return skipGenerated
		}
	}
	if !skipGenerated {
		return false
	}
	content, err := os.ReadFile(file)
	if err != nil {
		return false
	}
	return analyzer.IsGenerated(content)
}
