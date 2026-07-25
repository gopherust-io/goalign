package layout

import (
	"go/ast"
	"go/token"
	"math/bits"
	"strconv"
)

// Unknown marks a type whose size cannot be determined from the AST alone.
var Unknown = Info{Size: -1, Align: -1}

// IsUnknown reports whether size/align could not be resolved.
func (i Info) IsUnknown() bool { return i.Size < 0 }

// ValidArch reports whether goarch is a known GOARCH for SizerFor.
func ValidArch(goarch string) bool {
	switch goarch {
	case "386", "amd64", "arm", "arm64", "mips", "mipsle", "mips64", "mips64le",
		"ppc", "ppc64", "ppc64le", "riscv", "riscv64", "s390x", "wasm":
		return true
	default:
		return false
	}
}

// TypeInfo returns size and alignment for an AST type expression.
// locals maps same-file type names to resolved Info (may be nil).
// Unresolvable array lengths and unknown named types return Unknown.
func (s Sizer) TypeInfo(expr ast.Expr, locals map[string]Info) Info {
	if expr == nil {
		return s.ptr()
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
		return s.selectorInfo(t)
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

func (s Sizer) selectorInfo(sel *ast.SelectorExpr) Info {
	pkg := ""
	if id, ok := sel.X.(*ast.Ident); ok {
		pkg = id.Name
	}
	name := sel.Sel.Name
	if pkg == "atomic" {
		switch name {
		case "Bool", "Int32", "Uint32":
			return Info{Size: 4, Align: 4}
		case "Int64", "Uint64", "Uintptr", "Pointer", "Value":
			// Atomics stay 8-byte aligned even on 32-bit (nats convention).
			return Info{Size: 8, Align: 8}
		case "Int", "Uint":
			return s.ptr()
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
	maxAlign := 1
	lastSize := 0
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
			pad := alignPad(total, info.Align)
			total += pad + info.Size
			lastSize = info.Size
		}
	}
	if lastSize == 0 && total > 0 {
		total++
	}
	total += alignPad(total, maxAlign)
	if maxAlign < 1 {
		maxAlign = 1
	}
	return Info{Size: total, Align: maxAlign}
}

// CollectLocals resolves same-file defined types (aliases and defined types).
// Iterates to a fixpoint so order of declarations does not matter.
func (s Sizer) CollectLocals(file *ast.File) map[string]Info {
	if file == nil {
		return nil
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
		return nil
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
	return locals
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
			return l + r, true
		case token.SUB:
			return l - r, true
		case token.MUL:
			return l * r, true
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
			if r < 0 || r > 62 {
				return 0, false
			}
			return l << uint(r), true
		case token.SHR:
			if r < 0 || r > 62 {
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

func alignPad(offset, align int) int {
	if align <= 1 {
		return 0
	}
	return (align - (offset % align)) % align
}
