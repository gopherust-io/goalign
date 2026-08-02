package analyzer

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/gopherust-io/goalign/internal/layout"
)

// Result represents the analysis result for a file.
type Result struct {
	File    string  `json:"file"`
	Content []byte  `json:"-"` // source bytes from analyze; reuse for fix to avoid double read
	Issues  []Issue `json:"issues"`
}

// Issue represents a struct alignment issue.
type Issue struct {
	StructName      string         `json:"struct_name"`
	Message         string         `json:"message"`
	Severity        string         `json:"severity"`
	Fields          []layout.Field `json:"fields"`
	Suggested       []layout.Field `json:"suggested,omitempty"`
	Notes           []string       `json:"notes,omitempty"`
	BoolPack        []string       `json:"bool_pack,omitempty"` // unexported bools for --rewrite-bools
	Line            int            `json:"line"`
	Wasted          int            `json:"wasted_bytes"`
	TotalSize       int            `json:"total_size"`
	SuggestedWasted int            `json:"suggested_wasted"`
	Saved           int            `json:"saved_bytes"`
	CacheLine       int            `json:"cache_line,omitempty"` // for CLINE column / Cacheguard
}

var ignoreDirective = regexp.MustCompile(`(?m)^\s*(//|/\*)\s*goalign:ignore\b`)

func isIgnoreDirective(text string) bool {
	return ignoreDirective.MatchString(text)
}

// AnalyzeFile analyzes a Go file for struct alignment issues.
func AnalyzeFile(filename string) (Result, error) {
	return AnalyzeFileWithSizer(filename, layout.DefaultSizer())
}

// AnalyzeFileWithSizer analyzes a file using the given architecture sizer.
func AnalyzeFileWithSizer(filename string, sizer layout.Sizer) (Result, error) {
	return AnalyzeFileWithOptions(filename, Options{Sizer: sizer, Policy: layout.PolicyAtomics})
}

// AnalyzeFileWithOptions analyzes a file with full options.
func AnalyzeFileWithOptions(filename string, opts Options) (Result, error) {
	result := Result{File: filename}

	content, err := os.ReadFile(filename)
	if err != nil {
		return result, err
	}
	return AnalyzeSourceWithOptions(filename, content, opts)
}

// AnalyzeSource analyzes Go source bytes (single parse; no second disk read).
func AnalyzeSource(filename string, content []byte, sizer layout.Sizer) (Result, error) {
	return AnalyzeSourceWithOptions(filename, content, Options{Sizer: sizer, Policy: layout.PolicyAtomics})
}

