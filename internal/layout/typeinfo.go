package layout

import (
	"go/ast"
	"go/token"
	"strconv"
)

// TypeInfo returns size and alignment for an AST type expression without
// allocating type-name strings. Named/unknown types default to a pointer.
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
		n := arrayLen(t.Len)
		elem := s.TypeInfo(t.Elt)
		if n < 0 || elem.Size < 0 {
			return s.ptr()
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
	// sync/atomic types and common packages.
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
	if st.Fields == nil {
		return Info{Size: 0, Align: 1}
	}
	total := 0
	maxAlign := 1
	for _, f := range st.Fields.List {
		info := s.TypeInfo(f.Type)
		if info.Align > maxAlign {
			maxAlign = info.Align
		}
		n := fieldNameCount(f)
		for i := 0; i < n; i++ {
			pad := alignPad(total, info.Align)
			total += pad + info.Size
		}
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

func arrayLen(expr ast.Expr) int {
	switch e := expr.(type) {
	case *ast.BasicLit:
		if e.Kind == token.INT {
			n, err := strconv.Atoi(e.Value)
			if err == nil {
				return n
			}
		}
	case *ast.Ident:
		// named const — unknown without types; treat as 1
		return 1
	case *ast.ParenExpr:
		return arrayLen(e.X)
	}
	return -1
}

func alignPad(offset, align int) int {
	if align <= 1 {
		return 0
	}
	return (align - (offset % align)) % align
}
