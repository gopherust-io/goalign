package formatter

import (
	"encoding/json"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/nekruzjm/goalign/internal/analyzer"
)

// Format formats the analysis results based on the specified format
func Format(results []analyzer.Result, format string) (string, error) {
	switch strings.ToLower(format) {
	case "json":
		return formatJSON(results)
	case "table":
		return formatTable(results), nil
	case "text":
		fallthrough
	default:
		return formatText(results), nil
	}
}

func formatText(results []analyzer.Result) string {
	if len(results) == 0 {
		return "No struct alignment issues found.\n"
	}

	var output strings.Builder
	totalIssues := 0
	totalWasted := 0

	for _, result := range results {
		if len(result.Issues) == 0 {
			continue
		}

		output.WriteString(fmt.Sprintf("\n📁 %s\n", result.File))
		output.WriteString(strings.Repeat("=", len(result.File)+4) + "\n")

		for _, issue := range result.Issues {
			totalIssues++
			totalWasted += issue.Wasted

			severityIcon := getSeverityIcon(issue.Severity)
			output.WriteString(fmt.Sprintf("%s %s (line %d)\n", severityIcon, issue.StructName, issue.Line))
			output.WriteString(fmt.Sprintf("   %s\n", issue.Message))

			if len(issue.Fields) > 0 {
				output.WriteString("   Fields:\n")
				for _, field := range issue.Fields {
					output.WriteString(fmt.Sprintf("     %s %s (size: %d, offset: %d, align: %d)\n",
						field.Name, field.Type, field.Size, field.Offset, field.Align))
				}
			}
			output.WriteString("\n")
		}
	}

	output.WriteString(fmt.Sprintf("\n📊 Summary: %d issues found, %d bytes wasted\n", totalIssues, totalWasted))
	return output.String()
}

func formatTable(results []analyzer.Result) string {
	if len(results) == 0 {
		return "No struct alignment issues found.\n"
	}

	var output strings.Builder
	w := tabwriter.NewWriter(&output, 0, 0, 2, ' ', 0)

	fmt.Fprintln(w, "FILE\tSTRUCT\tLINE\tSEVERITY\tWASTED\tMESSAGE")
	fmt.Fprintln(w, "----\t------\t----\t--------\t------\t-------")

	for _, result := range results {
		for _, issue := range result.Issues {
			fmt.Fprintf(w, "%s\t%s\t%d\t%s\t%d\t%s\n",
				result.File, issue.StructName, issue.Line, issue.Severity, issue.Wasted, issue.Message)
		}
	}

	w.Flush()
	return output.String()
}

func formatJSON(results []analyzer.Result) (string, error) {
	jsonData, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return "", err
	}
	return string(jsonData) + "\n", nil
}

func getSeverityIcon(severity string) string {
	switch severity {
	case "high":
		return "🔴"
	case "medium":
		return "🟡"
	case "low":
		return "🟢"
	default:
		return "⚪"
	}
}
