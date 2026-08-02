package layout_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
	"unsafe"

	"github.com/gopherust-io/goalign/internal/layout"
)

func TestDefaultSizer(t *testing.T) {
	t.Parallel()
	s := layout.DefaultSizer()
	if s.PtrSize != int(unsafe.Sizeof(uintptr(0))) {
		t.Fatalf("PtrSize=%d", s.PtrSize)
	}
}

func TestSuggestEmptyAndGrow(t *testing.T) {
	t.Parallel()
	empty := layout.Suggest(nil, nil, 0)
	if len(empty.Fields) != 0 {
		t.Fatal("empty")
	}

	st := parseStruct(t, `type S struct {
		A bool
		B int64
	}`)
	s := layout.SizerFor("amd64")
	_, fields := s.Compute(nil, st.Fields, nil)

	// Force both allocation branches (cap < n and cap >= n but < 2n).
	sug := layout.Suggest(nil, fields, 8)
	if len(sug.Fields) != len(fields) {
		t.Fatalf("nil dst len=%d", len(sug.Fields))
	}
	small := make([]layout.Field, len(fields))
	sug = layout.Suggest(small[:0], fields, 8)
	if len(sug.Fields) != len(fields) {
		t.Fatalf("cap=n len=%d", len(sug.Fields))
	}
}

func TestFillTypeNamesVariety(t *testing.T) {
	t.Parallel()
	st := parseStruct(t, `type S struct {
		A *int
		B []byte
		C [4]int
		D map[string]int
		E chan int
		F func()
		G interface{}
		H struct{}
		I (int)
		J, K bool
	}`)
	s := layout.SizerFor("amd64")
	res, fields := s.Compute(nil, st.Fields, nil)
	if res.Unknown {
		t.Fatal("unexpected unknown")
	}
	layout.FillTypeNames(fields, st.Fields)
	joined := ""
	for _, f := range fields {
		joined += f.Name + ":" + f.Type + ";"
	}
	for _, want := range []string{
		"A:*int",
		"B:[]byte",
		"C:[4]int",
		"D:map[string]int",
		"E:chan int",
		"F:func",
		"G:interface{}",
		"H:struct{}",
		"I:int",
		"J:bool",
		"K:bool",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %s in %s", want, joined)
		}
	}
}

func TestEmbedAndAtomicSelector(t *testing.T) {
	t.Parallel()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "t.go", `package p
type Inner struct { X int }
type S struct {
	Inner
	*Inner
	N atomic.Int64
	B atomic.Bool
	U uint64
}
`, 0)
	if err != nil {
		t.Fatal(err)
	}
	s := layout.SizerFor("amd64")
	locals := s.CollectLocals(file)
	st := file.Decls[1].(*ast.GenDecl).Specs[0].(*ast.TypeSpec).Type.(*ast.StructType)
	res, fields := s.Compute(nil, st.Fields, locals)
	if res.Unknown {
		t.Fatalf("unexpected unknown locals=%v", locals)
	}
	layout.FillTypeNames(fields, st.Fields)
	names := map[string]layout.Field{}
	for _, f := range fields {
		names[f.Name] = f
	}
	if _, ok := names["Inner"]; !ok {
		t.Fatalf("embed Inner missing: %v", fieldNames(fields))
	}
	// Second embed is *Inner — embedName returns Inner again; may collide in map.
	if !names["N"].IsAtomic() || !names["U"].IsAtomic() {
		t.Fatalf("atomic flags: %+v %+v", names["N"], names["U"])
	}
	if !names["B"].IsBool() {
		t.Fatalf("atomic.Bool should be bool flag: %+v", names["B"])
	}
}

func TestTypeInfoExtras(t *testing.T) {
	t.Parallel()
	s := layout.SizerFor("amd64")
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "t.go", `package p
type S struct {
	A [1<<3]byte
	B [0]int
	C any
}
`, 0)
	if err != nil {
		t.Fatal(err)
	}
	st := file.Decls[0].(*ast.GenDecl).Specs[0].(*ast.TypeSpec).Type.(*ast.StructType)
	res, fields := s.Compute(nil, st.Fields, nil)
	if res.Unknown {
		t.Fatal("unknown")
	}
	if len(fields) != 3 {
		t.Fatalf("fields=%d", len(fields))
	}
	// [0]int is zero-sized
	if fields[1].Size != 0 {
		t.Fatalf("zero array size=%d", fields[1].Size)
	}
}

