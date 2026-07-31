package fixer

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"sort"
	"strings"

	"github.com/gopherust-io/goalign/internal/analyzer"
	"github.com/gopherust-io/goalign/internal/layout"
)

// FileResult is the outcome of rewriting one file.
type FileResult struct {
	File       string
	Structs    []string
	BytesSaved int
	Changed    bool
}

// ShouldFix reports whether an issue should be applied by fix.
func ShouldFix(iss analyzer.Issue) bool {
	if len(iss.Suggested) == 0 || sameOrder(iss.Fields, iss.Suggested) {
		return false
	}
	if iss.Saved > 0 {
		return true
	}
	for _, n := range iss.Notes {
		if strings.HasPrefix(n, "atomics-first") {
			return true
		}
	}
	return false
}

func sameOrder(a, b []layout.Field) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Name != b[i].Name {
			return false
		}
	}
	return true
}

type textEdit struct {
	new   string
	start int
	end   int
}

// FixFile rewrites content applying suggested field orders for fixable issues.
// Returns formatted source, number of structs fixed, and bytes saved.
func FixFile(filename string, content []byte, issues []analyzer.Issue) ([]byte, int, int, error) {
	fixable := make([]analyzer.Issue, 0, len(issues))
	for _, iss := range issues {
		if ShouldFix(iss) {
			fixable = append(fixable, iss)
		}
	}
	if len(fixable) == 0 {
		return content, 0, 0, nil
	}

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, content, parser.ParseComments)
	if err != nil {
		return nil, 0, 0, err
	}

	edits := make([]textEdit, 0, len(fixable))
	nFixed := 0
	bytesSaved := 0
	for _, iss := range fixable {
		st, ok := findStruct(fset, file, iss.StructName, iss.Line)
		if !ok {
			return nil, 0, 0, fmt.Errorf("%s: struct %s at line %d not found", filename, iss.StructName, iss.Line)
		}
		edit, err := structBodyEdit(fset, content, st, iss.Suggested)
		if err != nil {
			return nil, 0, 0, fmt.Errorf("%s: %s: %w", filename, iss.StructName, err)
		}
		edits = append(edits, edit)
		nFixed++
		bytesSaved += iss.Saved
	}

	// Apply back-to-front so offsets stay valid.
	sort.Slice(edits, func(i, j int) bool { return edits[i].start > edits[j].start })
	out := content
	for _, e := range edits {
		var b strings.Builder
		b.Grow(len(out) - (e.end - e.start) + len(e.new))
		b.Write(out[:e.start])
		b.WriteString(e.new)
		b.Write(out[e.end:])
		out = []byte(b.String())
	}

	formatted, err := format.Source(out)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("format after fix: %w", err)
	}
	return formatted, nFixed, bytesSaved, nil
}

// FixPath reads, rewrites, and writes a Go file when issues are fixable.
func FixPath(filename string, issues []analyzer.Issue) (FileResult, error) {
	res := FileResult{File: filename}
	fixable := make([]analyzer.Issue, 0, len(issues))
	names := make([]string, 0, len(issues))
	for _, iss := range issues {
		if ShouldFix(iss) {
			fixable = append(fixable, iss)
			names = append(names, iss.StructName)
		}
	}
	if len(fixable) == 0 {
		return res, nil
	}

	content, err := os.ReadFile(filename)
	if err != nil {
		return res, err
	}
	mode := os.FileMode(0o644)
	if fi, statErr := os.Stat(filename); statErr == nil {
		mode = fi.Mode().Perm()
	}
	out, nFixed, saved, err := FixFile(filename, content, fixable)
	if err != nil {
		return res, err
	}
	if nFixed == 0 || bytes.Equal(content, out) {
		return res, nil
	}
	if err := os.WriteFile(filename, out, mode); err != nil {
		return res, err
	}
	res.Changed = true
	res.Structs = names[:nFixed]
	res.BytesSaved = saved
	return res, nil
}

func findStruct(fset *token.FileSet, file *ast.File, name string, line int) (*ast.StructType, bool) {
	var found *ast.StructType
	ast.Inspect(file, func(n ast.Node) bool {
		if found != nil {
			return false
		}
		ts, ok := n.(*ast.TypeSpec)
		if !ok || ts.Name == nil || ts.Name.Name != name {
			return true
		}
		st, ok := ts.Type.(*ast.StructType)
		if !ok {
			return true
		}
		if fset.Position(ts.Pos()).Line == line {
			found = st
			return false
		}
		return true
	})
	return found, found != nil
}

type fieldSlot struct {
	field *ast.Field
	name  *ast.Ident // nil for embedded
}

