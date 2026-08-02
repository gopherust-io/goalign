package layout

import (
	"go/ast"
	"go/token"

	"github.com/gopherust-io/goalign/internal/alignmath"
)

// Result is the layout computation output for one struct.
type Result struct {
	N        int  // number of fields written to dst
	Total    int  // total size including trailing padding
	Wasted   int  // inter-field + trailing padding bytes
	MaxAlign int  // struct alignment
	Unknown  bool // true if a field type/size could not be resolved
}

// Compute lays out fields into dst without allocating when dst has capacity.
// locals is an optional same-file type map from CollectLocals (may be nil).
// localFlags, when provided, supplies bool/atomic/pointer flags for named aliases.
// If any field type is Unknown, Unknown is set and metrics should not be trusted for reporting.
func (s Sizer) Compute(dst []Field, fields *ast.FieldList, locals map[string]Info, localFlags ...map[string]FieldFlags) (Result, []Field) {
	if fields == nil {
		return Result{MaxAlign: 1}, dst[:0]
	}
	var flagMap map[string]FieldFlags
	if len(localFlags) > 0 {
		flagMap = localFlags[0]
	}

	n := 0
	total := 0
	wasted := 0
	maxAlign := 1
	lastSize := 0

	for _, f := range fields.List {
		info := s.TypeInfo(f.Type, locals)
		if info.IsUnknown() {
			return Result{Unknown: true, MaxAlign: 1}, dst[:0]
		}
		if info.Align > maxAlign {
			maxAlign = info.Align
		}
		flags := classifyFlags(f.Type, info)
		if flags == 0 {
			if id, ok := f.Type.(*ast.Ident); ok && flagMap != nil {
				flags = flagMap[id.Name]
			}
		}

		if len(f.Names) == 0 {
			var ok bool
			n, total, wasted, dst, ok = appendField(dst, n, total, wasted, embedName(f.Type), info, flags)
			if !ok {
				return Result{Unknown: true, MaxAlign: 1}, dst[:0]
			}
			lastSize = info.Size
			continue
		}
		for _, name := range f.Names {
			var ok bool
			n, total, wasted, dst, ok = appendField(dst, n, total, wasted, name.Name, info, flags)
			if !ok {
				return Result{Unknown: true, MaxAlign: 1}, dst[:0]
			}
			lastSize = info.Size
		}
	}

	// gc ABI: if the last field is zero-sized and the struct already has
	// non-zero size, add one byte so &lastField stays inside the object.
	// All-zero-sized structs stay size 0 (matches unsafe.Sizeof).
	var ok bool
	total, wasted, maxAlign, ok = alignmath.Finish(total, wasted, maxAlign, lastSize, n)
	if !ok {
		return Result{Unknown: true, MaxAlign: 1}, dst[:0]
	}

	return Result{N: n, Total: total, Wasted: wasted, MaxAlign: maxAlign}, dst[:n]
}

func appendField(dst []Field, n, total, wasted int, name string, info Info, flags FieldFlags) (int, int, int, []Field, bool) {
	total, wasted, offset, ok := alignmath.AddField(total, wasted, info.Size, info.Align)
	if !ok {
		return n, total, wasted, dst, false
	}
	f := Field{
		Name:   name,
		Size:   info.Size,
		Offset: offset,
		Align:  info.Align,
		Index:  n,
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
	return n, total, wasted, dst, true
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
			// Atomics-first packing only; Cacheguard contend requires atomic.*/sync
			// or an explicit // goalign:contend (avoids false-share spam on plain counters).
			flags |= FlagAtomic
		case "string", "any", "error":
			flags |= FlagPointer
		}
	case *ast.StarExpr:
		flags |= FlagPointer
	case *ast.ArrayType:
		if t.Len == nil {
			flags |= FlagPointer // slice header
		} else {
			flags |= classifyFlags(t.Elt, Info{}) & FlagPointer
		}
	case *ast.MapType, *ast.ChanType, *ast.FuncType, *ast.InterfaceType:
		flags |= FlagPointer
	case *ast.SelectorExpr:
		pkg := ""
		if id, ok := t.X.(*ast.Ident); ok {
			pkg = id.Name
		}
		if pkg == "atomic" {
			switch t.Sel.Name {
			case "Int64", "Uint64", "Uintptr", "Int", "Uint", "Pointer", "Value":
				flags |= FlagAtomic | FlagContend
			case "Bool", "Int32", "Uint32":
				flags |= FlagContend
			}
			if t.Sel.Name == "Bool" {
				flags |= FlagBool
			}
			if t.Sel.Name == "Pointer" || t.Sel.Name == "Value" {
				flags |= FlagPointer
			}
		}
		if pkg == "sync" {
			switch t.Sel.Name {
			case "Mutex", "RWMutex":
				flags |= FlagContend
			}
		}
		if pkg == "unsafe" && t.Sel.Name == "Pointer" {
			flags |= FlagPointer
		}
	case *ast.IndexExpr:
		return classifyFlags(t.X, Info{})
	case *ast.IndexListExpr:
		return classifyFlags(t.X, Info{})
	case *ast.ParenExpr:
		return classifyFlags(t.X, Info{})
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
