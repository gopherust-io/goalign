package fixer_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gopherust-io/goalign/internal/analyzer"
	"github.com/gopherust-io/goalign/internal/fixer"
	"github.com/gopherust-io/goalign/internal/layout"
)

func TestFixPaddingReorder(t *testing.T) {
	src := `package p

type BadStruct struct {
	A bool
	B int64
	C int32
	D bool
}
`
	sizer := layout.SizerFor("amd64")
	res, err := analyzer.AnalyzeSource("x.go", []byte(src), sizer)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Issues) == 0 {
		t.Fatal("expected issues")
	}

	out, n, saved, err := fixer.FixFile("x.go", []byte(src), res.Issues)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("n=%d want 1", n)
	}
	if saved <= 0 {
		t.Fatalf("saved=%d want >0", saved)
	}

	after, err := analyzer.AnalyzeSource("x.go", out, sizer)
	if err != nil {
		t.Fatal(err)
	}
	for _, iss := range after.Issues {
		if iss.StructName == "BadStruct" && iss.Saved > 0 {
			t.Fatalf("still fixable after fix:\n%s\nissue=%+v", out, iss)
		}
	}
	if !strings.Contains(string(out), "B int64") && !strings.Contains(string(out), "B\tint64") {
		t.Fatalf("missing B:\n%s", out)
	}
	bi := fieldPos(out, "B")
	ai := fieldPos(out, "A")
	if bi < 0 || ai < 0 || bi > ai {
		t.Fatalf("expected B before A:\n%s", out)
	}
}

func TestFixAtomicsFirst(t *testing.T) {
	src := `package p

type S struct {
	Flag bool
	Count int64
	Name string
}
`
	sizer := layout.SizerFor("amd64")
	res, err := analyzer.AnalyzeSource("x.go", []byte(src), sizer)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Issues) == 0 {
		t.Fatal("expected issues")
	}

	out, n, _, err := fixer.FixFile("x.go", []byte(src), res.Issues)
	if err != nil {
		t.Fatal(err)
	}
	if n < 1 {
		t.Fatalf("n=%d want >=1", n)
	}
	ci := fieldPos(out, "Count")
	fi := fieldPos(out, "Flag")
	if ci < 0 || fi < 0 || ci > fi {
		t.Fatalf("expected Count before Flag:\n%s", out)
	}
}

func TestFixMultiNameSplit(t *testing.T) {
	src := `package p

type S struct {
	A, B bool
	C    int64
}
`
	sizer := layout.SizerFor("amd64")
	res, err := analyzer.AnalyzeSource("x.go", []byte(src), sizer)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Issues) == 0 {
		t.Fatal("expected issues")
	}

	out, n, _, err := fixer.FixFile("x.go", []byte(src), res.Issues)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("n=%d want 1", n)
	}
	ci := fieldPos(out, "C")
	ai := fieldPos(out, "A")
	if ci < 0 || ai < 0 || ci > ai {
		t.Fatalf("expected C before A:\n%s", out)
	}
	after, err := analyzer.AnalyzeSource("x.go", out, sizer)
	if err != nil {
		t.Fatal(err)
	}
	for _, iss := range after.Issues {
		if iss.StructName == "S" && fixer.ShouldFix(iss) {
			t.Fatalf("still fixable:\n%s", out)
		}
	}
}

func TestFixIgnoresSkipped(t *testing.T) {
	src := `package p

// goalign:ignore
type Legacy struct {
	A bool
	B int64
}

type Bad struct {
	A bool
	B int64
}
`
	sizer := layout.SizerFor("amd64")
	res, err := analyzer.AnalyzeSource("x.go", []byte(src), sizer)
	if err != nil {
		t.Fatal(err)
	}
	for _, iss := range res.Issues {
		if iss.StructName == "Legacy" {
			t.Fatal("Legacy should be ignored")
		}
	}
	out, n, _, err := fixer.FixFile("x.go", []byte(src), res.Issues)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("n=%d want 1 (only Bad)", n)
	}
	if !strings.Contains(string(out), "type Legacy struct {\n\tA bool\n\tB int64") {
		// ignore may reformat slightly; ensure A still before B in Legacy
		legacy := string(out)
		li := strings.Index(legacy, "type Legacy struct")
		bi := strings.Index(legacy, "type Bad struct")
		chunk := legacy[li:bi]
		if strings.Index(chunk, "A bool") > strings.Index(chunk, "B int64") {
			t.Fatalf("Legacy was reordered:\n%s", out)
		}
	}
}

