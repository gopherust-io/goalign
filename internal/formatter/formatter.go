package formatter

import (
	"encoding/json"
	"io"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/gopherust-io/goalign/internal/analyzer"
	"github.com/gopherust-io/goalign/internal/layout"
)

// Format writes analysis results to w in the given format (text|table|json).
func Format(w io.Writer, results []analyzer.Result, format string) error {
	switch strings.ToLower(format) {
	case "json":
		return formatJSON(w, results)
	case "table":
		return formatTable(w, results)
	default:
		return formatText(w, results)
	}
}

func formatText(w io.Writer, results []analyzer.Result) error {
	if len(results) == 0 {
		_, err := io.WriteString(w, "No struct alignment issues found.\n")
		return err
	}

	var buf []byte
	totalIssues := 0
	totalWasted := 0
	totalSaved := 0

	for _, result := range results {
		if len(result.Issues) == 0 {
			continue
		}

		buf = append(buf, '\n')
		buf = append(buf, []byte("📁 ")...)
		buf = append(buf, result.File...)
		buf = append(buf, '\n')
		for i := 0; i < len(result.File)+4; i++ {
			buf = append(buf, '=')
		}
		buf = append(buf, '\n')

		for _, issue := range result.Issues {
			totalIssues++
			totalWasted += issue.Wasted
			totalSaved += issue.Saved

			buf = append(buf, getSeverityIcon(issue.Severity)...)
			buf = append(buf, ' ')
			buf = append(buf, issue.StructName...)
			buf = append(buf, []byte(" (line ")...)
			buf = strconv.AppendInt(buf, int64(issue.Line), 10)
			buf = append(buf, ')', '\n')
			buf = append(buf, []byte("   ")...)
			buf = append(buf, issue.Message...)
			buf = append(buf, '\n')

			if len(issue.Fields) > 0 {
				buf = append(buf, []byte("   Fields:\n")...)
				buf = appendFields(buf, issue.Fields)
			}
			if len(issue.Suggested) > 0 && issue.Saved > 0 {
				buf = append(buf, []byte("   Suggested order")...)
				if issue.Saved > 0 {
					buf = append(buf, []byte(" (saves ")...)
					buf = strconv.AppendInt(buf, int64(issue.Saved), 10)
					buf = append(buf, []byte(" bytes)")...)
				}
				buf = append(buf, ':', '\n')
				buf = appendFields(buf, issue.Suggested)
			}
			for _, note := range issue.Notes {
				buf = append(buf, []byte("   note: ")...)
				buf = append(buf, note...)
				buf = append(buf, '\n')
			}
			buf = append(buf, '\n')
		}

		if _, err := w.Write(buf); err != nil {
			return err
		}
		buf = buf[:0]
	}

	buf = append(buf, []byte("\n📊 Summary: ")...)
	buf = strconv.AppendInt(buf, int64(totalIssues), 10)
	buf = append(buf, []byte(" issues found, ")...)
	buf = strconv.AppendInt(buf, int64(totalWasted), 10)
	buf = append(buf, []byte(" bytes wasted")...)
	if totalSaved > 0 {
		buf = append(buf, []byte(", ")...)
		buf = strconv.AppendInt(buf, int64(totalSaved), 10)
		buf = append(buf, []byte(" bytes savable by reorder")...)
	}
	buf = append(buf, '\n')
	_, err := w.Write(buf)
	return err
}

func appendFields(buf []byte, fields []layout.Field) []byte {
	for _, field := range fields {
		buf = append(buf, []byte("     ")...)
		buf = append(buf, field.Name...)
		buf = append(buf, ' ')
		buf = append(buf, field.Type...)
		buf = append(buf, []byte(" (size: ")...)
		buf = strconv.AppendInt(buf, int64(field.Size), 10)
		buf = append(buf, []byte(", offset: ")...)
		buf = strconv.AppendInt(buf, int64(field.Offset), 10)
		buf = append(buf, []byte(", align: ")...)
		buf = strconv.AppendInt(buf, int64(field.Align), 10)
		buf = append(buf, ')', '\n')
	}
	return buf
}

func formatTable(w io.Writer, results []analyzer.Result) error {
	if len(results) == 0 {
		_, err := io.WriteString(w, "No struct alignment issues found.\n")
		return err
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	_, _ = io.WriteString(tw, "FILE\tSTRUCT\tLINE\tSEVERITY\tWASTED\tSAVED\tMESSAGE\n")
	_, _ = io.WriteString(tw, "----\t------\t----\t--------\t------\t-----\t-------\n")

	var line []byte
	for _, result := range results {
		for _, issue := range result.Issues {
			line = line[:0]
			line = append(line, result.File...)
			line = append(line, '\t')
			line = append(line, issue.StructName...)
			line = append(line, '\t')
			line = strconv.AppendInt(line, int64(issue.Line), 10)
			line = append(line, '\t')
			line = append(line, issue.Severity...)
			line = append(line, '\t')
			line = strconv.AppendInt(line, int64(issue.Wasted), 10)
			line = append(line, '\t')
			line = strconv.AppendInt(line, int64(issue.Saved), 10)
			line = append(line, '\t')
			line = append(line, issue.Message...)
			line = append(line, '\n')
			if _, err := tw.Write(line); err != nil {
				return err
			}
		}
	}
	return tw.Flush()
}

func formatJSON(w io.Writer, results []analyzer.Result) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(results)
}

func getSeverityIcon(severity string) string {
	switch severity {
	case "high":
		return "🔴"
	case "medium":
		return "🟡"
	case "low":
		return "🟢"
	case "info":
		return "ℹ️"
	default:
		return "⚪"
	}
}
