package layout_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/gopherust-io/goalign/internal/layout"
)

func parseStruct(t *testing.T, src string) *ast.StructType {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "t.go", "package p\n"+src, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, decl := range file.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, spec := range gd.Specs {
			ts := spec.(*ast.TypeSpec)
			return ts.Type.(*ast.StructType)
		}
	}
	t.Fatal("no struct found")
	return nil
}

func TestComputeGolden(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		src        string
		wantN      int
		wantTotal  int
		wantWasted int
		check      func(t *testing.T, fields []layout.Field)
	}{
		{
			name: "bad_struct",
			src: `type BadStruct struct {
				A bool
				B int64
				C int32
				D bool
			}`,
			wantN: 4, wantTotal: 24, wantWasted: 10,
		},
		{
			name: "good_struct",
			src: `type GoodStruct struct {
				B int64
				C int32
				A bool
				D bool
			}`,
			wantN: 4, wantTotal: 16, wantWasted: 2,
		},
		{
			name: "fixed_array",
			src: `type S struct {
				A [4]int32
				B bool
			}`,
			wantN: 2, wantTotal: 20, wantWasted: 3,
			check: func(t *testing.T, fields []layout.Field) {
				t.Helper()
				if fields[0].Size != 16 {
					t.Fatalf("array size=%d want 16", fields[0].Size)
				}
			},
		},
		{
			name: "nested_anon",
			src: `type S struct {
				Inner struct {
					X int64
					Y bool
				}
				Z bool
			}`,
			wantN: 2, wantTotal: 24, wantWasted: 7,
			check: func(t *testing.T, fields []layout.Field) {
				t.Helper()
				if fields[0].Size != 16 {
					t.Fatalf("nested size=%d want 16", fields[0].Size)
				}
			},
		},
		{
			name: "embed",
			src: `type S struct {
				int64
				Z bool
			}`,
			wantN: 2, wantTotal: 16, wantWasted: 7,
			check: func(t *testing.T, fields []layout.Field) {
				t.Helper()
				if fields[0].Name != "int64" {
					t.Fatalf("embed name=%q want int64", fields[0].Name)
				}
				if fields[0].Size != 8 {
					t.Fatalf("embed size=%d want 8", fields[0].Size)
				}
			},
		},
		{
			name: "trailing_zero_sized",
			src: `type S struct {
				X int64
				Y struct{}
			}`,
			wantN: 2, wantTotal: 16, wantWasted: 8,
		},
		{
			name: "only_zero_sized",
			src: `type S struct {
				Y struct{}
			}`,
			wantN: 1, wantTotal: 0, wantWasted: 0,
		},
		{
			name: "array_shift_len",
			src: `type S struct {
				A [1<<3]byte
				B bool
			}`,
			wantN: 2, wantTotal: 9, wantWasted: 0,
			check: func(t *testing.T, fields []layout.Field) {
				t.Helper()
				if fields[0].Size != 8 {
					t.Fatalf("array size=%d want 8", fields[0].Size)
				}
			},
		},
		{
			name: "zero_array_only",
			src: `type S struct {
				Z [0]byte
			}`,
			wantN: 1, wantTotal: 0, wantWasted: 0,
		},
	}

	s := layout.SizerFor("amd64")
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			st := parseStruct(t, tt.src)
			res, fields := s.Compute(nil, st.Fields, nil)
			if res.N != tt.wantN {
				t.Fatalf("n=%d want %d", res.N, tt.wantN)
			}
			if res.Total != tt.wantTotal {
				t.Fatalf("total=%d want %d", res.Total, tt.wantTotal)
			}
			if res.Wasted != tt.wantWasted {
				t.Fatalf("wasted=%d want %d", res.Wasted, tt.wantWasted)
			}
			if tt.check != nil {
				tt.check(t, fields)
			}
		})
	}
}

