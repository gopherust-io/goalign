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

	"github.com/nekruzjm/goalign/internal/analyzer"
	"github.com/nekruzjm/goalign/internal/formatter"
	"github.com/nekruzjm/goalign/internal/layout"
)

var (
	recursive      bool
	exclude        []string
	arch           string
	failOnFindings bool
	minWaste       int
)

// Default directory names skipped during recursive walks (in addition to -e).
var defaultSkipDirs = map[string]struct{}{
	"vendor":       {},
	".git":         {},
	"node_modules": {},
	"bin":          {},
}

var analyzeCmd = &cobra.Command{
	Use:   "analyze [path]",
	Short: "Analyze Go files for struct alignment issues",
	Long: `Analyze Go source files for struct alignment issues.

This command will scan Go files and report struct alignment problems that could
lead to memory waste and performance issues. It also suggests NATS-style field
reordering (atomics first, then density packing).`,
	Args: cobra.MaximumNArgs(1),
	Run:  runAnalyze,
}

func init() {
	rootCmd.AddCommand(analyzeCmd)
	analyzeCmd.Flags().BoolVarP(&recursive, "recursive", "r", false, "analyze files recursively")
	analyzeCmd.Flags().StringSliceVarP(&exclude, "exclude", "e", []string{}, "exclude path substrings (e.g., vendor/,testdata/)")
	analyzeCmd.Flags().StringVar(&arch, "arch", "", "target GOARCH for sizes (amd64, arm64, 386, arm); default: host")
	analyzeCmd.Flags().BoolVar(&failOnFindings, "fail-on-findings", false, "exit 1 if any issue meets --min-waste")
	analyzeCmd.Flags().IntVar(&minWaste, "min-waste", 0, "only report/fail on issues with wasted bytes >= N")
}

func runAnalyze(cmd *cobra.Command, args []string) {
	path := "."
	if len(args) > 0 {
		path = args[0]
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "Error: path '%s' does not exist\n", path)
		os.Exit(1)
	}

	sizer := layout.DefaultSizer()
	if arch != "" {
		sizer = layout.SizerFor(arch)
	}

	goFiles, err := findGoFiles(path, recursive)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error finding Go files: %v\n", err)
		os.Exit(1)
	}

	if len(goFiles) == 0 {
		fmt.Println("No Go files found")
		return
	}

	if verbose {
		fmt.Printf("Found %d Go files to analyze\n", len(goFiles))
	}

	results := analyzeParallel(goFiles, sizer)
	results = filterMinWaste(results, minWaste)

	if err := formatter.Format(os.Stdout, results, format); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Error formatting output: %v\n", err)
		os.Exit(1)
	}

	if failOnFindings && countIssues(results) > 0 {
		os.Exit(1)
	}
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

func analyzeParallel(goFiles []string, sizer layout.Sizer) []analyzer.Result {
	workers := runtime.GOMAXPROCS(0)
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
	for item := range out {
		if item.err != nil {
			fmt.Fprintf(os.Stderr, "Error analyzing %s: %v\n", goFiles[item.i], item.err)
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
	return results
}

func findGoFiles(path string, recursive bool) ([]string, error) {
	var files []string

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

	err := filepath.WalkDir(path, func(filePath string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if _, skip := defaultSkipDirs[name]; skip {
				return filepath.SkipDir
			}
			// Also skip dirs matching user exclude patterns.
			if shouldExclude(filePath) || shouldExclude(name+string(filepath.Separator)) {
				return filepath.SkipDir
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
		if pattern == "" {
			continue
		}
		if strings.Contains(file, pattern) {
			return true
		}
	}
	return false
}
