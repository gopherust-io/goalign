package analyzer

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"regexp"
	"strings"

	"github.com/gopherust-io/goalign/internal/layout"
)

// Result represents the analysis result for a file.
type Result struct {
	File   string  `json:"file"`
	Issues []Issue `json:"issues"`
}

// Issue represents a struct alignment issue.
type Issue struct {
	StructName      string         `json:"struct_name"`
	Message         string         `json:"message"`
	Severity        string         `json:"severity"`
	Fields          []layout.Field `json:"fields"`
	Suggested       []layout.Field `json:"suggested,omitempty"`
	Notes           []string       `json:"notes,omitempty"`
	Line            int            `json:"line"`
	Wasted          int            `json:"wasted_bytes"`
	TotalSize       int            `json:"total_size"`
	SuggestedWasted int            `json:"suggested_wasted"`
	Saved           int            `json:"saved_bytes"`
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
	result := Result{File: filename}

	content, err := os.ReadFile(filename)
	if err != nil {
		return result, err
	}
	return AnalyzeSource(filename, content, sizer)
}

// AnalyzeSource analyzes Go source bytes (single parse; no second disk read).
func AnalyzeSource(filename string, content []byte, sizer layout.Sizer) (Result, error) {
	result := Result{File: filename}

	fileSet := token.NewFileSet()
	node, err := parser.ParseFile(fileSet, filename, content, parser.ParseComments)
	if err != nil {
		return result, err
	}

	var cmap ast.CommentMap
	if len(node.Comments) > 0 {
		cmap = ast.NewCommentMap(fileSet, node, node.Comments)
	}
	locals := sizer.CollectLocals(node)
	fieldBuf := make([]layout.Field, 0, 16)
	suggestBuf := make([]layout.Field, 0, 32)

	ast.Inspect(node, func(n ast.Node) bool {
		gd, ok := n.(*ast.GenDecl)
		if !ok || gd.Tok != token.TYPE {
			return true
		}
		ignoreDecl := genDeclIgnored(fileSet, gd, cmap, node)
		for _, spec := range gd.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			st, ok := ts.Type.(*ast.StructType)
			if !ok {
				continue
			}
			if ignoreDecl || hasIgnoreComment(fileSet, ts, gd, cmap, node) {
				continue
			}

			res, fields := sizer.Compute(fieldBuf[:0], st.Fields, locals)
			fieldBuf = fields[:cap(fields)]

			if res.Unknown {
				continue
			}
			if !layout.NeedsReport(res.Wasted, fields) {
				continue
			}

			owned := make([]layout.Field, res.N)
			copy(owned, fields)
			layout.FillTypeNames(owned, st.Fields)

			sug := layout.Suggest(suggestBuf[:0], owned, res.Wasted)
			suggestBuf = sug.Fields[:cap(sug.Fields)]
			suggestedOwned := make([]layout.Field, len(sug.Fields))
			copy(suggestedOwned, sug.Fields)

			line := fileSet.Position(ts.Pos()).Line
			msg := buildMessage(ts.Name.Name, res.Wasted, res.Total, sug.Saved, sug.Notes)

			result.Issues = append(result.Issues, Issue{
				StructName:      ts.Name.Name,
				Line:            line,
				Message:         msg,
				Severity:        getSeverity(res.Wasted),
				Wasted:          res.Wasted,
				TotalSize:       res.Total,
				Fields:          owned,
				Suggested:       suggestedOwned,
				SuggestedWasted: sug.Wasted,
				Saved:           sug.Saved,
				Notes:           sug.Notes,
			})
		}
		return false
	})

	return result, nil
}

func buildMessage(name string, wasted, total, saved int, notes []string) string {
	pct := 0
	if total > 0 {
		pct = (wasted * 100) / total
	}
	msg := fmt.Sprintf("Struct '%s' has %d bytes of padding (%d%% waste)", name, wasted, pct)
	if saved > 0 {
		msg += fmt.Sprintf("; reorder saves %d bytes", saved)
	}
	if len(notes) > 0 {
		msg += "; " + strings.Join(notes, "; ")
	}
	return msg
}

func genDeclIgnored(fset *token.FileSet, gd *ast.GenDecl, cmap ast.CommentMap, file *ast.File) bool {
	return ignoreFromDocCmapEOL(fset, gd.Doc, gd, gd.End(), cmap, file)
}

func hasIgnoreComment(fset *token.FileSet, typeSpec *ast.TypeSpec, gd *ast.GenDecl, cmap ast.CommentMap, file *ast.File) bool {
	end := typeSpec.End()
	if gd != nil && gd.End() > end {
		end = gd.End()
	}
	return ignoreFromDocCmapEOL(fset, typeSpec.Doc, typeSpec, end, cmap, file)
}

// ignoreFromDocCmapEOL reports goalign:ignore on doc comments, EOL at end, or CommentMap[key].
func ignoreFromDocCmapEOL(fset *token.FileSet, doc *ast.CommentGroup, cmapKey ast.Node, end token.Pos, cmap ast.CommentMap, file *ast.File) bool {
	if commentGroupHasIgnore(doc) {
		return true
	}
	if hasEOLIgnore(fset, end, file) {
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

// hasEOLIgnore reports whether a // goalign:ignore comment ends on the same
// source line as pos (handles last-decl CommentMap attaching to *ast.File).
func hasEOLIgnore(fset *token.FileSet, pos token.Pos, file *ast.File) bool {
	if file == nil || fset == nil {
		return false
	}
	line := fset.Position(pos).Line
	for _, cg := range file.Comments {
		for _, c := range cg.List {
			if !strings.HasPrefix(c.Text, "//") {
				continue
			}
			if fset.Position(c.Pos()).Line != line {
				continue
			}
			if isIgnoreDirective(c.Text) {
				return true
			}
		}
	}
	return false
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
