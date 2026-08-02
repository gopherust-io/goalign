package formatter

import (
	"encoding/json"
	"io"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/gopherust-io/goalign/internal/analyzer"
	"github.com/gopherust-io/goalign/internal/bytesconv"
	"github.com/gopherust-io/goalign/internal/layout"
)

const (
	ansiReset  = "\033[0m"
	ansiBold   = "\033[1m"
	ansiRed    = "\033[31m"
	ansiYellow = "\033[33m"
	ansiGreen  = "\033[32m"
	ansiCyan   = "\033[36m"
	ansiDim    = "\033[2m"
)

// Format writes analysis results to w in the given format (text|table|json|sarif).
func Format(w io.Writer, results []analyzer.Result, format string) error {
	switch strings.ToLower(format) {
	case "json":
		return formatJSON(w, results)
	case "table":
		return formatTable(w, results)
	case "sarif":
		return formatSARIF(w, results)
	default:
		return formatText(w, results, useColor(w))
	}
}

// FormatFixSummary writes a one-line fix summary.
func FormatFixSummary(w io.Writer, files, structs, bytesSaved int) error {
	color := useColor(w)
	var buf []byte
	if structs == 0 {
		buf = append(buf, bytesconv.StringToBytes("No structs needed fixing.\n")...)
		_, err := w.Write(buf)
		return err
	}
	if color {
		buf = append(buf, ansiGreen...)
		buf = append(buf, ansiBold...)
	}
	buf = append(buf, bytesconv.StringToBytes("Fixed ")...)
	buf = strconv.AppendInt(buf, int64(structs), 10)
	buf = append(buf, bytesconv.StringToBytes(" structs in ")...)
	buf = strconv.AppendInt(buf, int64(files), 10)
	buf = append(buf, bytesconv.StringToBytes(" files")...)
	if bytesSaved > 0 {
		buf = append(buf, bytesconv.StringToBytes(", saved ")...)
		buf = strconv.AppendInt(buf, int64(bytesSaved), 10)
		buf = append(buf, bytesconv.StringToBytes(" bytes")...)
	}
	if color {
		buf = append(buf, ansiReset...)
	}
	buf = append(buf, '\n')
	_, err := w.Write(buf)
	return err
}

