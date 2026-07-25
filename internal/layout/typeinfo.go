package layout

import (
	"go/ast"
	"go/token"
	"strconv"
)

// Unknown marks a type whose size cannot be determined from the AST alone.
var Unknown = Info{Size: -1, Align: -1}

// IsUnknown reports whether size/align could not be resolved.
func (i Info) IsUnknown() bool { return i.Size < 0 }

// TypeInfo returns size and alignment for an AST type expression without
// allocating type-name strings. Named/unknown types default to a pointer.
// Unresolvable array lengths return Unknown (Size < 0).
func (s Sizer) TypeInfo(expr ast.Expr) Info {
	if expr == nil {
		return s.ptr()
	}
	switch t := expr.(type) {
	case *ast.Ident:
		return s.identInfo(t.Name)
	case *ast.StarExpr:
		return s.ptr()
	case *ast.ArrayType:
		if t.Len == nil {
			// slice header: ptr + len + cap
			return Info{Size: 3 * s.PtrSize, Align: s.PtrSize}
		}
		n, ok := evalInt(t.Len)
		elem := s.TypeInfo(t.Elt)
		if !ok || elem.IsUnknown() {
			return Unknown
		}
		if n == 0 {
			// [0]T has size 0 but keeps element alignment.
			align := elem.Align
			if align < 1 {
				align = 1
			}
			return Info{Size: 0, Align: align}
		}
		return Info{Size: n * elem.Size, Align: elem.Align}
	case *ast.StructType:
		return s.structInfo(t)
	case *ast.SelectorExpr:
		return s.selectorInfo(t)
	case *ast.MapType, *ast.ChanType, *ast.FuncType:
		return s.ptr()
	case *ast.InterfaceType:
		// iface: type + data
		return Info{Size: 2 * s.PtrSize, Align: s.PtrSize}
	case *ast.Ellipsis:
		return s.TypeInfo(t.Elt)
	case *ast.ParenExpr:
		return s.TypeInfo(t.X)
	default:
		return s.ptr()
	}
}

func (s Sizer) identInfo(name string) Info {
	switch name {
	case "bool", "int8", "uint8", "byte":
		return Info{Size: 1, Align: 1}
	case "int16", "uint16":
		return Info{Size: 2, Align: 2}
	case "int32", "uint32", "rune", "float32":
		return Info{Size: 4, Align: 4}
	case "int64", "uint64", "float64":
		return Info{Size: 8, Align: 8}
	case "complex64":
		return Info{Size: 8, Align: 4}
	case "complex128":
		return Info{Size: 16, Align: 8}
	case "int", "uint", "uintptr":
		return s.ptr()
	case "string":
		return Info{Size: 2 * s.PtrSize, Align: s.PtrSize}
	case "any", "error":
		return Info{Size: 2 * s.PtrSize, Align: s.PtrSize}
	default:
		// Named types: assume pointer-sized (heuristic without go/types).
		return s.ptr()
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
			return Info{Size: 8, Align: 8}
		case "Int", "Uint":
			return s.ptr()
		}
	}
	return s.ptr()
}

func (s Sizer) structInfo(st *ast.StructType) Info {
	if st.Fields == nil || len(st.Fields.List) == 0 {
		return Info{Size: 0, Align: 1}
	}
	total := 0
	maxAlign := 1
	lastSize := 0
	for _, f := range st.Fields.List {
		info := s.TypeInfo(f.Type)
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
	// gc ABI: trailing zero-sized field in a non-empty struct.
	if lastSize == 0 && total > 0 {
		total++
	}
	total += alignPad(total, maxAlign)
	if maxAlign < 1 {
		maxAlign = 1
	}
	return Info{Size: total, Align: maxAlign}
}

func fieldNameCount(f *ast.Field) int {
	if len(f.Names) == 0 {
		return 1 // embedded field
	}
	return len(f.Names)
}

// evalInt evaluates a simple integer constant expression used as an array length.
// Returns ok=false when the expression cannot be resolved without go/types.
func evalInt(expr ast.Expr) (int, bool) {
	switch e := expr.(type) {
	case *ast.BasicLit:
		if e.Kind != token.INT {
			return 0, false
		}
		n, err := strconv.ParseInt(e.Value, 0, 64)
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
		// Named const — unknown without types.
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