// AnalyzeSourceWithOptions analyzes Go source with policy and optional type sizes.
func AnalyzeSourceWithOptions(filename string, content []byte, opts Options) (Result, error) {
	result := Result{File: filename, Content: content}
	if opts.Policy == "" {
		opts.Policy = layout.PolicyAtomics
	}
	cacheLine := layout.NormalizeCacheLine(opts.CacheLine)

	fileSet := token.NewFileSet()
	node, err := parser.ParseFile(fileSet, filename, content, parser.ParseComments)
	if err != nil {
		return result, err
	}

	var cmap ast.CommentMap
	if len(node.Comments) > 0 {
		cmap = ast.NewCommentMap(fileSet, node, node.Comments)
	}
	eolIgnore := buildEOLIgnoreLines(fileSet, node)
	locals, localFlags := opts.Sizer.CollectLocalsFull(node)
	if len(opts.TypeSizes) > 0 {
		locals = mergeLocals(locals, opts.TypeSizes)
		locals = mergeImportAliases(node, locals)
	}
	fieldBuf := make([]layout.Field, 0, 16)
	suggestBuf := make([]layout.Field, 0, 32)

	ast.Inspect(node, func(n ast.Node) bool {
		gd, ok := n.(*ast.GenDecl)
		if !ok || gd.Tok != token.TYPE {
			return true
		}
		ignoreDecl := genDeclIgnored(fileSet, gd, cmap, eolIgnore)
		for _, spec := range gd.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			st, ok := ts.Type.(*ast.StructType)
			if !ok {
				continue
			}
			if ignoreDecl || hasIgnoreComment(fileSet, ts, gd, cmap, eolIgnore) {
				continue
			}

			if st.Fields == nil || len(st.Fields.List) == 0 {
				continue
			}
			partialIgnore := hasAnyFieldIgnore(fileSet, st.Fields, cmap, eolIgnore)

			// Always size the full struct so metrics match real layout.
			res, fields := opts.Sizer.Compute(fieldBuf[:0], st.Fields, locals, localFlags)
			fieldBuf = fields[:cap(fields)]

			if res.Unknown || res.N == 0 {
				continue
			}

			owned := make([]layout.Field, res.N)
			copy(owned, fields)
			layout.FillTypeNames(owned, st.Fields)
			applyFieldDirectives(fileSet, st.Fields, owned, cmap, eolIgnore)

			if !layout.NeedsReportCacheLine(res.Wasted, owned, cacheLine) {
				continue
			}

			var suggestedOwned []layout.Field
			var sug layout.SuggestResult
			var boolPack []string
			if !partialIgnore {
				sug = layout.SuggestWithPolicy(suggestBuf[:0], owned, res.Wasted, opts.Policy)
				suggestBuf = sug.Fields[:cap(sug.Fields)]
				suggestedOwned = make([]layout.Field, len(sug.Fields))
				copy(suggestedOwned, sug.Fields)
				boolPack = layout.BoolPackCandidates(owned)
				if opts.Cacheguard {
					base := suggestedOwned
					if !layout.HasFalseShare(base, cacheLine) && layout.HasFalseShare(owned, cacheLine) {
						base = owned
					}
					if layout.HasFalseShare(base, cacheLine) {
						guarded, nPads := layout.ApplyCacheguard(base, cacheLine)
						if nPads > 0 {
							if relayout, total, wasted, ok := layout.Relayout(guarded); ok {
								suggestedOwned = relayout
								sug.Total = total
								sug.Wasted = wasted
							} else {
								suggestedOwned = guarded
							}
							sug.Saved = 0 // Cacheguard grows the object on purpose
						}
					}
				}
			} else {
				sug.Notes = append(sug.Notes, "field-ignore: autofix skipped (per-field // goalign:ignore)")
			}

			notes := append([]string{}, sug.Notes...)
			notes = appendUniqueNotes(notes, layout.FalseShareNotes(owned, cacheLine))
			if len(suggestedOwned) > 0 {
				notes = appendUniqueNotes(notes, layout.FalseShareNotes(suggestedOwned, cacheLine))
			}
			// If Cacheguard cleared collisions, keep a short advisory that pads were applied.
			if opts.Cacheguard && len(suggestedOwned) > 0 && !layout.HasFalseShare(suggestedOwned, cacheLine) {
				if layout.HasFalseShare(owned, cacheLine) {
					notes = appendUniqueNotes(notes, []string{
						"cacheguard: suggested layout isolates contended fields onto separate cache lines",
					})
				}
			}

			line := fileSet.Position(ts.Pos()).Line
			msg := buildMessage(ts.Name.Name, res.Wasted, res.Total, sug.Saved, notes)

			result.Issues = append(result.Issues, Issue{
				StructName:      ts.Name.Name,
				Line:            line,
				Message:         msg,
				Severity:        getSeverityCacheguard(res.Wasted, notes),
				Wasted:          res.Wasted,
				TotalSize:       res.Total,
				Fields:          owned,
				Suggested:       suggestedOwned,
				SuggestedWasted: sug.Wasted,
				Saved:           sug.Saved,
				Notes:           notes,
				BoolPack:        boolPack,
				CacheLine:       cacheLine,
			})
		}
		return false
	})

	return result, nil
}

func appendUniqueNotes(dst, extra []string) []string {
	if len(extra) == 0 {
		return dst
	}
	seen := make(map[string]struct{}, len(dst)+len(extra))
	for _, n := range dst {
		seen[n] = struct{}{}
	}
	for _, n := range extra {
		if _, ok := seen[n]; ok {
			continue
		}
		seen[n] = struct{}{}
		dst = append(dst, n)
	}
	return dst
}

func getSeverityCacheguard(wasted int, notes []string) string {
	sev := getSeverity(wasted)
	if wasted > 0 {
		return sev
	}
	for _, n := range notes {
		if strings.HasPrefix(n, "false-share") {
			return "medium"
		}
	}
	return sev
}

var fieldDirective = regexp.MustCompile(`goalign:(contend|hot|cold)\b`)

func applyFieldDirectives(fset *token.FileSet, list *ast.FieldList, fields []layout.Field, cmap ast.CommentMap, _ map[int]struct{}) {
	if list == nil || len(fields) == 0 {
		return
	}
	i := 0
	for _, f := range list.List {
		count := 1
		if len(f.Names) > 0 {
			count = len(f.Names)
		}
		flags := fieldDirectiveFlags(f, cmap)
		for j := 0; j < count && i < len(fields); j++ {
			fields[i].Flags |= flags
			i++
		}
	}
	_ = fset
}

