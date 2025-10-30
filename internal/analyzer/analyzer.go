package analyzer

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"sort"
	"strings"
)

// Result represents the analysis result for a file
type Result struct {
	File   string  `json:"file"`
	Issues []Issue `json:"issues"`
}

// Issue represents a struct alignment issue
type Issue struct {
	StructName string  `json:"struct_name"`
	Line       int     `json:"line"`
	Message    string  `json:"message"`
	Severity   string  `json:"severity"`
	Wasted     int     `json:"wasted_bytes"`
	Fields     []Field `json:"fields"`
}

// Field represents a struct field
type Field struct {
	Name   string `json:"name"`
	Type   string `json:"type"`
	Size   int    `json:"size"`
	Offset int    `json:"offset"`
	Align  int    `json:"align"`
}

// AnalyzeFile analyzes a Go file for struct alignment issues
func AnalyzeFile(filename string) (Result, error) {
	result := Result{
		File: filename,
	}

	content, err := os.ReadFile(filename)
	if err != nil {
		return result, err
	}
	lines := strings.Split(string(content), "\n")

	fileSet := token.NewFileSet()
	node, err := parser.ParseFile(fileSet, filename, nil, parser.ParseComments)
	if err != nil {
		return result, err
	}

	ast.Inspect(node, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.TypeSpec:
			if structType, ok := x.Type.(*ast.StructType); ok {
				issue := analyzeStruct(fileSet, lines, x.Name.Name, structType, x)
				if issue != nil {
					result.Issues = append(result.Issues, *issue)
				}
			}
		}
		return true
	})

	return result, nil
}

func analyzeStruct(fset *token.FileSet, lines []string, name string, structType *ast.StructType, typeSpec *ast.TypeSpec) *Issue {
	if hasIgnoreComment(fset, lines, typeSpec) {
		return nil
	}

	fields := make([]Field, 0)
	totalSize := 0
	wastedBytes := 0

	for _, field := range structType.Fields.List {
		for _, fieldName := range field.Names {
			fieldType := getTypeString(field.Type)
			size := getTypeSize(fieldType)
			align := getTypeAlign(fieldType)

			padding := (align - (totalSize % align)) % align
			wastedBytes += padding
			offset := totalSize + padding

			fields = append(fields, Field{
				Name:   fieldName.Name,
				Type:   fieldType,
				Size:   size,
				Offset: offset,
				Align:  align,
			})

			totalSize = offset + size
		}
	}

	if wastedBytes > 0 {
		line := fset.Position(typeSpec.Pos()).Line
		severity := getSeverity(wastedBytes)

		return &Issue{
			StructName: name,
			Line:       line,
			Message:    fmt.Sprintf("Struct '%s' has %d bytes of padding (%d%% waste)", name, wastedBytes, (wastedBytes*100)/totalSize),
			Severity:   severity,
			Wasted:     wastedBytes,
			Fields:     fields,
		}
	}

	return nil
}

func hasIgnoreComment(fset *token.FileSet, lines []string, typeSpec *ast.TypeSpec) bool {
	if typeSpec.Doc != nil {
		for _, comment := range typeSpec.Doc.List {
			if strings.Contains(comment.Text, "goalign:ignore") {
				return true
			}
		}
	}

	pos := typeSpec.Pos()
	file := fset.File(pos)
	if file == nil {
		return false
	}

	line := file.Line(pos)
	if line <= 1 {
		return false
	}

	prevLine := line - 1
	if prevLine > 0 && prevLine <= len(lines) {
		lineContent := strings.TrimSpace(lines[prevLine-1])
		if strings.Contains(lineContent, "goalign:ignore") {
			return true
		}
	}

	return false
}

func getTypeString(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return "*" + getTypeString(t.X)
	case *ast.ArrayType:
		if t.Len == nil {
			return "[]" + getTypeString(t.Elt)
		}
		return fmt.Sprintf("[%s]%s", getTypeString(t.Len), getTypeString(t.Elt))
	case *ast.SelectorExpr:
		return getTypeString(t.X) + "." + t.Sel.Name
	case *ast.MapType:
		return fmt.Sprintf("map[%s]%s", getTypeString(t.Key), getTypeString(t.Value))
	case *ast.ChanType:
		return "chan " + getTypeString(t.Value)
	case *ast.FuncType:
		return "func"
	case *ast.InterfaceType:
		return "interface{}"
	default:
		return "unknown"
	}
}

func getTypeSize(typeStr string) int {
	switch typeStr {
	case "bool":
		return 1
	case "int8", "uint8", "byte":
		return 1
	case "int16", "uint16":
		return 2
	case "int32", "uint32", "rune", "float32":
		return 4
	case "int64", "uint64", "float64", "complex64":
		return 8
	case "complex128":
		return 16
	case "int", "uint", "uintptr":
		return 8
	case "string":
		return 16
	case "interface{}", "any":
		return 16
	default:
		if strings.HasPrefix(typeStr, "*") {
			return 8 // pointer
		}
		if strings.HasPrefix(typeStr, "[]") {
			return 24 // slice header
		}
		if strings.HasPrefix(typeStr, "map[") {
			return 8 // map pointer
		}
		if strings.HasPrefix(typeStr, "chan ") {
			return 8 // channel pointer
		}
		if strings.HasPrefix(typeStr, "func") {
			return 8 // function pointer
		}
		return 8
	}
}

func getTypeAlign(typeStr string) int {
	switch typeStr {
	case "bool", "int8", "uint8", "byte":
		return 1
	case "int16", "uint16":
		return 2
	case "int32", "uint32", "rune", "float32":
		return 4
	case "int", "uint", "uintptr", "int64", "uint64", "float64", "complex64":
		return 8
	case "complex128":
		return 8
	case "string":
		return 8
	case "interface{}", "any":
		return 8
	default:
		return 8
	}
}

func getSeverity(wastedBytes int) string {
	if wastedBytes >= 16 {
		return "high"
	} else if wastedBytes >= 8 {
		return "medium"
	}
	return "low"
}

// SuggestOptimization suggests field reordering to minimize padding
func SuggestOptimization(fields []Field) []Field {
	sorted := make([]Field, len(fields))
	copy(sorted, fields)

	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Align != sorted[j].Align {
			return sorted[i].Align > sorted[j].Align
		}
		return sorted[i].Size > sorted[j].Size
	})

	totalSize := 0
	for i := range sorted {
		padding := (sorted[i].Align - (totalSize % sorted[i].Align)) % sorted[i].Align
		sorted[i].Offset = totalSize + padding
		totalSize = sorted[i].Offset + sorted[i].Size
	}

	return sorted
}