func TestFixPreservesTags(t *testing.T) {
	src := `package p

type S struct {
	A bool   ` + "`json:\"a\"`" + `
	B int64  ` + "`json:\"b\"`" + `
}
`
	sizer := layout.SizerFor("amd64")
	res, err := analyzer.AnalyzeSource("x.go", []byte(src), sizer)
	if err != nil {
		t.Fatal(err)
	}
	out, n, _, err := fixer.FixFile("x.go", []byte(src), res.Issues)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("n=%d", n)
	}
	if !strings.Contains(string(out), `json:"a"`) || !strings.Contains(string(out), `json:"b"`) {
		t.Fatalf("tags lost:\n%s", out)
	}
}

func TestFixPreservesComments(t *testing.T) {
	src := `package p

type BadStruct struct {
	A bool  // keep-a
	B int64 // keep-b
	C int32 // keep-c
	D bool  // keep-d
}
`
	sizer := layout.SizerFor("amd64")
	res, err := analyzer.AnalyzeSource("x.go", []byte(src), sizer)
	if err != nil {
		t.Fatal(err)
	}
	out, n, _, err := fixer.FixFile("x.go", []byte(src), res.Issues)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("n=%d", n)
	}
	s := string(out)
	for _, c := range []string{"keep-a", "keep-b", "keep-c", "keep-d"} {
		if !strings.Contains(s, c) {
			t.Fatalf("missing comment %s:\n%s", c, s)
		}
	}
	// Trailing comment should stay with its field.
	if !strings.Contains(s, "B int64 // keep-b") && !strings.Contains(s, "B int64\t// keep-b") {
		// gofmt may align spaces
		bi := strings.Index(s, "B int64")
		ki := strings.Index(s, "keep-b")
		if bi < 0 || ki < 0 || ki < bi || ki-bi > 40 {
			t.Fatalf("keep-b not near B:\n%s", s)
		}
	}
}

func TestFixIdempotent(t *testing.T) {
	src := `package p

type BadStruct struct {
	A bool
	B int64
	C int32
	D bool
}
`
	sizer := layout.SizerFor("amd64")
	res, err := analyzer.AnalyzeSource("x.go", []byte(src), sizer)
	if err != nil {
		t.Fatal(err)
	}
	out1, _, _, err := fixer.FixFile("x.go", []byte(src), res.Issues)
	if err != nil {
		t.Fatal(err)
	}
	res2, err := analyzer.AnalyzeSource("x.go", out1, sizer)
	if err != nil {
		t.Fatal(err)
	}
	out2, n2, _, err := fixer.FixFile("x.go", out1, res2.Issues)
	if err != nil {
		t.Fatal(err)
	}
	if n2 != 0 {
		t.Fatalf("second fix n=%d want 0", n2)
	}
	if !bytes.Equal(out1, out2) {
		t.Fatalf("not idempotent:\n%s\nvs\n%s", out1, out2)
	}
}

func TestFixPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.go")
	src := []byte(`package p

type BadStruct struct {
	A bool
	B int64
	C int32
	D bool
}
`)
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatal(err)
	}
	sizer := layout.SizerFor("amd64")
	res, err := analyzer.AnalyzeFileWithSizer(path, sizer)
	if err != nil {
		t.Fatal(err)
	}
	fr, err := fixer.FixPath(path, res.Issues)
	if err != nil {
		t.Fatal(err)
	}
	if !fr.Changed || fr.BytesSaved <= 0 {
		t.Fatalf("fr=%+v", fr)
	}
	after, err := analyzer.AnalyzeFileWithSizer(path, sizer)
	if err != nil {
		t.Fatal(err)
	}
	for _, iss := range after.Issues {
		if fixer.ShouldFix(iss) {
			t.Fatalf("still fixable after FixPath")
		}
	}
}