func TestComputeNoAlloc(t *testing.T) {
	st := parseStruct(t, `type S struct {
		A bool
		B int64
		C int32
		D bool
		E uint16
		F byte
	}`)
	s := layout.SizerFor("amd64")
	dst := make([]layout.Field, 16)

	allocs := testing.AllocsPerRun(1000, func() {
		_, _ = s.Compute(dst[:0], st.Fields, nil)
	})
	if allocs != 0 {
		t.Fatalf("Compute allocs/op = %v, want 0", allocs)
	}
}

func TestUnknownArrayLen(t *testing.T) {
	t.Parallel()
	st := parseStruct(t, `type S struct {
		A [N]byte
		B int64
	}`)
	s := layout.SizerFor("amd64")
	res, _ := s.Compute(nil, st.Fields, nil)
	if !res.Unknown {
		t.Fatal("expected Unknown for named const array length")
	}
}

func TestSuggestNoAlloc(t *testing.T) {
	st := parseStruct(t, `type S struct {
		A bool
		B int64
		C int32
		D bool
	}`)
	s := layout.SizerFor("amd64")
	_, fields := s.Compute(nil, st.Fields, nil)
	dst := make([]layout.Field, 2*len(fields))

	// Warmup + ensure Suggest works with a reused 2*n buffer (no grow).
	_ = layout.Suggest(dst[:0], fields, 10)
	sug := layout.Suggest(dst[:0], fields, 10)
	if len(sug.Fields) != len(fields) {
		t.Fatalf("len=%d want %d", len(sug.Fields), len(fields))
	}
	// Cap must remain shared with dst (no realloc of the result slice).
	if cap(sug.Fields) < 2*len(fields) {
		t.Fatalf("cap=%d want >= %d (reused dst)", cap(sug.Fields), 2*len(fields))
	}
}

func TestSuggestSavesBytes(t *testing.T) {
	t.Parallel()
	st := parseStruct(t, `type BadStruct struct {
		A bool
		B int64
		C int32
		D bool
	}`)
	s := layout.SizerFor("amd64")
	res, fields := s.Compute(nil, st.Fields, nil)
	layout.FillTypeNames(fields, st.Fields)
	sug := layout.Suggest(nil, fields, res.Wasted)
	if sug.Saved < 8 {
		t.Fatalf("saved=%d want >= 8 (suggested wasted=%d original=%d order=%v)",
			sug.Saved, sug.Wasted, res.Wasted, names(sug.Fields))
	}
	if sug.Wasted > res.Wasted {
		t.Fatalf("suggested worse: %d > %d", sug.Wasted, res.Wasted)
	}
}

func TestSuggestAtomicsFirst(t *testing.T) {
	t.Parallel()
	st := parseStruct(t, `type S struct {
		Flag bool
		Count int64
		Name string
	}`)
	s := layout.SizerFor("amd64")
	_, fields := s.Compute(nil, st.Fields, nil)
	sug := layout.Suggest(nil, fields, 8)
	if !sug.Fields[0].IsAtomic() {
		t.Fatalf("first field should be atomic counter, got %+v", sug.Fields[0])
	}
	found := false
	for _, n := range sug.Notes {
		if contains(n, "atomics-first") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected atomics-first note, got %v", sug.Notes)
	}
}

func TestBoolPackNote(t *testing.T) {
	t.Parallel()
	st := parseStruct(t, `type S struct {
		A bool
		B int64
		C bool
		D bool
	}`)
	s := layout.SizerFor("amd64")
	res, fields := s.Compute(nil, st.Fields, nil)
	sug := layout.Suggest(nil, fields, res.Wasted)
	found := false
	for _, n := range sug.Notes {
		if contains(n, "bool-pack") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected bool-pack note, got %v", sug.Notes)
	}
}