func structBodyEdit(fset *token.FileSet, content []byte, st *ast.StructType, suggested []layout.Field) (textEdit, error) {
	if st.Fields == nil {
		return textEdit{}, fmt.Errorf("nil field list")
	}

	slots := make([]fieldSlot, 0, len(st.Fields.List))
	byName := make(map[string]int, len(st.Fields.List))
	for _, f := range st.Fields.List {
		if len(f.Names) == 0 {
			name := embedName(f.Type)
			byName[name] = len(slots)
			slots = append(slots, fieldSlot{field: f, name: nil})
			continue
		}
		for _, id := range f.Names {
			byName[id.Name] = len(slots)
			slots = append(slots, fieldSlot{field: f, name: id})
		}
	}

	if len(suggested) != len(slots) {
		return textEdit{}, fmt.Errorf("suggested field count %d != struct field count %d", len(suggested), len(slots))
	}

	indent := detectIndent(fset, content, st)
	var body strings.Builder
	body.WriteByte('\n')
	used := make([]bool, len(slots))

	for i := 0; i < len(suggested); {
		name := suggested[i].Name
		idx, ok := byName[name]
		if !ok || used[idx] {
			return textEdit{}, fmt.Errorf("cannot map suggested field %q", name)
		}
		slot := slots[idx]
		used[idx] = true

		if slot.name == nil {
			body.WriteString(indent)
			body.WriteString(fieldText(fset, content, slot.field))
			body.WriteByte('\n')
			i++
			continue
		}

		orig := slot.field
		group := []*ast.Ident{slot.name}
		j := i + 1
		for j < len(suggested) {
			n2 := suggested[j].Name
			idx2, ok := byName[n2]
			if !ok || used[idx2] {
				break
			}
			s2 := slots[idx2]
			if s2.field != orig || s2.name == nil {
				break
			}
			if !namesInOrder(orig.Names, group[len(group)-1].Name, s2.name.Name) {
				break
			}
			used[idx2] = true
			group = append(group, s2.name)
			j++
		}

		body.WriteString(indent)
		if len(group) == len(orig.Names) && namesExact(orig.Names, group) {
			body.WriteString(fieldText(fset, content, orig))
		} else if len(group) == 1 && len(orig.Names) == 1 {
			body.WriteString(fieldText(fset, content, orig))
		} else {
			for gi, id := range group {
				if gi > 0 {
					body.WriteByte('\n')
					body.WriteString(indent)
				}
				body.WriteString(splitFieldText(fset, content, orig, id))
			}
		}
		body.WriteByte('\n')
		i = j
	}

	for _, u := range used {
		if !u {
			return textEdit{}, fmt.Errorf("not all fields consumed during reorder")
		}
	}

	open := fset.Position(st.Fields.Opening).Offset
	closeOff := fset.Position(st.Fields.Closing).Offset
	return textEdit{start: open + 1, end: closeOff, new: body.String()}, nil
}

func detectIndent(fset *token.FileSet, content []byte, st *ast.StructType) string {
	if st.Fields == nil || len(st.Fields.List) == 0 {
		return "\t"
	}
	pos := fset.Position(st.Fields.List[0].Pos())
	lineStart := pos.Offset - (pos.Column - 1)
	if lineStart < 0 {
		lineStart = 0
	}
	i := lineStart
	for i < pos.Offset && (content[i] == '\t' || content[i] == ' ') {
		i++
	}
	if i > lineStart {
		return string(content[lineStart:i])
	}
	return "\t"
}

func fieldText(fset *token.FileSet, content []byte, f *ast.Field) string {
	start := fset.Position(f.Pos()).Offset
	end := fset.Position(f.End()).Offset
	if f.Comment != nil {
		cend := fset.Position(f.Comment.End()).Offset
		if cend > end {
			end = cend
		}
	}
	// Include doc comment above the field when present.
	if f.Doc != nil {
		dstart := fset.Position(f.Doc.Pos()).Offset
		if dstart < start {
			start = dstart
		}
	}
	if start < 0 {
		start = 0
	}
	if end > len(content) {
		end = len(content)
	}
	text := string(content[start:end])
	return strings.TrimRight(text, "\n\r")
}

func splitFieldText(fset *token.FileSet, content []byte, orig *ast.Field, name *ast.Ident) string {
	typeStart := fset.Position(orig.Type.Pos()).Offset
	typeEnd := fset.Position(orig.Type.End()).Offset
	var b strings.Builder
	b.WriteString(name.Name)
	b.WriteByte(' ')
	b.Write(content[typeStart:typeEnd])
	if orig.Tag != nil {
		b.WriteByte(' ')
		tagStart := fset.Position(orig.Tag.Pos()).Offset
		tagEnd := fset.Position(orig.Tag.End()).Offset
		b.Write(content[tagStart:tagEnd])
	}
	return b.String()
}

func namesExact(orig []*ast.Ident, group []*ast.Ident) bool {
	if len(orig) != len(group) {
		return false
	}
	for i := range orig {
		if orig[i].Name != group[i].Name {
			return false
		}
	}
	return true
}

func namesInOrder(orig []*ast.Ident, a, b string) bool {
	ia, ib := -1, -1
	for i, id := range orig {
		if id.Name == a {
			ia = i
		}
		if id.Name == b {
			ib = i
		}
	}
	return ia >= 0 && ib >= 0 && ia < ib
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