// fieldPos returns the byte index of a struct field declaration named name.
func fieldPos(src []byte, name string) int {
	for _, prefix := range []string{"\t" + name + " ", "\t" + name + "\t", " " + name + " "} {
		if i := bytes.Index(src, []byte(prefix)); i >= 0 {
			return i
		}
	}
	// multi-name: "A, B bool"
	if i := bytes.Index(src, []byte(name+",")); i >= 0 {
		return i
	}
	if i := bytes.Index(src, []byte(", "+name+" ")); i >= 0 {
		return i
	}
	return bytes.Index(src, []byte(name))
}

func TestFixEmbed(t *testing.T) {
	src := `package p

type Inner struct {
	X int64
}

type Outer struct {
	Flag bool
	Inner
	N int64
}
`
	sizer := layout.SizerFor("amd64")
	res, err := analyzer.AnalyzeSource("x.go", []byte(src), sizer)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Issues) == 0 {
		t.Fatal("expected issues")
	}
	out, n, _, err := fixer.FixFile("x.go", []byte(src), res.Issues)
	if err != nil {
		t.Fatal(err)
	}
	if n < 1 {
		t.Fatalf("n=%d", n)
	}
	if !strings.Contains(string(out), "Inner") {
		t.Fatalf("embed lost:\n%s", out)
	}
	after, err := analyzer.AnalyzeSource("x.go", out, sizer)
	if err != nil {
		t.Fatal(err)
	}
	for _, iss := range after.Issues {
		if iss.StructName == "Outer" && fixer.ShouldFix(iss) {
			t.Fatalf("Outer still fixable:\n%s", out)
		}
	}
}

func TestFixMultiNameReorderSplit(t *testing.T) {
	// Suggested order puts C first, then B, then A — forces splitting A,B.
	src := `package p

type S struct {
	A, B bool
	C    int64
}
`
	sizer := layout.SizerFor("amd64")
	res, err := analyzer.AnalyzeSource("x.go", []byte(src), sizer)
	if err != nil {
		t.Fatal(err)
	}
	out, n, _, err := fixer.FixFile("x.go", []byte(src), res.Issues)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("n=%d", n)
	}
	s := string(out)
	// After density sort, C should lead; A and B may be separate decls.
	if fieldPos([]byte(s), "C") > fieldPos([]byte(s), "A") {
		t.Fatalf("expected C before A:\n%s", s)
	}
}

func TestFixPathNoChange(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ok.go")
	src := []byte(`package p

type Good struct {
	B int64
	C int32
	A bool
	D bool
}
`)
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatal(err)
	}
	sizer := layout.SizerFor("amd64")
	res, err := analyzer.AnalyzeFileWithSizer(path, sizer)
	if err != nil {
		t.Fatal(err)
	}
	fr, err := fixer.FixPath(path, res.Issues)
	if err != nil {
		t.Fatal(err)
	}
	if fr.Changed {
		t.Fatalf("should not rewrite already-good order: %+v", fr)
	}
}

func TestFixPathEmptyIssues(t *testing.T) {
	fr, err := fixer.FixPath("nope.go", nil)
	if err != nil {
		t.Fatal(err)
	}
	if fr.Changed {
		t.Fatal("empty issues")
	}
}

