// Package analysis provides a go/analysis Analyzer wrapping GoAlign heuristics.
//
//	go vet -vettool=$(which goalign-analyzer) ./...
//
// Or import Analyzer into a multichecker / golangci-lint plugin.
package analysis

import (
	"go/ast"
	"go/token"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"

	"github.com/gopherust-io/goalign/internal/analyzer"
	"github.com/gopherust-io/goalign/internal/layout"
)

// Analyzer reports struct padding waste using GoAlign's AST sizer (fast path).
var Analyzer = &analysis.Analyzer{
	Name:     "goalign",
	Doc:      "report Go struct padding waste (GoAlign AST heuristics)",
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	Run:      run,
}

func run(pass *analysis.Pass) (any, error) {
	_ = pass.ResultOf[inspect.Analyzer].(*inspector.Inspector) // keep Requires edge
	sizer := layout.DefaultSizer()

	for _, file := range pass.Files {
		locals, localFlags := sizer.CollectLocalsFull(file)
		eolIgnore := analyzer.BuildEOLIgnoreLines(pass.Fset, file)
		var cmap ast.CommentMap
		if len(file.Comments) > 0 {
			cmap = ast.NewCommentMap(pass.Fset, file, file.Comments)
		}

		for _, decl := range file.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.TYPE {
				continue
			}
			if analyzer.CommentGroupHasIgnore(gd.Doc) {
				continue
			}
			if analyzer.HasEOLIgnore(pass.Fset, gd.End(), eolIgnore) {
				continue
			}
			for _, spec := range gd.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				st, ok := ts.Type.(*ast.StructType)
				if !ok {
					continue
				}
				if analyzer.CommentGroupHasIgnore(ts.Doc) {
					continue
				}
				end := ts.End()
				if gd.End() > end {
					end = gd.End()
				}
				if analyzer.HasEOLIgnore(pass.Fset, end, eolIgnore) {
					continue
				}
				if cmap != nil {
					ignored := false
					for _, cg := range cmap[ts] {
						if analyzer.CommentGroupHasIgnore(cg) {
							ignored = true
							break
						}
					}
					if ignored {
						continue
					}
				}
				if hasFieldIgnore(pass.Fset, st, eolIgnore) {
					continue
				}
				res, fields := sizer.Compute(nil, st.Fields, locals, localFlags)
				if res.Unknown || !layout.NeedsReport(res.Wasted, fields) {
					continue
				}
				sug := layout.Suggest(nil, fields[:res.N], res.Wasted)
				msg := ts.Name.Name + " has padding waste"
				if sug.Saved > 0 {
					msg += "; reorder may save bytes"
				}
				pass.Reportf(ts.Pos(), "%s (%d bytes wasted)", msg, res.Wasted)
			}
		}
	}
	return nil, nil
}

func hasFieldIgnore(fset *token.FileSet, st *ast.StructType, eolIgnore map[int]struct{}) bool {
	if st.Fields == nil {
		return false
	}
	for _, f := range st.Fields.List {
		if analyzer.CommentGroupHasIgnore(f.Doc) || analyzer.CommentGroupHasIgnore(f.Comment) {
			return true
		}
		if analyzer.HasEOLIgnore(fset, f.End(), eolIgnore) {
			return true
		}
	}
	return false
}
