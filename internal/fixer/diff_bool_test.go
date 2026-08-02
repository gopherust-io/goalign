package fixer_test

import (
	"strings"
	"testing"

	"github.com/gopherust-io/goalign/internal/analyzer"
	"github.com/gopherust-io/goalign/internal/fixer"
	"github.com/gopherust-io/goalign/internal/layout"
)

func TestFixDiffOnly(t *testing.T) {
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
	fr, err := fixer.FixContentWithOptions("x.go", []byte(src), res.Issues, fixer.Options{DiffOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if !fr.Changed || fr.Diff == "" {
		t.Fatalf("expected diff: %+v", fr)
	}
	if !strings.Contains(fr.Diff, "--- a/x.go") {
		t.Fatalf("diff:\n%s", fr.Diff)
	}
}

func TestRewriteBools(t *testing.T) {
	// Crafted: no density/atomics reorder (ShouldFix false), bool-pack only.
	src := []byte(`package p

type S struct {
	a bool
	X int64
	b bool
	c bool
}
`)
	iss := analyzer.Issue{
		StructName: "S",
		Line:       3,
		Saved:      0,
		BoolPack:   []string{"a", "b", "c"},
		Fields: []layout.Field{
			{Name: "a", Index: 0, Size: 1, Align: 1, Flags: layout.FlagBool},
			{Name: "X", Index: 1, Size: 8, Align: 8, Flags: layout.FlagAtomic},
			{Name: "b", Index: 2, Size: 1, Align: 1, Flags: layout.FlagBool},
			{Name: "c", Index: 3, Size: 1, Align: 1, Flags: layout.FlagBool},
		},
	}
	out, n, _, err := fixer.FixFileWithOptions("x.go", src, []analyzer.Issue{iss}, fixer.Options{RewriteBools: true})
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("n=%d", n)
	}
	s := string(out)
	if !strings.Contains(s, "flags uint8") || !strings.Contains(s, "goalign:bool-pack") {
		t.Fatalf("out:\n%s", s)
	}
	if strings.Contains(s, "\ta bool") || strings.Contains(s, " a bool") {
		t.Fatalf("bools should be packed:\n%s", s)
	}
}

func TestRewriteBoolsDefersToDensity(t *testing.T) {
	// When both density fix and bool-pack apply, density wins.
	src := `package p

type S struct {
	a bool
	X int64
	b bool
	c bool
}
`
	sizer := layout.SizerFor("amd64")
	res, err := analyzer.AnalyzeSource("x.go", []byte(src), sizer)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Issues) == 0 || !fixer.ShouldFix(res.Issues[0]) {
		t.Fatalf("expected density ShouldFix: %+v", res.Issues)
	}
	if len(res.Issues[0].BoolPack) < 3 {
		t.Fatalf("expected bool pack candidates: %+v", res.Issues[0].BoolPack)
	}
	out, n, _, err := fixer.FixFileWithOptions("x.go", []byte(src), res.Issues, fixer.Options{RewriteBools: true})
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("n=%d", n)
	}
	s := string(out)
	if strings.Contains(s, "flags uint") {
		t.Fatalf("density should win over bool-pack:\n%s", s)
	}
	if !strings.Contains(s, "X int64") {
		t.Fatalf("expected reorder:\n%s", s)
	}
}

func TestRewriteBoolsSkipsFieldIgnore(t *testing.T) {
	src := `package p

type S struct {
	keep bool // goalign:ignore
	a bool
	X int64
	b bool
	c bool
}
`
	sizer := layout.SizerFor("amd64")
	res, err := analyzer.AnalyzeSource("x.go", []byte(src), sizer)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Issues) == 0 {
		t.Fatal("expected issue")
	}
	if len(res.Issues[0].BoolPack) != 0 {
		t.Fatalf("BoolPack should be empty with field-ignore: %v", res.Issues[0].BoolPack)
	}
	out, n, _, err := fixer.FixFileWithOptions("x.go", []byte(src), res.Issues, fixer.Options{RewriteBools: true})
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("expected no rewrite, n=%d out:\n%s", n, out)
	}
	if !strings.Contains(string(out), "keep bool") {
		t.Fatalf("keep field must remain:\n%s", out)
	}
}

func TestRewriteBoolsFlagsCollision(t *testing.T) {
	src := []byte(`package p

type S struct {
	a bool
	X int64
	b bool
	c bool
	flags uint32
}
`)
	iss := analyzer.Issue{
		StructName: "S",
		Line:       3,
		Saved:      0,
		BoolPack:   []string{"a", "b", "c"},
		Fields: []layout.Field{
			{Name: "a", Index: 0},
			{Name: "X", Index: 1},
			{Name: "b", Index: 2},
			{Name: "c", Index: 3},
			{Name: "flags", Index: 4},
		},
	}
	_, _, _, err := fixer.FixFileWithOptions("x.go", src, []analyzer.Issue{iss}, fixer.Options{RewriteBools: true})
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected flags collision error, got %v", err)
	}
}
