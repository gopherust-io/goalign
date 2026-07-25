package analyzer

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"

	"github.com/nekruzjm/goalign/internal/layout"
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
	fieldBuf := make([]layout.Field, 0, 16)
	// Suggest needs capacity >= 2*n for zero-alloc partition scratch.
	suggestBuf := make([]layout.Field, 0, 32)

	ast.Inspect(node, func(n ast.Node) bool {
		gd, ok := n.(*ast.GenDecl)
		if !ok || gd.Tok != token.TYPE {
			return true
		}
		ignoreDecl := genDeclIgnored(gd, cmap)
		for _, spec := range gd.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			st, ok := ts.Type.(*ast.StructType)
			if !ok {
				continue
			}
			if ignoreDecl || hasIgnoreComment(ts, cmap) {
				continue
			}

			res, fields := sizer.Compute(fieldBuf[:0], st.Fields)
			fieldBuf = fields[:cap(fields)]

			if res.Unknown {
				continue // unresolvable sizes — avoid false positives
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
			// Type strings already present on Field after FillTypeNames + Suggest copy.

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
		return false // already handled TypeSpecs; avoid double visit
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

func genDeclIgnored(gd *ast.GenDecl, cmap ast.CommentMap) bool {
	if gd.Doc != nil {
		for _, c := range gd.Doc.List {
			if strings.Contains(c.Text, "goalign:ignore") {
				return true
			}
		}
	}
	if cmap == nil {
		return false
	}
	for _, cg := range cmap[gd] {
		for _, c := range cg.List {
			if strings.Contains(c.Text, "goalign:ignore") {
				return true
			}
		}
	}
	return false
}

func hasIgnoreComment(typeSpec *ast.TypeSpec, cmap ast.CommentMap) bool {
	if typeSpec.Doc != nil {
		for _, c := range typeSpec.Doc.List {
			if strings.Contains(c.Text, "goalign:ignore") {
				return true
			}
		}
	}
	if cmap == nil {
		return false
	}
	for _, cg := range cmap[typeSpec] {
		for _, c := range cg.List {
			if strings.Contains(c.Text, "goalign:ignore") {
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
	return "info" // rule-only (e.g. atomics-first with zero padding)
}
