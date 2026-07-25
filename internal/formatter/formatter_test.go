package formatter_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/nekruzjm/goalign/internal/analyzer"
	"github.com/nekruzjm/goalign/internal/formatter"
	"github.com/nekruzjm/goalign/internal/layout"
)

func TestFormatTextSuggested(t *testing.T) {
	results := []analyzer.Result{{
		File: "x.go",
		Issues: []analyzer.Issue{{
			StructName: "Bad",
			Line:       3,
			Message:    "has padding",
			Severity:   "high",
			Wasted:     10,
			Saved:      8,
			Fields: []layout.Field{
				{Name: "A", Type: "bool", Size: 1, Offset: 0, Align: 1},
				{Name: "B", Type: "int64", Size: 8, Offset: 8, Align: 8},
			},
			Suggested: []layout.Field{
				{Name: "B", Type: "int64", Size: 8, Offset: 0, Align: 8},
				{Name: "A", Type: "bool", Size: 1, Offset: 8, Align: 1},
			},
			Notes: []string{"atomics-first: place counters first"},
		}},
	}}

	var buf bytes.Buffer
	if err := formatter.Format(&buf, results, "text"); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "Suggested order") {
		t.Fatalf("missing suggested order:\n%s", out)
	}
	if !strings.Contains(out, "note: atomics-first") {
		t.Fatalf("missing note:\n%s", out)
	}
	if !strings.Contains(out, "bytes savable") {
		t.Fatalf("missing summary savings:\n%s", out)
	}
}

func TestFormatJSON(t *testing.T) {
	results := []analyzer.Result{{
		File: "x.go",
		Issues: []analyzer.Issue{{
			StructName: "S",
			Line:       1,
			Severity:   "low",
			Wasted:     1,
		}},
	}}
	var buf bytes.Buffer
	if err := formatter.Format(&buf, results, "json"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `"struct_name": "S"`) {
		t.Fatalf("bad json: %s", buf.String())
	}
}
