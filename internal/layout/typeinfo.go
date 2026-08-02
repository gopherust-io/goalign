package layout

import (
	"go/ast"
	"go/token"
	"math/bits"
	"strconv"

	"github.com/gopherust-io/goalign/internal/alignmath"
	"github.com/gopherust-io/goalign/internal/goarch"
)

// Unknown marks a type whose size cannot be determined from the AST alone.
var Unknown = Info{Size: -1, Align: -1}

// IsUnknown reports whether size/align could not be resolved.
func (i Info) IsUnknown() bool { return i.Size < 0 }

// ValidArch reports whether goarch is a known GOARCH for SizerFor.
func ValidArch(name string) bool {
	return goarch.Valid(name)
}

// TypeInfo returns size and alignment for an AST type expression.
// locals maps same-file type names to resolved Info (may be nil).
// Unresolvable array lengths and unknown named types return Unknown.
func (s Sizer) TypeInfo(expr ast.Expr, locals map[string]Info) Info {
	if expr == nil {
		return Unknown
	}
	switch t := expr.(type) {
	case *ast.Ident:
		return s.identInfo(t.Name, locals)
	case *ast.StarExpr:
		return s.ptr()
	case *ast.ArrayType:
		if t.Len == nil {
			return Info{Size: 3 * s.PtrSize, Align: s.PtrSize}
		}
		n, ok := evalInt(t.Len)
		if !ok || n < 0 {
			return Unknown
		}
		elem := s.TypeInfo(t.Elt, locals)
		if elem.IsUnknown() {
			return Unknown
		}
		if n == 0 {
			align := elem.Align
			if align < 1 {
				align = 1
			}
			return Info{Size: 0, Align: align}
		}
		hi, lo := bits.Mul64(uint64(n), uint64(elem.Size))
		if hi != 0 || lo > uint64(^uint(0)>>1) {
			return Unknown
		}
		return Info{Size: int(lo), Align: elem.Align}
	case *ast.StructType:
		return s.structInfo(t, locals)
	case *ast.SelectorExpr:
		return s.selectorInfo(t, locals)
	case *ast.IndexExpr:
		// atomic.Pointer[T] / generic instantiation — size of the base type.
		return s.TypeInfo(t.X, locals)
	case *ast.IndexListExpr:
		return s.TypeInfo(t.X, locals)
	case *ast.MapType, *ast.ChanType, *ast.FuncType:
		return s.ptr()
	case *ast.InterfaceType:
		return Info{Size: 2 * s.PtrSize, Align: s.PtrSize}
	case *ast.Ellipsis:
		return s.TypeInfo(t.Elt, locals)
	case *ast.ParenExpr:
		return s.TypeInfo(t.X, locals)
	default:
		return Unknown
	}
}

func (s Sizer) align64() int {
	if s.PtrSize == 4 {
		return 4
	}
	return 8
}

func (s Sizer) identInfo(name string, locals map[string]Info) Info {
	switch name {
	case "bool", "int8", "uint8", "byte":
		return Info{Size: 1, Align: 1}
	case "int16", "uint16":
		return Info{Size: 2, Align: 2}
	case "int32", "uint32", "rune", "float32":
		return Info{Size: 4, Align: 4}
	case "int64", "uint64", "float64":
		a := s.align64()
		return Info{Size: 8, Align: a}
	case "complex64":
		return Info{Size: 8, Align: 4}
	case "complex128":
		a := s.align64()
		return Info{Size: 16, Align: a}
	case "int", "uint", "uintptr":
		return s.ptr()
	case "string":
		return Info{Size: 2 * s.PtrSize, Align: s.PtrSize}
	case "any", "error":
		return Info{Size: 2 * s.PtrSize, Align: s.PtrSize}
	default:
		if locals != nil {
			if info, ok := locals[name]; ok {
				return info
			}
		}
		// Unresolved named type (likely imported) — skip rather than guess.
		return Unknown
	}
}

func (s Sizer) selectorInfo(sel *ast.SelectorExpr, locals map[string]Info) Info {
	pkg := ""
	if id, ok := sel.X.(*ast.Ident); ok {
		pkg = id.Name
	}
	name := sel.Sel.Name
	if pkg == "atomic" {
		switch name {
		case "Bool", "Int32", "Uint32":
			return Info{Size: 4, Align: 4}
		case "Int64", "Uint64":
			// 64-bit atomics stay 8-byte aligned even on 32-bit.
			return Info{Size: 8, Align: 8}
		case "Uintptr", "Pointer":
			// Pointer-sized; Align matches uintptr (not forced to 8).
			return s.ptr()
		case "Value":
			// atomic.Value is an interface (any).
			return Info{Size: 2 * s.PtrSize, Align: s.PtrSize}
		case "Int", "Uint":
			return s.ptr()
		}
	}
	if pkg == "unsafe" && name == "Pointer" {
		return s.ptr()
	}
	if pkg == "sync" {
		switch name {
		case "Mutex":
			// state int32 + sema uint32
			return Info{Size: 8, Align: 4}
		case "RWMutex":
			// w Mutex + reader/writer semaphores (amd64/arm64 layout ≈ 24 bytes)
			return Info{Size: 24, Align: 4}
		}
	}
	// Opt-in --packages mode fills locals with "pkg.Type" keys.
	if locals != nil && pkg != "" {
		if info, ok := locals[pkg+"."+name]; ok {
			return info
		}
	}
	// pkg.Type without go/types — unknown.
	return Unknown
}