func useColor(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

func formatText(w io.Writer, results []analyzer.Result, color bool) error {
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
		if color {
			buf = append(buf, ansiBold...)
			buf = append(buf, ansiCyan...)
		}
		buf = append(buf, result.File...)
		if color {
			buf = append(buf, ansiReset...)
		}
		buf = append(buf, '\n')
		for i := 0; i < len(result.File); i++ {
			buf = append(buf, '-')
		}
		buf = append(buf, '\n')

		for _, issue := range result.Issues {
			totalIssues++
			totalWasted += issue.Wasted
			totalSaved += issue.Saved

			buf = appendSeverity(buf, issue.Severity, color)
			buf = append(buf, ' ')
			if color {
				buf = append(buf, ansiBold...)
			}
			buf = append(buf, issue.StructName...)
			if color {
				buf = append(buf, ansiReset...)
			}
			buf = append(buf, bytesconv.StringToBytes("  line ")...)
			buf = strconv.AppendInt(buf, int64(issue.Line), 10)
			buf = append(buf, '\n')

			buf = append(buf, bytesconv.StringToBytes("  ")...)
			if color {
				buf = append(buf, ansiDim...)
			}
			buf = append(buf, issue.Message...)
			if color {
				buf = append(buf, ansiReset...)
			}
			buf = append(buf, '\n')

			showCLine := notesHavePrefix(issue.Notes, "false-share") || notesHavePrefix(issue.Notes, "cacheguard")
			clineSize := issue.CacheLine
			if clineSize <= 0 {
				clineSize = layout.DefaultCacheLine
			}
			if len(issue.Fields) > 0 {
				buf = append(buf, bytesconv.StringToBytes("  Current\n")...)
				buf = appendFieldTable(buf, issue.Fields, showCLine, clineSize)
			}
			if len(issue.Suggested) > 0 && (issue.Saved > 0 || !sameFieldNames(issue.Fields, issue.Suggested)) {
				buf = append(buf, bytesconv.StringToBytes("  Suggested")...)
				if issue.Saved > 0 {
					buf = append(buf, bytesconv.StringToBytes("  (saves ")...)
					buf = strconv.AppendInt(buf, int64(issue.Saved), 10)
					buf = append(buf, bytesconv.StringToBytes(" bytes)")...)
				}
				buf = append(buf, '\n')
				buf = appendFieldTable(buf, issue.Suggested, showCLine, clineSize)
			}
			for _, note := range issue.Notes {
				buf = append(buf, bytesconv.StringToBytes("  note: ")...)
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

	if color {
		buf = append(buf, ansiBold...)
	}
	buf = append(buf, bytesconv.StringToBytes("Summary: ")...)
	if color {
		buf = append(buf, ansiReset...)
	}
	buf = strconv.AppendInt(buf, int64(totalIssues), 10)
	buf = append(buf, bytesconv.StringToBytes(" issues, ")...)
	buf = strconv.AppendInt(buf, int64(totalWasted), 10)
	buf = append(buf, bytesconv.StringToBytes(" bytes wasted")...)
	if totalSaved > 0 {
		buf = append(buf, bytesconv.StringToBytes(", ")...)
		buf = strconv.AppendInt(buf, int64(totalSaved), 10)
		buf = append(buf, bytesconv.StringToBytes(" bytes savable")...)
	}
	buf = append(buf, '\n')
	_, err := w.Write(buf)
	return err
}

func sameFieldNames(a, b []layout.Field) bool {
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

func appendSeverity(buf []byte, severity string, color bool) []byte {
	var label string
	switch severity {
	case "high":
		label = "HIGH"
		if color {
			buf = append(buf, ansiRed...)
			buf = append(buf, ansiBold...)
		}
	case "medium":
		label = "MED"
		if color {
			buf = append(buf, ansiYellow...)
			buf = append(buf, ansiBold...)
		}
	case "low":
		label = "LOW"
		if color {
			buf = append(buf, ansiGreen...)
		}
	case "info":
		label = "INFO"
		if color {
			buf = append(buf, ansiCyan...)
		}
	default:
		label = strings.ToUpper(severity)
	}
	buf = append(buf, label...)
	if color {
		buf = append(buf, ansiReset...)
	}
	return buf
}

func notesHavePrefix(notes []string, prefix string) bool {
	for _, n := range notes {
		if strings.HasPrefix(n, prefix) {
			return true
		}
	}
	return false
}

func appendFieldTable(buf []byte, fields []layout.Field, showCLine bool, cacheLine int) []byte {
	if cacheLine <= 0 {
		cacheLine = layout.DefaultCacheLine
	}
	var twBuf strings.Builder
	tw := tabwriter.NewWriter(&twBuf, 0, 0, 2, ' ', 0)
	if showCLine {
		_, _ = io.WriteString(tw, "    NAME\tTYPE\tSIZE\tOFFSET\tALIGN\tCLINE\n")
	} else {
		_, _ = io.WriteString(tw, "    NAME\tTYPE\tSIZE\tOFFSET\tALIGN\n")
	}
	for _, field := range fields {
		_, _ = io.WriteString(tw, "    ")
		_, _ = io.WriteString(tw, field.Name)
		_, _ = io.WriteString(tw, "\t")
		_, _ = io.WriteString(tw, field.Type)
		_, _ = io.WriteString(tw, "\t")
		_, _ = io.WriteString(tw, strconv.Itoa(field.Size))
		_, _ = io.WriteString(tw, "\t")
		_, _ = io.WriteString(tw, strconv.Itoa(field.Offset))
		_, _ = io.WriteString(tw, "\t")
		_, _ = io.WriteString(tw, strconv.Itoa(field.Align))
		if showCLine {
			_, _ = io.WriteString(tw, "\t")
			_, _ = io.WriteString(tw, strconv.Itoa(field.Offset/cacheLine))
		}
		_, _ = io.WriteString(tw, "\n")
	}
	_ = tw.Flush()
	buf = append(buf, twBuf.String()...)
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
