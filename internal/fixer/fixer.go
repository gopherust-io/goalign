package fixer

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gopherust-io/goalign/internal/analyzer"
	"github.com/gopherust-io/goalign/internal/diff"
	"github.com/gopherust-io/goalign/internal/layout"
)

// FileResult is the outcome of rewriting one file.
type FileResult struct {
	File       string
	Structs    []string
	Diff       string // set when DiffOnly
	BytesSaved int
	Changed    bool
}

// Options controls fix behavior.
type Options struct {
	DiffOnly     bool // compute new source / diff but do not write
	RewriteBools bool // pack unexported bools into a flag word when safe
	Cacheguard   bool // allow Cacheguard pad inserts from Suggested
}

// ShouldFix reports whether an issue should be applied by fix.
func ShouldFix(iss analyzer.Issue) bool {
	return shouldFix(iss, false)
}

func shouldFix(iss analyzer.Issue, cacheguard bool) bool {
	if len(iss.Suggested) == 0 {
		return false
	}
	if sameOrder(iss.Fields, iss.Suggested) && !suggestedHasCachePads(iss.Suggested) {
		return false
	}
	if iss.Saved > 0 {
		return true
	}
	for _, n := range iss.Notes {
		if strings.HasPrefix(n, "atomics-first") {
			return true
		}
		if cacheguard && (strings.HasPrefix(n, "false-share") || strings.HasPrefix(n, "cacheguard")) {
			return true
		}
	}
	if cacheguard && suggestedHasCachePads(iss.Suggested) {
		return true
	}
	return false
}