func (s Sizer) structInfo(st *ast.StructType, locals map[string]Info) Info {
	if st.Fields == nil || len(st.Fields.List) == 0 {
		return Info{Size: 0, Align: 1}
	}
	total := 0
	wasted := 0
	maxAlign := 1
	lastSize := 0
	fieldCount := 0
	for _, f := range st.Fields.List {
		info := s.TypeInfo(f.Type, locals)
		if info.IsUnknown() {
			return Unknown
		}
		if info.Align > maxAlign {
			maxAlign = info.Align
		}
		n := fieldNameCount(f)
		for i := 0; i < n; i++ {
			var ok bool
			total, wasted, _, ok = alignmath.AddField(total, wasted, info.Size, info.Align)
			if !ok {
				return Unknown
			}
			lastSize = info.Size
			fieldCount++
		}
	}
	var ok bool
	total, _, maxAlign, ok = alignmath.Finish(total, wasted, maxAlign, lastSize, fieldCount)
	if !ok {
		return Unknown
	}
	return Info{Size: total, Align: maxAlign}
}

// CollectLocals resolves same-file defined types (aliases and defined types).
// Iterates to a fixpoint so order of declarations does not matter.
func (s Sizer) CollectLocals(file *ast.File) map[string]Info {
	sizes, _ := s.CollectLocalsFull(file)
	return sizes
}

// CollectLocalsFull is CollectLocals plus flag inheritance for defined/alias types.
func (s Sizer) CollectLocalsFull(file *ast.File) (map[string]Info, map[string]FieldFlags) {
	if file == nil {
		return nil, nil
	}
	specs := make([]*ast.TypeSpec, 0)
	for _, decl := range file.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.TYPE {
			continue
		}
		for _, spec := range gd.Specs {
			if ts, ok := spec.(*ast.TypeSpec); ok {
				specs = append(specs, ts)
			}
		}
	}
	if len(specs) == 0 {
		return nil, nil
	}

	locals := make(map[string]Info, len(specs))
	for pass := 0; pass < len(specs)+2; pass++ {
		progress := false
		for _, ts := range specs {
			if _, done := locals[ts.Name.Name]; done {
				continue
			}
			info := s.TypeInfo(ts.Type, locals)
			if info.IsUnknown() {
				continue
			}
			locals[ts.Name.Name] = info
			progress = true
		}
		if !progress {
			break
		}
	}

	flags := make(map[string]FieldFlags, len(specs))
	for pass := 0; pass < len(specs)+2; pass++ {
		progress := false
		for _, ts := range specs {
			if _, done := flags[ts.Name.Name]; done {
				continue
			}
			if _, ok := locals[ts.Name.Name]; !ok {
				continue
			}
			f := classifyFlags(ts.Type, locals[ts.Name.Name])
			if f == 0 {
				if id, ok := ts.Type.(*ast.Ident); ok {
					if uf, ok := flags[id.Name]; ok {
						f = uf
					}
				}
			}
			if f != 0 {
				flags[ts.Name.Name] = f
				progress = true
			}
		}
		if !progress {
			break
		}
	}
	if len(flags) == 0 {
		flags = nil
	}
	return locals, flags
}

func fieldNameCount(f *ast.Field) int {
	if len(f.Names) == 0 {
		return 1
	}
	return len(f.Names)
}

func evalInt(expr ast.Expr) (int, bool) {
	switch e := expr.(type) {
	case *ast.BasicLit:
		if e.Kind != token.INT {
			return 0, false
		}
		n, err := strconv.ParseInt(e.Value, 0, 0)
		if err != nil || n < 0 {
			return 0, false
		}
		return int(n), true
	case *ast.ParenExpr:
		return evalInt(e.X)
	case *ast.UnaryExpr:
		v, ok := evalInt(e.X)
		if !ok {
			return 0, false
		}
		switch e.Op {
		case token.ADD:
			return v, true
		case token.SUB:
			if v < 0 {
				return 0, false
			}
			return -v, true
		case token.XOR:
			return ^v, true
		default:
			return 0, false
		}
	case *ast.BinaryExpr:
		l, ok1 := evalInt(e.X)
		r, ok2 := evalInt(e.Y)
		if !ok1 || !ok2 {
			return 0, false
		}
		switch e.Op {
		case token.ADD:
			sum, carry := bits.Add(uint(l), uint(r), 0)
			if carry != 0 {
				return 0, false
			}
			return int(sum), true
		case token.SUB:
			diff, borrow := bits.Sub(uint(l), uint(r), 0)
			if borrow != 0 {
				return 0, false
			}
			return int(diff), true
		case token.MUL:
			hi, lo := bits.Mul(uint(l), uint(r))
			if hi != 0 {
				return 0, false
			}
			return int(lo), true
		case token.QUO:
			if r == 0 {
				return 0, false
			}
			return l / r, true
		case token.REM:
			if r == 0 {
				return 0, false
			}
			return l % r, true
		case token.SHL:
			if r < 0 || r >= bits.UintSize {
				return 0, false
			}
			if l != 0 && r >= bits.LeadingZeros(uint(l)) {
				return 0, false
			}
			return l << uint(r), true
		case token.SHR:
			if r < 0 || r >= bits.UintSize {
				return 0, false
			}
			return l >> uint(r), true
		case token.AND:
			return l & r, true
		case token.OR:
			return l | r, true
		case token.XOR:
			return l ^ r, true
		default:
			return 0, false
		}
	case *ast.Ident:
		return 0, false
	default:
		return 0, false
	}
}
