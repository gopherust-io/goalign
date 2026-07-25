package analyzer_test

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/nekruzjm/goalign/internal/analyzer"
	"github.com/nekruzjm/goalign/internal/layout"
)

func examplesDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("no caller")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "examples")
}

func TestAnalyzeBadAlignment(t *testing.T) {
	path := filepath.Join(examplesDir(t), "bad_alignment.go")
	res, err := analyzer.AnalyzeFileWithSizer(path, layout.SizerFor("amd64"))
	if err != nil {
		t.Fatal(err)
	}

	byName := map[string]analyzer.Issue{}
	for _, iss := range res.Issues {
		byName[iss.StructName] = iss
	}

	if _, ok := byName["IgnoredStruct"]; ok {
		t.Fatal("IgnoredStruct should be skipped")
	}
	bad, ok := byName["BadStruct"]
	if !ok {
		t.Fatalf("BadStruct missing, issues=%v", names(res.Issues))
	}
	if bad.Wasted != 10 {
		t.Fatalf("BadStruct wasted=%d want 10", bad.Wasted)
	}
	if bad.Saved < 8 {
		t.Fatalf("BadStruct saved=%d want >= 8", bad.Saved)
	}
	if len(bad.Suggested) == 0 {
		t.Fatal("expected suggested order")
	}
	// GoodStruct still has 2 bytes trailing padding
	if good, ok := byName["GoodStruct"]; ok {
		if good.Wasted != 2 {
			t.Fatalf("GoodStruct wasted=%d want 2", good.Wasted)
		}
	}
}

func TestAnalyzeComplex(t *testing.T) {
	path := filepath.Join(examplesDir(t), "complex_alignment.go")
	res, err := analyzer.AnalyzeFileWithSizer(path, layout.SizerFor("amd64"))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Issues) < 2 {
		t.Fatalf("expected >=2 issues, got %d", len(res.Issues))
	}
	for _, iss := range res.Issues {
		if iss.Wasted > 0 && iss.Saved == 0 && len(iss.Suggested) > 0 {
			// may be already optimal density-wise
			continue
		}
		if iss.Wasted > 0 && len(iss.Suggested) == 0 {
			t.Fatalf("%s: missing suggested fields", iss.StructName)
		}
	}
}

func TestAnalyzeSourceAtomics(t *testing.T) {
	src := []byte(`package p
type S struct {
	Ok bool
	N int64
}
`)
	res, err := analyzer.AnalyzeSource("x.go", src, layout.SizerFor("amd64"))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Issues) != 1 {
		t.Fatalf("issues=%d want 1", len(res.Issues))
	}
	iss := res.Issues[0]
	if iss.Suggested[0].Name != "N" {
		t.Fatalf("suggested first=%s want N", iss.Suggested[0].Name)
	}
	hasNote := false
	for _, n := range iss.Notes {
		if len(n) > 0 {
			hasNote = true
		}
	}
	if !hasNote && iss.Saved == 0 {
		t.Fatal("expected notes or savings for atomics-first case")
	}
}

func TestAnalyzeSkipsUnknownArrayLen(t *testing.T) {
	src := []byte(`package p
type S struct {
	A [N]byte
	B bool
	C int64
}
`)
	res, err := analyzer.AnalyzeSource("x.go", src, layout.SizerFor("amd64"))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Issues) != 0 {
		t.Fatalf("expected skip for unknown array len, got %+v", res.Issues)
	}
}

func BenchmarkAnalyzeFile(b *testing.B) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		b.Fatal("no caller")
	}
	path := filepath.Join(filepath.Dir(file), "..", "..", "examples", "complex_alignment.go")
	sizer := layout.SizerFor("amd64")

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := analyzer.AnalyzeFileWithSizer(path, sizer)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func names(issues []analyzer.Issue) []string {
	out := make([]string, len(issues))
	for i, iss := range issues {
		out[i] = iss.StructName
	}
	return out
}