func suggestedHasCachePads(fields []layout.Field) bool {
	for _, f := range fields {
		if f.IsCachePad() || layout.IsCachePadName(f.Name) {
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
		if a[i].Index != b[i].Index || a[i].Name != b[i].Name {
			return false
		}
	}
	return true
}

func fieldIgnoreNote(iss analyzer.Issue) bool {
	for _, n := range iss.Notes {
		if strings.HasPrefix(n, "field-ignore") {
			return true
		}
	}
	return false
}

func canRewriteBools(iss analyzer.Issue) bool {
	return len(iss.BoolPack) >= 3 && !fieldIgnoreNote(iss)
}

type textEdit struct {
	new   string
	start int
	end   int
}

// FixFile rewrites content applying suggested field orders for fixable issues.
// Returns formatted source, number of structs fixed, and bytes saved.
func FixFile(filename string, content []byte, issues []analyzer.Issue) ([]byte, int, int, error) {
	return FixFileWithOptions(filename, content, issues, Options{})
}

// FixFileWithOptions applies fixes with DiffOnly/RewriteBools options.
func FixFileWithOptions(filename string, content []byte, issues []analyzer.Issue, opts Options) ([]byte, int, int, error) {
	fixable := make([]analyzer.Issue, 0, len(issues))
	for _, iss := range issues {
		if shouldFix(iss, opts.Cacheguard) || (opts.RewriteBools && canRewriteBools(iss)) {
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
		var edit textEdit
		// Density/atomics/cacheguard reorder takes precedence; bool-pack only when
		// it is the sole applicable action (--rewrite-bools without a reorder fix).
		if shouldFix(iss, opts.Cacheguard) {
			edit, err = structBodyEdit(fset, content, st, iss.Suggested)
		} else {
			edit, err = structBoolPackEdit(fset, content, st, iss)
		}
		if err != nil {
			return nil, 0, 0, fmt.Errorf("%s: %s: %w", filename, iss.StructName, err)
		}
		edits = append(edits, edit)
		nFixed++
		bytesSaved += iss.Saved
	}

	// Apply front-to-back into one pre-sized buffer (edits must not overlap).
	sort.Slice(edits, func(i, j int) bool { return edits[i].start < edits[j].start })
	size := len(content)
	for _, e := range edits {
		size += len(e.new) - (e.end - e.start)
	}
	out := make([]byte, 0, size)
	pos := 0
	for _, e := range edits {
		out = append(out, content[pos:e.start]...)
		out = append(out, e.new...)
		pos = e.end
	}
	out = append(out, content[pos:]...)

	formatted, err := format.Source(out)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("format after fix: %w", err)
	}
	return formatted, nFixed, bytesSaved, nil
}

// FixPath reads, rewrites, and writes a Go file when issues are fixable.
func FixPath(filename string, issues []analyzer.Issue) (FileResult, error) {
	return FixPathWithOptions(filename, issues, Options{})
}

// FixPathWithOptions is FixPath with options.
func FixPathWithOptions(filename string, issues []analyzer.Issue, opts Options) (FileResult, error) {
	res := FileResult{File: filename}
	for _, iss := range issues {
		if shouldFix(iss, opts.Cacheguard) || (opts.RewriteBools && canRewriteBools(iss)) {
			content, err := os.ReadFile(filename)
			if err != nil {
				return res, err
			}
			return FixContentWithOptions(filename, content, issues, opts)
		}
	}
	return res, nil
}

// FixContent rewrites and writes using already-loaded source bytes (avoids a
// second disk read when paired with analyzer.Result.Content).
func FixContent(filename string, content []byte, issues []analyzer.Issue) (FileResult, error) {
	return FixContentWithOptions(filename, content, issues, Options{})
}

// FixContentWithOptions rewrites (or diffs) using options.
func FixContentWithOptions(filename string, content []byte, issues []analyzer.Issue, opts Options) (FileResult, error) {
	res := FileResult{File: filename}
	fixable := make([]analyzer.Issue, 0, len(issues))
	names := make([]string, 0, len(issues))
	for _, iss := range issues {
		if shouldFix(iss, opts.Cacheguard) || (opts.RewriteBools && canRewriteBools(iss)) {
			fixable = append(fixable, iss)
			names = append(names, iss.StructName)
		}
	}
	if len(fixable) == 0 {
		return res, nil
	}

	out, nFixed, saved, err := FixFileWithOptions(filename, content, fixable, opts)
	if err != nil {
		return res, err
	}
	if nFixed == 0 || bytes.Equal(content, out) {
		return res, nil
	}
	res.Changed = true
	res.Structs = names[:nFixed]
	res.BytesSaved = saved
	if opts.DiffOnly {
		res.Diff = diff.Unified(filename, content, out)
		return res, nil
	}
	mode := os.FileMode(0o644)
	if fi, statErr := os.Stat(filename); statErr == nil {
		mode = fi.Mode().Perm()
	}
	if err := writeFileAtomic(filename, out, mode); err != nil {
		return res, err
	}
	return res, nil
}

func writeFileAtomic(filename string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(filename)
	tmp, err := os.CreateTemp(dir, ".goalign-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, filename); err != nil {
		return err
	}
	cleanup = false
	return nil
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

func buildSlots(st *ast.StructType) []fieldSlot {
	slots := make([]fieldSlot, 0, len(st.Fields.List))
	for _, f := range st.Fields.List {
		if len(f.Names) == 0 {
			slots = append(slots, fieldSlot{field: f, name: nil})
			continue
		}
		for _, id := range f.Names {
			slots = append(slots, fieldSlot{field: f, name: id})
		}
	}
	return slots
}

func structBodyEdit(fset *token.FileSet, content []byte, st *ast.StructType, suggested []layout.Field) (textEdit, error) {
	if st.Fields == nil {
		return textEdit{}, fmt.Errorf("nil field list")
	}

	slots := buildSlots(st)
	// Drop existing Cacheguard pads from slots so re-apply is idempotent.
	realSlots := make([]fieldSlot, 0, len(slots))
	for _, slot := range slots {
		name := "_"
		if slot.name != nil {
			name = slot.name.Name
		} else {
			name = embedName(slot.field.Type)
		}
		if layout.IsCachePadName(name) {
			continue
		}
		realSlots = append(realSlots, slot)
	}
	slots = realSlots

	realSuggested := 0
	for _, f := range suggested {
		if !f.IsCachePad() && !layout.IsCachePadName(f.Name) {
			realSuggested++
		}
	}
	if realSuggested != len(slots) {
		return textEdit{}, fmt.Errorf("suggested field count %d != struct field count %d", realSuggested, len(slots))
	}

	byIndex := indexMappingOK(suggested, len(slots))
	byName := make(map[string]int, len(slots))
	if !byIndex {
		for i, slot := range slots {
			name := "_"
			if slot.name != nil {
				name = slot.name.Name
			} else {
				name = embedName(slot.field.Type)
			}
			// Last-wins for duplicate names; prefer Index mapping when available.
			byName[name] = i
		}
	}

	mapIdx := func(f layout.Field) (int, bool) {
		if f.IsCachePad() || layout.IsCachePadName(f.Name) {
			return -1, false
		}
		if byIndex {
			if f.Index < 0 || f.Index >= len(slots) {
				return 0, false
			}
			return f.Index, true
		}
		idx, ok := byName[f.Name]
		return idx, ok
	}

	indent := detectIndent(fset, content, st)
	var body strings.Builder
	body.WriteByte('\n')
	used := make([]bool, len(slots))

	for i := 0; i < len(suggested); {
		f := suggested[i]
		if f.IsCachePad() || layout.IsCachePadName(f.Name) {
			body.WriteString(indent)
			body.WriteString(cachePadDecl(f))
			body.WriteByte('\n')
			i++
			continue
		}
		idx, ok := mapIdx(f)
		if !ok || used[idx] {
			return textEdit{}, fmt.Errorf("cannot map suggested field %q (index %d)", f.Name, f.Index)
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
			if suggested[j].IsCachePad() || layout.IsCachePadName(suggested[j].Name) {
				break
			}
			idx2, ok := mapIdx(suggested[j])
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
				body.WriteString(splitFieldText(fset, content, orig, id, gi == 0, gi == len(group)-1))
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

// indexMappingOK reports whether suggested fields carry a complete unique Index set.
func indexMappingOK(suggested []layout.Field, nslots int) bool {
	if nslots == 0 {
		return false
	}
	seen := make([]bool, nslots)
	n := 0
	for _, f := range suggested {
		if f.IsCachePad() || layout.IsCachePadName(f.Name) {
			continue
		}
		if f.Index < 0 || f.Index >= nslots || seen[f.Index] {
			return false
		}
		seen[f.Index] = true
		n++
	}
	return n == nslots
}

func cachePadDecl(f layout.Field) string {
	typ := f.Type
	if typ == "" {
		typ = fmt.Sprintf("[%d]byte", f.Size)
	}
	return f.Name + " " + typ + " // goalign:cacheguard — separate contended fields"
}

func slotKey(f layout.Field, useIdx bool, byName map[string]int) int {
	if useIdx {
		return f.Index
	}
	return byName[f.Name]
}

func resolveSlot(f layout.Field, useIdx bool, byName map[string]int, nslots int) (int, bool) {
	if useIdx {
		if f.Index < 0 || f.Index >= nslots {
			return 0, false
		}
		return f.Index, true
	}
	idx, ok := byName[f.Name]
	return idx, ok
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

func splitFieldText(fset *token.FileSet, content []byte, orig *ast.Field, name *ast.Ident, withDoc, withComment bool) string {
	var b strings.Builder
	if withDoc && orig.Doc != nil {
		dstart := fset.Position(orig.Doc.Pos()).Offset
		dend := fset.Position(orig.Doc.End()).Offset
		if dstart >= 0 && dend <= len(content) && dstart < dend {
			b.Write(content[dstart:dend])
			if dend < len(content) && content[dend-1] != '\n' {
				b.WriteByte('\n')
			}
		}
	}
	typeStart := fset.Position(orig.Type.Pos()).Offset
	typeEnd := fset.Position(orig.Type.End()).Offset
	b.WriteString(name.Name)
	b.WriteByte(' ')
	b.Write(content[typeStart:typeEnd])
	if orig.Tag != nil {
		b.WriteByte(' ')
		tagStart := fset.Position(orig.Tag.Pos()).Offset
		tagEnd := fset.Position(orig.Tag.End()).Offset
		b.Write(content[tagStart:tagEnd])
	}
	if withComment && orig.Comment != nil {
		b.WriteByte(' ')
		cstart := fset.Position(orig.Comment.Pos()).Offset
		cend := fset.Position(orig.Comment.End()).Offset
		if cstart >= 0 && cend <= len(content) && cstart < cend {
			b.Write(bytes.TrimSpace(content[cstart:cend]))
		}
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

// structBoolPackEdit replaces unexported bool fields listed in iss.BoolPack
// with a single flags word, then applies the suggested order for remaining fields.
// Callers must update field accesses; this is an opt-in breaking rewrite.
func structBoolPackEdit(fset *token.FileSet, content []byte, st *ast.StructType, iss analyzer.Issue) (textEdit, error) {
	if fieldIgnoreNote(iss) {
		return textEdit{}, fmt.Errorf("bool-pack refused: per-field // goalign:ignore present")
	}
	packSet := make(map[string]int, len(iss.BoolPack))
	for i, n := range iss.BoolPack {
		packSet[n] = i
	}
	if len(packSet) < 3 {
		return textEdit{}, fmt.Errorf("bool-pack needs >= 3 unexported bools")
	}

	flagType := "uint8"
	if len(packSet) > 8 {
		flagType = "uint16"
	}
	if len(packSet) > 16 {
		flagType = "uint32"
	}
	if len(packSet) > 32 {
		return textEdit{}, fmt.Errorf("bool-pack supports at most 32 bools")
	}

	slots := buildSlots(st)
	byName := make(map[string]int, len(slots))
	for i, slot := range slots {
		name := ""
		if slot.name != nil {
			name = slot.name.Name
		} else {
			name = embedName(slot.field.Type)
		}
		if name == "flags" {
			return textEdit{}, fmt.Errorf("bool-pack refused: field %q already exists", "flags")
		}
		byName[name] = i
	}
	useIdx := indexMappingOK(iss.Fields, len(slots))

	// Build suggested order: non-packed fields from Suggested (or Fields), then flags.
	var ordered []layout.Field
	seen := make(map[int]bool)
	srcOrder := iss.Suggested
	if len(srcOrder) == 0 {
		srcOrder = iss.Fields
	}
	for _, f := range srcOrder {
		if _, pack := packSet[f.Name]; pack {
			continue
		}
		ordered = append(ordered, f)
		seen[slotKey(f, useIdx, byName)] = true
	}
	for _, f := range iss.Fields {
		if _, pack := packSet[f.Name]; pack {
			continue
		}
		k := slotKey(f, useIdx, byName)
		if seen[k] {
			continue
		}
		ordered = append(ordered, f)
	}

	bits := make([]string, len(iss.BoolPack))
	for name, bit := range packSet {
		bits[bit] = name
	}
	var bitDoc strings.Builder
	for i, n := range bits {
		if i > 0 {
			bitDoc.WriteString(", ")
		}
		fmt.Fprintf(&bitDoc, "bit%d=%s", i, n)
	}

	indent := detectIndent(fset, content, st)
	var body strings.Builder
	body.WriteByte('\n')

	for _, f := range ordered {
		idx, ok := resolveSlot(f, useIdx, byName, len(slots))
		if !ok {
			return textEdit{}, fmt.Errorf("cannot map field %q for bool-pack", f.Name)
		}
		slot := slots[idx]
		body.WriteString(indent)
		if slot.name == nil {
			body.WriteString(fieldText(fset, content, slot.field))
		} else if len(slot.field.Names) == 1 {
			body.WriteString(fieldText(fset, content, slot.field))
		} else {
			body.WriteString(splitFieldText(fset, content, slot.field, slot.name, true, true))
		}
		body.WriteByte('\n')
	}
	body.WriteString(indent)
	body.WriteString("flags ")
	body.WriteString(flagType)
	body.WriteString(" // goalign:bool-pack ")
	body.WriteString(bitDoc.String())
	body.WriteByte('\n')

	open := fset.Position(st.Fields.Opening).Offset
	closeOff := fset.Position(st.Fields.Closing).Offset
	return textEdit{start: open + 1, end: closeOff, new: body.String()}, nil
}
