package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gopherust-io/goalign/internal/analyzer"
	"github.com/gopherust-io/goalign/internal/bytesconv"
	"github.com/gopherust-io/goalign/internal/layout"
)

func TestResolvePath(t *testing.T) {
	t.Parallel()
	if got := resolvePath(nil); got != "." {
		t.Fatalf("nil args: %q", got)
	}
	if got := resolvePath([]string{"pkg"}); got != "pkg" {
		t.Fatalf("args: %q", got)
	}
}

func TestResolveSizer(t *testing.T) {
	prev := arch
	t.Cleanup(func() { arch = prev })

	arch = ""
	s, err := resolveSizer()
	if err != nil || s.PtrSize == 0 {
		t.Fatalf("default: s=%+v err=%v", s, err)
	}

	arch = "amd64"
	s, err = resolveSizer()
	if err != nil || s.PtrSize != 8 {
		t.Fatalf("amd64: s=%+v err=%v", s, err)
	}

	arch = "nope"
	if _, err := resolveSizer(); err == nil {
		t.Fatal("expected error for bad arch")
	}
}

func TestFilterMinWasteAndCount(t *testing.T) {
	t.Parallel()
	in := []analyzer.Result{{
		File: "a.go",
		Issues: []analyzer.Issue{
			{StructName: "Small", Wasted: 2},
			{StructName: "Big", Wasted: 16},
		},
	}}
	if n := countIssues(in); n != 2 {
		t.Fatalf("count=%d", n)
	}
	got := filterMinWaste(in, 8)
	if len(got) != 1 || len(got[0].Issues) != 1 || got[0].Issues[0].StructName != "Big" {
		t.Fatalf("filtered=%+v", got)
	}
	if same := filterMinWaste(in, 0); len(same) != 1 || len(same[0].Issues) != 2 {
		t.Fatalf("min=0 should keep all: %+v", same)
	}
}

func TestFilterMinWasteKeepsCacheguard(t *testing.T) {
	t.Parallel()
	in := []analyzer.Result{{
		File: "a.go",
		Issues: []analyzer.Issue{
			{StructName: "PadOnly", Wasted: 2},
			{StructName: "FalseShare", Wasted: 0, Notes: []string{"false-share: A and B share cache line 0"}},
			{StructName: "Guarded", Wasted: 0, Notes: []string{"cacheguard: suggested layout isolates contended fields"}},
		},
	}}
	got := filterMinWaste(in, 8)
	if len(got) != 1 || len(got[0].Issues) != 2 {
		t.Fatalf("want 2 kept (false-share+cacheguard), got %+v", got)
	}
	names := map[string]bool{}
	for _, iss := range got[0].Issues {
		names[iss.StructName] = true
	}
	if !names["FalseShare"] || !names["Guarded"] || names["PadOnly"] {
		t.Fatalf("names=%v", names)
	}
}

func TestShouldFormatAfterArches(t *testing.T) {
	t.Parallel()
	if !shouldFormatAfterArches("text", false, nil) {
		t.Fatal("no arches: always format")
	}
	if shouldFormatAfterArches("text", false, []string{"amd64"}) {
		t.Fatal("text+arches without verbose: matrix only")
	}
	if !shouldFormatAfterArches("text", true, []string{"amd64"}) {
		t.Fatal("text+arches+verbose")
	}
	for _, f := range []string{"json", "sarif", "table", "JSON"} {
		if !shouldFormatAfterArches(f, false, []string{"amd64"}) {
			t.Fatalf("%s should format under arches", f)
		}
	}
}

func TestShouldExclude(t *testing.T) {
	prev := exclude
	t.Cleanup(func() { exclude = prev })

	exclude = []string{"", "testdata/", "vendor"}
	if shouldExclude("pkg/foo.go") {
		t.Fatal("should not exclude")
	}
	if !shouldExclude("pkg/testdata/x.go") {
		t.Fatal("testdata")
	}
	if !shouldExclude("vendor/mod/x.go") {
		t.Fatal("vendor")
	}
}

