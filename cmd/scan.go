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
	"github.com/gopherust-io/goalign/internal/layout"
)

const maxAnalyzeWorkers = 8

var (
	recursive bool
	exclude   []string
	arch      string
	minWaste  int
	jobs      int
)

// Default directory names skipped during recursive walks (in addition to -e).
var defaultSkipDirs = map[string]struct{}{
	"vendor":       {},
	".git":         {},
	"node_modules": {},
	"bin":          {},
}

func addScanFlags(cmd *cobra.Command) {
	cmd.Flags().BoolVarP(&recursive, "recursive", "r", false, "analyze files recursively")
	cmd.Flags().StringSliceVarP(&exclude, "exclude", "e", []string{}, "exclude path substrings (e.g., vendor/,testdata/)")
	cmd.Flags().StringVar(&arch, "arch", "", "target GOARCH for sizes (amd64, arm64, 386, arm); default: host")
	cmd.Flags().IntVar(&minWaste, "min-waste", 0, "only report/fix issues with wasted bytes >= N")
	cmd.Flags().IntVarP(&jobs, "jobs", "j", 0, "max parallel file analyzes (default: min(GOMAXPROCS, 8))")
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

// collectResults scans path and returns filtered analysis results.
// nFiles is 0 when no Go files matched.
func collectResults(path string) ([]analyzer.Result, int, int, error) {
	if _, statErr := os.Stat(path); os.IsNotExist(statErr) {
		return nil, 0, 0, fmt.Errorf("path '%s' does not exist", path)
	}

	sizer, err := resolveSizer()
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

	results, fileErrs := analyzeParallel(goFiles, sizer)
	return filterMinWaste(results, minWaste), len(goFiles), fileErrs, nil
}

func filterMinWaste(results []analyzer.Result, min int) []analyzer.Result {
	if min <= 0 {
		return results
	}
	out := make([]analyzer.Result, 0, len(results))
	for _, r := range results {
		kept := make([]analyzer.Issue, 0, len(r.Issues))
		for _, iss := range r.Issues {
			if iss.Wasted >= min {
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

func countIssues(results []analyzer.Result) int {
	n := 0
	for _, r := range results {
		n += len(r.Issues)
	}
	return n
}

func analyzeParallel(goFiles []string, sizer layout.Sizer) ([]analyzer.Result, int) {
	workers := runtime.GOMAXPROCS(0)
	if workers > maxAnalyzeWorkers {
		workers = maxAnalyzeWorkers
	}
	if jobs > 0 {
		workers = jobs
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

	jobs := make(chan int, workers)
	out := make(chan indexed, workers)

	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				file := goFiles[i]
				if verbose {
					fmt.Printf("Analyzing: %s\n", file)
				}
				result, err := analyzer.AnalyzeFileWithSizer(file, sizer)
				out <- indexed{i: i, result: result, err: err}
			}
		}()
	}

	go func() {
		for i := range goFiles {
			jobs <- i
		}
		close(jobs)
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
				if shouldExclude(path) {
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
			if shouldExclude(fp) {
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
		if shouldExclude(filePath) {
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
