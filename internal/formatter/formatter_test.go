package formatter_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/gopherust-io/goalign/internal/analyzer"
	"github.com/gopherust-io/goalign/internal/formatter"
	"github.com/gopherust-io/goalign/internal/layout"
)

func sampleResults() []analyzer.Result {
	return []analyzer.Result{{
		File: "x.go",
		Issues: []analyzer.Issue{
			{
				StructName: "High",
				Line:       1,
				Message:    "high waste",
				Severity:   "high",
				Wasted:     16,
				Saved:      8,
				Fields: []layout.Field{
					{Name: "A", Type: "bool", Size: 1, Offset: 0, Align: 1},
					{Name: "B", Type: "int64", Size: 8, Offset: 8, Align: 8},
				},
				Suggested: []layout.Field{
					{Name: "B", Type: "int64", Size: 8, Offset: 0, Align: 8},
					{Name: "A", Type: "bool", Size: 1, Offset: 8, Align: 1},
				},
			},
			{
				StructName: "Med",
				Line:       2,
				Message:    "med",
				Severity:   "medium",
				Wasted:     8,
				Saved:      0,
				Fields:     []layout.Field{{Name: "X", Type: "int", Size: 8, Offset: 0, Align: 8}},
				Suggested:  []layout.Field{{Name: "X", Type: "int", Size: 8, Offset: 0, Align: 8}},
			},
			{
				StructName: "Low",
				Line:       3,
				Message:    "low",
				Severity:   "low",
				Wasted:     2,
			},
			{
				StructName: "Info",
				Line:       4,
				Message:    "info",
				Severity:   "info",
				Wasted:     0,
			},
			{
				StructName: "Other",
				Line:       5,
				Message:    "other",
				Severity:   "weird",
				Wasted:     0,
			},
		},
	}}
}

func TestFormatTable(t *testing.T) {
	var buf bytes.Buffer
	if err := formatter.Format(&buf, sampleResults(), "table"); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "FILE") || !strings.Contains(out, "High") || !strings.Contains(out, "WASTED") {
		t.Fatalf("table:\n%s", out)
	}
}

func TestFormatTableEmpty(t *testing.T) {
	var buf bytes.Buffer
	if err := formatter.Format(&buf, nil, "table"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "No struct alignment issues found") {
		t.Fatalf("%q", buf.String())
	}
}

func TestFormatTextSeverities(t *testing.T) {
	var buf bytes.Buffer
	if err := formatter.Format(&buf, sampleResults(), "text"); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{"HIGH", "MED", "LOW", "INFO", "WEIRD", "Current", "Suggested", "Summary:"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "\033[") {
		t.Fatal("ANSI colors should not appear for bytes.Buffer")
	}
}

func TestFormatCaseInsensitive(t *testing.T) {
	var buf bytes.Buffer
	if err := formatter.Format(&buf, sampleResults(), "JSON"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `"struct_name"`) {
		t.Fatalf("%s", buf.String())
	}
}

func TestFormatFixSummary(t *testing.T) {
	var buf bytes.Buffer
	if err := formatter.FormatFixSummary(&buf, 0, 0, 0); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "No structs needed fixing") {
		t.Fatalf("got %q", buf.String())
	}
	buf.Reset()
	if err := formatter.FormatFixSummary(&buf, 2, 3, 16); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "3 structs") || !strings.Contains(out, "2 files") || !strings.Contains(out, "16 bytes") {
		t.Fatalf("got %q", out)
	}
}

func BenchmarkFormatText(b *testing.B) {
	results := sampleResults()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var buf bytes.Buffer
		_ = formatter.Format(&buf, results, "text")
	}
}

func BenchmarkFormatJSON(b *testing.B) {
	results := sampleResults()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var buf bytes.Buffer
		_ = formatter.Format(&buf, results, "json")
	}
}

func BenchmarkFormatTable(b *testing.B) {
	results := sampleResults()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var buf bytes.Buffer
		_ = formatter.Format(&buf, results, "table")
	}
}