func BenchmarkFixFile(b *testing.B) {
	src := []byte(`package p

type BadStruct struct {
	A bool
	B int64
	C int32
	D bool
}
`)
	sizer := layout.SizerFor("amd64")
	res, err := analyzer.AnalyzeSource("x.go", src, sizer)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _, err := fixer.FixFile("x.go", src, res.Issues)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func TestFixSplitsMultiNameWhenSeparated(t *testing.T) {
	src := []byte(`package p

type S struct {
	A, B bool
	C    int32
}
`)
	// Craft a suggested order that separates A and B (analyzer never does this
	// for equal-density siblings, but the fixer must still handle it).
	iss := analyzer.Issue{
		StructName: "S",
		Line:       3,
		Saved:      1,
		Fields: []layout.Field{
			{Name: "A"}, {Name: "B"}, {Name: "C"},
		},
		Suggested: []layout.Field{
			{Name: "A"}, {Name: "C"}, {Name: "B"},
		},
	}
	out, n, _, err := fixer.FixFile("x.go", src, []analyzer.Issue{iss})
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("n=%d", n)
	}
	s := string(out)
	if strings.Contains(s, "A, B") || strings.Contains(s, "A,B") {
		t.Fatalf("expected split multi-name:\n%s", s)
	}
	if !strings.Contains(s, "A bool") || !strings.Contains(s, "B bool") || !strings.Contains(s, "C") {
		t.Fatalf("missing fields:\n%s", s)
	}
}

func TestShouldFix(t *testing.T) {
	if fixer.ShouldFix(analyzer.Issue{}) {
		t.Fatal("empty should not fix")
	}
	iss := analyzer.Issue{
		Fields:    []layout.Field{{Name: "A", Index: 0}, {Name: "B", Index: 1}},
		Suggested: []layout.Field{{Name: "A", Index: 0}, {Name: "B", Index: 1}},
		Saved:     8,
	}
	if fixer.ShouldFix(iss) {
		t.Fatal("same order should not fix")
	}
	iss.Suggested = []layout.Field{{Name: "B", Index: 1}, {Name: "A", Index: 0}}
	if !fixer.ShouldFix(iss) {
		t.Fatal("saved>0 should fix")
	}
	iss.Saved = 0
	iss.Notes = []string{"atomics-first: place counters first"}
	if !fixer.ShouldFix(iss) {
		t.Fatal("atomics-first should fix")
	}
	iss.Notes = nil
	if fixer.ShouldFix(iss) {
		t.Fatal("no saved and no notes")
	}
}

func TestFixBlankIdentifiers(t *testing.T) {
	src := `package p

type S struct {
	_ bool
	N int64
	_ byte
}
`
	sizer := layout.SizerFor("amd64")
	res, err := analyzer.AnalyzeSource("x.go", []byte(src), sizer)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Issues) == 0 || res.Issues[0].Saved == 0 {
		t.Fatalf("expected fixable issue: %+v", res.Issues)
	}
	out, n, _, err := fixer.FixFile("x.go", []byte(src), res.Issues)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("n=%d out:\n%s", n, out)
	}
}

func TestFixPreservesSplitComments(t *testing.T) {
	src := []byte(`package p

type S struct {
	// doc-a
	A, B bool // trail
	C    int32
}
`)
	// Force A/C/B order so the multi-name decl splits (analyzer keeps equal-density siblings together).
	iss := analyzer.Issue{
		StructName: "S",
		Line:       3,
		Saved:      1,
		Fields: []layout.Field{
			{Name: "A", Index: 0}, {Name: "B", Index: 1}, {Name: "C", Index: 2},
		},
		Suggested: []layout.Field{
			{Name: "A", Index: 0}, {Name: "C", Index: 2}, {Name: "B", Index: 1},
		},
	}
	out, n, _, err := fixer.FixFile("x.go", src, []analyzer.Issue{iss})
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("n=%d", n)
	}
	s := string(out)
	if strings.Contains(s, "A, B") || strings.Contains(s, "A,B") {
		t.Fatalf("expected multi-name split:\n%s", s)
	}
	if !strings.Contains(s, "doc-a") {
		t.Fatalf("doc comment lost:\n%s", s)
	}
	if !strings.Contains(s, "trail") {
		t.Fatalf("trailing comment lost:\n%s", s)
	}
}
