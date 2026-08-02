package benchcmp_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/gopherust-io/goalign/internal/analyzer"
	"github.com/gopherust-io/goalign/internal/layout"
)

func corpusRoot(t testing.TB) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// internal/benchcmp -> repo root
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "../.."))
	return filepath.Join(root, "testdata", "benchcorpus")
}

func loadCorpusFiles(t testing.TB, sub string) []string {
	t.Helper()
	dir := filepath.Join(corpusRoot(t), sub)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var files []string
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".go" {
			continue
		}
		files = append(files, filepath.Join(dir, e.Name()))
	}
	if len(files) == 0 {
		t.Fatalf("no .go files in %s", dir)
	}
	return files
}

func TestDensitySuggestSavesOrKeepsSize(t *testing.T) {
	sizer := layout.DefaultSizer()
	for _, path := range loadCorpusFiles(t, "density") {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		res, err := analyzer.AnalyzeSource(path, content, sizer)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		for _, issue := range res.Issues {
			if issue.Saved < 0 {
				t.Errorf("%s %s: negative saved %d", path, issue.StructName, issue.Saved)
			}
			if issue.SuggestedWasted > issue.Wasted {
				t.Errorf("%s %s: suggested waste %d > original %d",
					path, issue.StructName, issue.SuggestedWasted, issue.Wasted)
			}
		}
	}
}

func BenchmarkGoalignAnalyzeCorpus(b *testing.B) {
	sizer := layout.DefaultSizer()
	type file struct {
		name    string
		content []byte
	}
	var files []file
	for _, sub := range []string{"density", "atomics"} {
		for _, path := range loadCorpusFiles(b, sub) {
			content, err := os.ReadFile(path)
			if err != nil {
				b.Fatal(err)
			}
			files = append(files, file{name: path, content: content})
		}
	}

	b.ReportAllocs()
	for b.Loop() {
		for _, f := range files {
			if _, err := analyzer.AnalyzeSource(f.name, f.content, sizer); err != nil {
				b.Fatal(err)
			}
		}
	}
}