func TestFindGoFilesNonRecursive(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "a.go"), bytesconv.StringToBytes("package a\n"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "b.txt"), bytesconv.StringToBytes("x"), 0o644)
	_ = os.MkdirAll(filepath.Join(dir, "sub"), 0o755)
	_ = os.WriteFile(filepath.Join(dir, "sub", "c.go"), bytesconv.StringToBytes("package c\n"), 0o644)

	files, err := findGoFiles(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("non-recursive files=%v want 1", files)
	}

	single := filepath.Join(dir, "a.go")
	files, err = findGoFiles(single, false)
	if err != nil || len(files) != 1 {
		t.Fatalf("single file: %v %v", files, err)
	}

	if _, err := findGoFiles(filepath.Join(dir, "b.txt"), false); err == nil {
		t.Fatal("expected error for non-go file")
	}
}

func TestFindGoFilesExclude(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "ok.go"), bytesconv.StringToBytes("package p\n"), 0o644)
	skip := filepath.Join(dir, "skipme")
	_ = os.MkdirAll(skip, 0o755)
	_ = os.WriteFile(filepath.Join(skip, "x.go"), bytesconv.StringToBytes("package x\n"), 0o644)

	prev := exclude
	t.Cleanup(func() { exclude = prev })
	exclude = []string{"skipme"}

	files, err := findGoFiles(dir, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("files=%v want only ok.go", files)
	}
}

func TestCollectResults(t *testing.T) {
	dir := t.TempDir()
	src := `package p
type Bad struct {
	A bool
	B int64
}
`
	path := filepath.Join(dir, "bad.go")
	if err := os.WriteFile(path, bytesconv.StringToBytes(src), 0o644); err != nil {
		t.Fatal(err)
	}

	prevR, prevA, prevM, prevE := recursive, arch, minWaste, exclude
	t.Cleanup(func() {
		recursive, arch, minWaste, exclude = prevR, prevA, prevM, prevE
	})
	recursive = false
	arch = "amd64"
	minWaste = 0
	exclude = nil

	results, nFiles, fileErrs, err := collectResults(nil, dir)
	if err != nil {
		t.Fatal(err)
	}
	if nFiles != 1 || fileErrs != 0 {
		t.Fatalf("nFiles=%d fileErrs=%d", nFiles, fileErrs)
	}
	if countIssues(results) == 0 {
		t.Fatal("expected issues")
	}

	minWaste = 1000
	results, _, _, err = collectResults(nil, dir)
	if err != nil {
		t.Fatal(err)
	}
	if countIssues(results) != 0 {
		t.Fatalf("min-waste should filter all: %+v", results)
	}

	if _, _, _, err := collectResults(nil, filepath.Join(dir, "missing")); err == nil {
		t.Fatal("expected missing path error")
	}

	empty := t.TempDir()
	_, nFiles, _, err = collectResults(nil, empty)
	if err != nil || nFiles != 0 {
		t.Fatalf("empty dir: nFiles=%d err=%v", nFiles, err)
	}
}

func TestAnalyzeParallel(t *testing.T) {
	dir := t.TempDir()
	src := `package p
type Bad struct {
	A bool
	B int64
}
`
	p1 := filepath.Join(dir, "a.go")
	p2 := filepath.Join(dir, "b.go")
	_ = os.WriteFile(p1, bytesconv.StringToBytes(src), 0o644)
	_ = os.WriteFile(p2, bytesconv.StringToBytes(src), 0o644)

	opts := analyzer.Options{Sizer: layout.SizerFor("amd64"), Policy: layout.PolicyAtomics}
	results, errs := analyzeParallel([]string{p1, p2, filepath.Join(dir, "missing.go")}, opts)
	if errs != 1 {
		t.Fatalf("fileErrs=%d want 1", errs)
	}
	if countIssues(results) < 2 {
		t.Fatalf("results=%+v", results)
	}
}

func TestModuleVersion(t *testing.T) {
	t.Parallel()
	v := moduleVersion()
	if v == "" {
		t.Fatal("empty version")
	}
}
