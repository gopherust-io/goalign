package analysis_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"github.com/gopherust-io/goalign/internal/analyzer"
	"github.com/gopherust-io/goalign/internal/layout"
)

func TestEOLIgnoreHelpers(t *testing.T) {
	src := `package p
type Reported struct {
	A bool
	B int64
}
type Skipped struct {
	A bool
	B int64
} // goalign:ignore
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "p.go", src, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	eol := analyzer.BuildEOLIgnoreLines(fset, file)
	var skipped, reported *ast.TypeSpec
	for _, decl := range file.Decls {
		gd := decl.(*ast.GenDecl)
		for _, spec := range gd.Specs {
			ts := spec.(*ast.TypeSpec)
			switch ts.Name.Name {
			case "Skipped":
				skipped = ts
			case "Reported":
				reported = ts
			}
		}
	}
	if skipped == nil || reported == nil {
		t.Fatal("parse")
	}
	if !analyzer.HasEOLIgnore(fset, skipped.End(), eol) {
		t.Fatal("Skipped should have EOL ignore")
	}
	if analyzer.HasEOLIgnore(fset, reported.End(), eol) {
		t.Fatal("Reported should not have EOL ignore")
	}
}

func TestPassUsesLocals(t *testing.T) {
	src := `package p
type Counter int64
type S struct {
	A bool
	C Counter
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "p.go", src, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	sizer := layout.SizerFor("amd64")
	locals, flags := sizer.CollectLocalsFull(file)
	var st *ast.StructType
	ast.Inspect(file, func(n ast.Node) bool {
		if ts, ok := n.(*ast.TypeSpec); ok && ts.Name.Name == "S" {
			st = ts.Type.(*ast.StructType)
			return false
		}
		return true
	})
	res, fields := sizer.Compute(nil, st.Fields, locals, flags)
	if res.Unknown {
		t.Fatal("locals should resolve Counter")
	}
	if !fields[1].IsAtomic() {
		t.Fatalf("Counter should inherit atomic flag: %+v", fields[1])
	}
}