func fieldDirectiveFlags(f *ast.Field, cmap ast.CommentMap) layout.FieldFlags {
	var flags layout.FieldFlags
	scan := func(cg *ast.CommentGroup) {
		if cg == nil {
			return
		}
		for _, c := range cg.List {
			for _, m := range fieldDirective.FindAllStringSubmatch(c.Text, -1) {
				if len(m) < 2 {
					continue
				}
				switch m[1] {
				case "contend":
					flags |= layout.FlagContend
				case "hot":
					flags |= layout.FlagHot
				case "cold":
					flags |= layout.FlagCold
				}
			}
		}
	}
	scan(f.Doc)
	scan(f.Comment)
	if cmap != nil {
		for _, cg := range cmap[f] {
			scan(cg)
		}
	}
	return flags
}

func mergeLocals(locals map[string]layout.Info, extra map[string]layout.Info) map[string]layout.Info {
	if len(extra) == 0 {
		return locals
	}
	if locals == nil {
		locals = make(map[string]layout.Info, len(extra))
	}
	for k, v := range extra {
		if _, ok := locals[k]; !ok {
			locals[k] = v
		}
	}
	return locals
}

// mergeImportAliases copies pkg.Type / path.Type entries under the file's import
// selector names (including explicit aliases like `import r "io"` → r.Reader).
func mergeImportAliases(file *ast.File, locals map[string]layout.Info) map[string]layout.Info {
	if file == nil || len(locals) == 0 || len(file.Imports) == 0 {
		return locals
	}
	extra := make(map[string]layout.Info)
	for _, imp := range file.Imports {
		if imp.Path == nil {
			continue
		}
		path, err := strconv.Unquote(imp.Path.Value)
		if err != nil || path == "" {
			continue
		}
		if imp.Name != nil && (imp.Name.Name == "_" || imp.Name.Name == ".") {
			continue
		}
		alias := pathBase(path)
		if imp.Name != nil {
			alias = imp.Name.Name
		}
		defaultName := pathBase(path)
		pathPrefix := path + "."
		namePrefix := defaultName + "."
		for k, v := range locals {
			var suffix string
			switch {
			case strings.HasPrefix(k, pathPrefix):
				suffix = k[len(pathPrefix):]
			case strings.HasPrefix(k, namePrefix):
				suffix = k[len(namePrefix):]
			default:
				continue
			}
			if suffix == "" || strings.Contains(suffix, ".") {
				continue
			}
			key := alias + "." + suffix
			if _, ok := locals[key]; ok {
				continue
			}
			if _, ok := extra[key]; ok {
				continue
			}
			extra[key] = v
		}
	}
	return mergeLocals(locals, extra)
}

func pathBase(importPath string) string {
	if i := strings.LastIndex(importPath, "/"); i >= 0 {
		return importPath[i+1:]
	}
	return importPath
}

// hasAnyFieldIgnore reports whether any field has // goalign:ignore.
func hasAnyFieldIgnore(fset *token.FileSet, fields *ast.FieldList, cmap ast.CommentMap, eolIgnore map[int]struct{}) bool {
	if fields == nil {
		return false
	}
	for _, f := range fields.List {
		if fieldIgnored(fset, f, cmap, eolIgnore) {
			return true
		}
	}
	return false
}

func fieldIgnored(fset *token.FileSet, f *ast.Field, cmap ast.CommentMap, eolIgnore map[int]struct{}) bool {
	if commentGroupHasIgnore(f.Doc) || commentGroupHasIgnore(f.Comment) {
		return true
	}
	if hasEOLIgnore(fset, f.End(), eolIgnore) {
		return true
	}
	if cmap != nil {
		for _, cg := range cmap[f] {
			if commentGroupHasIgnore(cg) {
				return true
			}
		}
	}
	return false
}

func buildMessage(name string, wasted, total, saved int, notes []string) string {
	pct := 0
	if total > 0 {
		pct = (wasted * 100) / total
	}
	var msg string
	if wasted == 0 && notesHavePrefix(notes, "false-share") {
		msg = fmt.Sprintf("Struct '%s' has contended fields sharing a cache line", name)
	} else {
		msg = fmt.Sprintf("Struct '%s' has %d bytes of padding (%d%% waste)", name, wasted, pct)
	}
	if saved > 0 {
		msg += fmt.Sprintf("; reorder saves %d bytes", saved)
	}
	if len(notes) > 0 {
		msg += "; " + strings.Join(notes, "; ")
	}
	return msg
}

func notesHavePrefix(notes []string, prefix string) bool {
	for _, n := range notes {
		if strings.HasPrefix(n, prefix) {
			return true
		}
	}
	return false
}

func genDeclIgnored(fset *token.FileSet, gd *ast.GenDecl, cmap ast.CommentMap, eolIgnore map[int]struct{}) bool {
	return ignoreFromDocCmapEOL(fset, gd.Doc, gd, gd.End(), cmap, eolIgnore)
}

