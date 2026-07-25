package layout

import (
	"go/ast"
	"go/token"
)

// Result is the layout computation output for one struct.
type Result struct {
	N        int // number of fields written to dst
	Total    int // total size including trailing padding
	Wasted   int // inter-field + trailing padding bytes
	MaxAlign int
}

// Compute lays out fields into dst without allocating when dst has capacity.
// Type display strings are left empty; call FillTypeNames if needed for output.
// Returns how many fields were written and layout metrics.
func (s Sizer) Compute(dst []Field, fields *ast.FieldList) (Result, []Field) {
	if fields == nil {
		return Result{MaxAlign: 1}, dst[:0]
	}

	n := 0
	total := 0
	wasted := 0
	maxAlign := 1

	for _, f := range fields.List {
		info := s.TypeInfo(f.Type)
		if info.Align > maxAlign {
			maxAlign = info.Align
		}
		flags := classifyFlags(f.Type, info)

		if len(f.Names) == 0 {
			// Embedded field — write once without allocating an Ident slice.
			n, total, wasted, dst = appendField(dst, n, total, wasted, embedName(f.Type), info, flags)
			continue
		}
		for _, name := range f.Names {
			n, total, wasted, dst = appendField(dst, n, total, wasted, name.Name, info, flags)
		}
	}

	trail := alignPad(total, maxAlign)
	wasted += trail
	total += trail
	if maxAlign < 1 {
		maxAlign = 1
	}

	return Result{N: n, Total: total, Wasted: wasted, MaxAlign: maxAlign}, dst[:n]
}

func appendField(dst []Field, n, total, wasted int, name string, info Info, flags FieldFlags) (int, int, int, []Field) {
	pad := alignPad(total, info.Align)
	wasted += pad
	offset := total + pad
	f := Field{
		Name:   name,
		Size:   info.Size,
		Offset: offset,
		Align:  info.Align,
		Flags:  flags,
	}
	if n < len(dst) {
		dst[n] = f
	} else if n < cap(dst) {
		dst = dst[:n+1]
		dst[n] = f
	} else {
		dst = append(dst, f)
	}
	n++
	total = offset + info.Size
	return n, total, wasted, dst
}

// FillTypeNames sets Field.Type display strings. Allocates; use only for reporting.
func FillTypeNames(fields []Field, list *ast.FieldList) {
	if list == nil {
		return
	}
	i := 0
	for _, f := range list.List {
		typeStr := typeString(f.Type)
		count := fieldNameCount(f)
		for j := 0; j < count && i < len(fields); j++ {
			fields[i].Type = typeStr
			i++
		}
	}
}

func classifyFlags(expr ast.Expr, _ Info) FieldFlags {
	var flags FieldFlags
	switch t := expr.(type) {
	case *ast.Ident:
		switch t.Name {
		case "bool":
			flags |= FlagBool
		case "int64", "uint64":
			flags |= FlagAtomic
		}
	case *ast.SelectorExpr:
		pkg := ""
		if id, ok := t.X.(*ast.Ident); ok {
			pkg = id.Name
		}
		if pkg == "atomic" {
			switch t.Sel.Name {
			case "Int64", "Uint64", "Uintptr", "Int", "Uint", "Pointer", "Value":
				flags |= FlagAtomic
			case "Bool":
				flags |= FlagBool
			}
		}
	}
	return flags
}

func embedName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return embedName(t.X)
	case *ast.SelectorExpr:
		return t.Sel.Name
	default:
		return "_"
	}
}

// typeString builds a display string (allocates). Used only for reports.
func typeString(expr ast.Expr) string {
	if expr == nil {
		return "unknown"
	}
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return "*" + typeString(t.X)
	case *ast.ArrayType:
		if t.Len == nil {
			return "[]" + typeString(t.Elt)
		}
		if lit, ok := t.Len.(*ast.BasicLit); ok && lit.Kind == token.INT {
			return "[" + lit.Value + "]" + typeString(t.Elt)
		}
		return "[?]" + typeString(t.Elt)
	case *ast.SelectorExpr:
		return typeString(t.X) + "." + t.Sel.Name
	case *ast.MapType:
		return "map[" + typeString(t.Key) + "]" + typeString(t.Value)
	case *ast.ChanType:
		return "chan " + typeString(t.Value)
	case *ast.FuncType:
		return "func"
	case *ast.InterfaceType:
		return "interface{}"
	case *ast.StructType:
		return "struct{}"
	case *ast.ParenExpr:
		return typeString(t.X)
	default:
		return "unknown"
	}
}
