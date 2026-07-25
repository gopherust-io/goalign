package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/spf13/cobra"

	"github.com/nekruzjm/goalign/internal/analyzer"
	"github.com/nekruzjm/goalign/internal/formatter"
)

var (
	recursive bool
	exclude   []string
)

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
	analyzeCmd.Flags().StringSliceVarP(&exclude, "exclude", "e", []string{}, "exclude patterns (e.g., vendor/,test/)")
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

	goFiles, err := findGoFiles(path, recursive)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error finding Go files: %v\n", err)
		os.Exit(1)
	}

	filtered := goFiles[:0]
	for _, file := range goFiles {
		if !shouldExclude(file) {
			filtered = append(filtered, file)
		}
	}
	goFiles = filtered

	if len(goFiles) == 0 {
		fmt.Println("No Go files found")
		return
	}

	if verbose {
		fmt.Printf("Found %d Go files to analyze\n", len(goFiles))
	}

	results := analyzeParallel(goFiles)

	if err := formatter.Format(os.Stdout, results, format); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Error formatting output: %v\n", err)
		os.Exit(1)
	}
}

func analyzeParallel(goFiles []string) []analyzer.Result {
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
				result, err := analyzer.AnalyzeFile(file)
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
				return []string{path}, nil
			}
			return nil, fmt.Errorf("file is not a Go file")
		}

		entries, err := os.ReadDir(path)
		if err != nil {
			return nil, err
		}

		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".go") {
				files = append(files, filepath.Join(path, entry.Name()))
			}
		}
		return files, nil
	}

	err := filepath.Walk(path, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(filePath, ".go") {
			files = append(files, filePath)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return files, nil
}

func shouldExclude(file string) bool {
	for _, pattern := range exclude {
		if strings.Contains(file, pattern) {
			return true
		}
	}
	return false
}