func TestSizerArch(t *testing.T) {
	t.Parallel()
	tests := []struct {
		arch      string
		ident     string
		wantSize  int
		wantAlign int
	}{
		{arch: "386", ident: "int", wantSize: 4, wantAlign: 4},
		{arch: "386", ident: "int64", wantSize: 8, wantAlign: 4},
		{arch: "amd64", ident: "int", wantSize: 8, wantAlign: 8},
		{arch: "amd64", ident: "int64", wantSize: 8, wantAlign: 8},
		{arch: "arm64", ident: "string", wantSize: 16, wantAlign: 8},
	}
	for _, tt := range tests {
		t.Run(tt.arch+"/"+tt.ident, func(t *testing.T) {
			t.Parallel()
			s := layout.SizerFor(tt.arch)
			info := s.TypeInfo(&ast.Ident{Name: tt.ident}, nil)
			if info.Size != tt.wantSize || info.Align != tt.wantAlign {
				t.Fatalf("got %+v want size=%d align=%d", info, tt.wantSize, tt.wantAlign)
			}
		})
	}
}

func TestCompute386BoolInt64(t *testing.T) {
	t.Parallel()
	st := parseStruct(t, `type S struct {
		A bool
		B int64
	}`)
	s := layout.SizerFor("386")
	res, _ := s.Compute(nil, st.Fields, nil)
	if res.Total != 12 {
		t.Fatalf("total=%d want 12 on 386", res.Total)
	}
}

func TestNegativeArrayLenUnknown(t *testing.T) {
	t.Parallel()
	st := parseStruct(t, `type S struct {
		A [0-1]struct{}
		B int64
	}`)
	s := layout.SizerFor("amd64")
	res, _ := s.Compute(nil, st.Fields, nil)
	if !res.Unknown {
		t.Fatal("expected Unknown for negative array length")
	}
}

func TestOverflowArrayLenUnknown(t *testing.T) {
	t.Parallel()
	st := parseStruct(t, `type S struct {
		A [1<<61]uint64
	}`)
	s := layout.SizerFor("amd64")
	res, _ := s.Compute(nil, st.Fields, nil)
	if !res.Unknown {
		t.Fatal("expected Unknown for overflowing array size")
	}
}

func TestSameFileNamedType(t *testing.T) {
	t.Parallel()
	src := `package p
type MyByte byte
type Hole struct {
	A MyByte
	B string
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "t.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	s := layout.SizerFor("amd64")
	locals := s.CollectLocals(file)
	var st *ast.StructType
	for _, decl := range file.Decls {
		gd := decl.(*ast.GenDecl)
		for _, spec := range gd.Specs {
			ts := spec.(*ast.TypeSpec)
			if ts.Name.Name == "Hole" {
				st = ts.Type.(*ast.StructType)
			}
		}
	}
	res, _ := s.Compute(nil, st.Fields, locals)
	if res.Unknown {
		t.Fatal("unexpected Unknown")
	}
	if res.Wasted < 7 {
		t.Fatalf("wasted=%d want >= 7", res.Wasted)
	}
}

func TestValidArch(t *testing.T) {
	if !layout.ValidArch("amd64") || layout.ValidArch("x86_64") {
		t.Fatal("ValidArch mismatch")
	}
}

func BenchmarkCompute(b *testing.B) {
	src := `type S struct {
		A bool
		B int64
		C int32
		D bool
		E []byte
		F string
		G uint16
	}`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "t.go", "package p\n"+src, 0)
	if err != nil {
		b.Fatal(err)
	}
	st := file.Decls[0].(*ast.GenDecl).Specs[0].(*ast.TypeSpec).Type.(*ast.StructType)
	s := layout.SizerFor("amd64")
	dst := make([]layout.Field, 16)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = s.Compute(dst[:0], st.Fields, nil)
	}
}

func names(fields []layout.Field) []string {
	out := make([]string, len(fields))
	for i, f := range fields {
		out[i] = f.Name
	}
	return out
}

func contains(s, sub string) bool {
	return strings.Contains(s, sub)
}