func hasIgnoreComment(fset *token.FileSet, typeSpec *ast.TypeSpec, gd *ast.GenDecl, cmap ast.CommentMap, eolIgnore map[int]struct{}) bool {
	end := typeSpec.End()
	if gd != nil && gd.End() > end {
		end = gd.End()
	}
	return ignoreFromDocCmapEOL(fset, typeSpec.Doc, typeSpec, end, cmap, eolIgnore)
}

// ignoreFromDocCmapEOL reports goalign:ignore on doc comments, EOL at end, or CommentMap[key].
func ignoreFromDocCmapEOL(fset *token.FileSet, doc *ast.CommentGroup, cmapKey ast.Node, end token.Pos, cmap ast.CommentMap, eolIgnore map[int]struct{}) bool {
	if commentGroupHasIgnore(doc) {
		return true
	}
	if hasEOLIgnore(fset, end, eolIgnore) {
		return true
	}
	if cmap == nil || cmapKey == nil {
		return false
	}
	for _, cg := range cmap[cmapKey] {
		if commentGroupHasIgnore(cg) {
			return true
		}
	}
	return false
}

func commentGroupHasIgnore(cg *ast.CommentGroup) bool {
	if cg == nil {
		return false
	}
	for _, c := range cg.List {
		if isIgnoreDirective(c.Text) {
			return true
		}
	}
	return false
}

// buildEOLIgnoreLines indexes // goalign:ignore comments by source line once per file.
func buildEOLIgnoreLines(fset *token.FileSet, file *ast.File) map[int]struct{} {
	if file == nil || fset == nil || len(file.Comments) == 0 {
		return nil
	}
	out := make(map[int]struct{})
	for _, cg := range file.Comments {
		for _, c := range cg.List {
			if !strings.HasPrefix(c.Text, "//") {
				continue
			}
			if !isIgnoreDirective(c.Text) {
				continue
			}
			out[fset.Position(c.Pos()).Line] = struct{}{}
		}
	}
	return out
}

// BuildEOLIgnoreLines is the exported form of buildEOLIgnoreLines for the vet pass.
func BuildEOLIgnoreLines(fset *token.FileSet, file *ast.File) map[int]struct{} {
	return buildEOLIgnoreLines(fset, file)
}

// hasEOLIgnore reports whether a // goalign:ignore comment ends on the same
// source line as pos (handles last-decl CommentMap attaching to *ast.File).
func hasEOLIgnore(fset *token.FileSet, pos token.Pos, eolIgnore map[int]struct{}) bool {
	if fset == nil || len(eolIgnore) == 0 {
		return false
	}
	_, ok := eolIgnore[fset.Position(pos).Line]
	return ok
}

// HasEOLIgnore is the exported form of hasEOLIgnore for the vet pass.
func HasEOLIgnore(fset *token.FileSet, pos token.Pos, eolIgnore map[int]struct{}) bool {
	return hasEOLIgnore(fset, pos, eolIgnore)
}

// CommentGroupHasIgnore reports whether a comment group contains goalign:ignore.
func CommentGroupHasIgnore(cg *ast.CommentGroup) bool {
	return commentGroupHasIgnore(cg)
}

func getSeverity(wastedBytes int) string {
	if wastedBytes >= 16 {
		return "high"
	}
	if wastedBytes >= 8 {
		return "medium"
	}
	if wastedBytes > 0 {
		return "low"
	}
	return "info"
}

// IsGenerated reports whether source looks like generated code.
func IsGenerated(content []byte) bool {
	// Match common "Code generated ... DO NOT EDIT" markers in the first 2KB.
	head := content
	if len(head) > 2048 {
		head = head[:2048]
	}
	s := string(head)
	return strings.Contains(s, "Code generated") && strings.Contains(s, "DO NOT EDIT")
}

// MatchGlob reports whether filePath matches pattern (* and ** supported).
func MatchGlob(pattern, filePath string) bool {
	pattern = strings.ReplaceAll(pattern, "\\", "/")
	filePath = strings.ReplaceAll(filePath, "\\", "/")
	if strings.Contains(pattern, "**") {
		// **/x.go or path/**/x
		suf := pattern
		if i := strings.Index(pattern, "**"); i >= 0 {
			suf = strings.TrimPrefix(pattern[i+2:], "/")
		}
		if suf == "" {
			return true
		}
		if ok, _ := path.Match(suf, path.Base(filePath)); ok {
			return true
		}
		return strings.HasSuffix(filePath, strings.TrimPrefix(suf, "*"))
	}
	if ok, _ := path.Match(pattern, filePath); ok {
		return true
	}
	ok, _ := path.Match(pattern, path.Base(filePath))
	return ok
}

// ExportedName reports whether name is exported (for bool-pack safety).
func ExportedName(name string) bool {
	if name == "" {
		return false
	}
	r, _ := utf8.DecodeRuneInString(name)
	return unicode.IsUpper(r)
}
