package analyzer_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
	"unsafe"

	"github.com/gopherust-io/goalign/internal/analyzer"
	"github.com/gopherust-io/goalign/internal/layout"
)

func TestIgnoreDirectiveExact(t *testing.T) {
	src := []byte(`package p
// See docs about goalign:ignore directive usage
type Mentioned struct {
	A bool
	B int64
}

// goalign:ignore
type Ignored struct {
	A bool
	B int64
}

type EOL struct {
	A bool
	B int64
} // goalign:ignore
`)
	res, err := analyzer.AnalyzeSource("x.go", src, layout.SizerFor("amd64"))
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]bool{}
	for _, iss := range res.Issues {
		byName[iss.StructName] = true
	}
	if !byName["Mentioned"] {
		t.Fatal("Mentioned should still be reported (docs mention is not a directive)")
	}
	if byName["Ignored"] {
		t.Fatal("Ignored should be skipped")
	}
	if byName["EOL"] {
		t.Fatal("EOL last-decl ignore should be skipped")
	}
}

func TestSameFileAliasWaste(t *testing.T) {
	src := []byte(`package p
type MyByte byte
type Hole struct {
	A MyByte
	B string
}
`)
	res, err := analyzer.AnalyzeSource("x.go", src, layout.SizerFor("amd64"))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Issues) != 1 || res.Issues[0].StructName != "Hole" {
		t.Fatalf("issues=%v", res.Issues)
	}
	if res.Issues[0].Wasted < 7 {
		t.Fatalf("wasted=%d", res.Issues[0].Wasted)
	}
}

func TestSizeofParityHost(t *testing.T) {
	type BadStruct struct {
		A bool
		B int64
		C int32
		D bool
	}
	type GoodStruct struct {
		B int64
		C int32
		A bool
		D bool
	}
	type TrailingZero struct {
		X int64
		Y struct{}
	}

	cases := []struct {
		name string
		src  string
		got  uintptr
	}{
		{"BadStruct", `type BadStruct struct { A bool; B int64; C int32; D bool }`, unsafe.Sizeof(BadStruct{})},
		{"GoodStruct", `type GoodStruct struct { B int64; C int32; A bool; D bool }`, unsafe.Sizeof(GoodStruct{})},
		{"TrailingZero", `type TrailingZero struct { X int64; Y struct{} }`, unsafe.Sizeof(TrailingZero{})},
	}
	s := layout.DefaultSizer()
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, "t.go", "package p\n"+tt.src, 0)
			if err != nil {
				t.Fatal(err)
			}
			var st *ast.StructType
			for _, decl := range file.Decls {
				gd := decl.(*ast.GenDecl)
				ts := gd.Specs[0].(*ast.TypeSpec)
				st = ts.Type.(*ast.StructType)
			}
			res, _ := s.Compute(nil, st.Fields, nil)
			if res.Unknown {
				t.Fatal("unexpected Unknown")
			}
			if uintptr(res.Total) != tt.got {
				t.Fatalf("Compute total=%d unsafe.Sizeof=%d", res.Total, tt.got)
			}
		})
	}
}
