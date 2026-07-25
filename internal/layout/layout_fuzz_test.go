package layout_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"github.com/nekruzjm/goalign/internal/layout"
)

// FuzzComputeSource fuzzes struct field lists through parse + Compute.
// Invalid snippets are ignored; valid ones must not panic.
func FuzzComputeSource(f *testing.F) {
	seeds := []string{
		"type S struct { A bool; B int64 }",
		"type S struct { X [4]int32; Y string }",
		"type S struct { int64; Z bool }",
		"type S struct { Inner struct { A int; B byte }; C bool }",
		"type S struct { M map[string]int; S []byte; P *int }",
		"type S struct {}",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	sizer := layout.SizerFor("amd64")
	dst := make([]layout.Field, 64)

	f.Fuzz(func(t *testing.T, src string) {
		if len(src) > 4096 {
			return
		}
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, "fuzz.go", "package p\n"+src, 0)
		if err != nil {
			return
		}
		for _, decl := range file.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.TYPE {
				continue
			}
			for _, spec := range gd.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				st, ok := ts.Type.(*ast.StructType)
				if !ok {
					continue
				}
				res, fields := sizer.Compute(dst[:0], st.Fields)
				if res.N < 0 || res.Total < 0 || res.Wasted < 0 {
					t.Fatalf("negative metrics: %+v", res)
				}
				if len(fields) != res.N {
					t.Fatalf("len(fields)=%d n=%d", len(fields), res.N)
				}
				_ = layout.Suggest(nil, fields, res.Wasted)
			}
		}
	})
}

// FuzzTypeInfo fuzzes identifier / selector type names for size table lookups.
func FuzzTypeInfo(f *testing.F) {
	seeds := []string{
		"bool", "int", "int64", "uint64", "string", "byte",
		"float64", "complex128", "any", "error",
		"[]byte", "*int", "map[string]int", "[8]uint64",
		"atomic.Uint64", "atomic.Bool", "chan int",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	sizer := layout.SizerFor("amd64")

	f.Fuzz(func(t *testing.T, typeSrc string) {
		if len(typeSrc) == 0 || len(typeSrc) > 256 {
			return
		}
		// Wrap as a single-field struct so ParseFile accepts it.
		src := "package p\ntype S struct { F " + typeSrc + " }"
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, "fuzz.go", src, 0)
		if err != nil {
			return
		}
		gd, ok := file.Decls[0].(*ast.GenDecl)
		if !ok || len(gd.Specs) == 0 {
			return
		}
		ts, ok := gd.Specs[0].(*ast.TypeSpec)
		if !ok {
			return
		}
		st, ok := ts.Type.(*ast.StructType)
		if !ok || st.Fields == nil || len(st.Fields.List) == 0 {
			return
		}
		info := sizer.TypeInfo(st.Fields.List[0].Type)
		if info.Size < 0 || info.Align < 0 {
			t.Fatalf("negative TypeInfo: %+v for %q", info, typeSrc)
		}
		if info.Align == 0 && info.Size > 0 {
			t.Fatalf("zero align with size=%d for %q", info.Size, typeSrc)
		}
	})
}