func TestCollectLocalsAlias(t *testing.T) {
	t.Parallel()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "t.go", `package p
type ID int64
type Alias = ID
type S struct {
	A Alias
	B bool
}
`, 0)
	if err != nil {
		t.Fatal(err)
	}
	s := layout.SizerFor("amd64")
	locals, flags := s.CollectLocalsFull(file)
	if locals["ID"].Size != 8 || locals["Alias"].Size != 8 {
		t.Fatalf("locals=%v", locals)
	}
	if flags["ID"]&layout.FlagAtomic == 0 || flags["Alias"]&layout.FlagAtomic == 0 {
		t.Fatalf("expected atomic flags for ID/Alias: %v", flags)
	}
	st := file.Decls[2].(*ast.GenDecl).Specs[0].(*ast.TypeSpec).Type.(*ast.StructType)
	res, fields := s.Compute(nil, st.Fields, locals, flags)
	if res.Unknown || res.Wasted < 7 {
		t.Fatalf("res=%+v", res)
	}
	if !fields[0].IsAtomic() {
		t.Fatalf("Alias field should be atomic: %+v", fields[0])
	}
}

func TestAtomicValueAndUnsafePointerSizes(t *testing.T) {
	t.Parallel()
	amd := layout.SizerFor("amd64")
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "t.go", `package p
import ("sync/atomic"; "unsafe")
type S struct {
	V atomic.Value
	P unsafe.Pointer
	U atomic.Uintptr
	X byte
}
`, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	st := file.Decls[1].(*ast.GenDecl).Specs[0].(*ast.TypeSpec).Type.(*ast.StructType)
	res, fields := amd.Compute(nil, st.Fields, nil)
	if res.Unknown {
		t.Fatalf("unexpected unknown: %+v", res)
	}
	by := map[string]layout.Field{}
	for _, f := range fields {
		by[f.Name] = f
	}
	if by["V"].Size != 16 || by["V"].Align != 8 {
		t.Fatalf("atomic.Value amd64: %+v", by["V"])
	}
	if by["P"].Size != 8 || by["U"].Size != 8 {
		t.Fatalf("ptr-sized: P=%+v U=%+v", by["P"], by["U"])
	}
	// Real layout: Value(16)+Pointer(8)+Uintptr(8)+byte+pad7 = 40
	if res.Total != 40 {
		t.Fatalf("total=%d want 40", res.Total)
	}

	x86 := layout.SizerFor("386")
	res32, fields32 := x86.Compute(nil, st.Fields, nil)
	if res32.Unknown {
		t.Fatal("386 unknown")
	}
	by32 := map[string]layout.Field{}
	for _, f := range fields32 {
		by32[f.Name] = f
	}
	if by32["V"].Size != 8 || by32["V"].Align != 4 {
		t.Fatalf("atomic.Value 386: %+v", by32["V"])
	}
	if by32["P"].Size != 4 || by32["U"].Size != 4 {
		t.Fatalf("386 ptr-sized: P=%+v U=%+v", by32["P"], by32["U"])
	}
}

func TestEvalIntExpressions(t *testing.T) {
	t.Parallel()
	st := parseStruct(t, `type S struct {
		A [1+2]byte
		B [2*3]byte
		C [(4)]byte
		D [+5]byte
		E [8/2]byte
		F [7%4]byte
		G [1<<3]byte
		H [16>>2]byte
		I [1|2]byte
		J [7&3]byte
		K [1^0]byte
	}`)
	s := layout.SizerFor("amd64")
	res, fields := s.Compute(nil, st.Fields, nil)
	if res.Unknown {
		t.Fatal("unexpected unknown")
	}
	wantSizes := []int{3, 6, 4, 5, 4, 3, 8, 4, 3, 3, 1}
	if len(fields) != len(wantSizes) {
		t.Fatalf("n=%d", len(fields))
	}
	for i, want := range wantSizes {
		if fields[i].Size != want {
			t.Fatalf("%s size=%d want %d", fields[i].Name, fields[i].Size, want)
		}
	}
}

func TestEvalIntRejects(t *testing.T) {
	t.Parallel()
	for _, src := range []string{
		`type S struct { A [-1]byte }`,
		`type S struct { A [1/0]byte }`,
		`type S struct { A [N]byte }`,
	} {
		st := parseStruct(t, src)
		res, _ := layout.SizerFor("amd64").Compute(nil, st.Fields, nil)
		if !res.Unknown {
			t.Fatalf("expected unknown for %s", src)
		}
	}
}

func fieldNames(fields []layout.Field) []string {
	out := make([]string, len(fields))
	for i, f := range fields {
		out[i] = f.Name
	}
	return out
}
