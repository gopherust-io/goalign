package formatter_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/gopherust-io/goalign/internal/analyzer"
	"github.com/gopherust-io/goalign/internal/formatter"
)

func TestFormatSARIF(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	err := formatter.Format(&buf, []analyzer.Result{{
		File: "a.go",
		Issues: []analyzer.Issue{{
			StructName: "S",
			Message:    "padding",
			Severity:   "high",
			Line:       3,
			Wasted:     16,
		}},
	}}, "sarif")
	if err != nil {
		t.Fatal(err)
	}
	s := buf.String()
	if !strings.Contains(s, `"version": "2.1.0"`) || !strings.Contains(s, "goalign/padding") {
		t.Fatalf("sarif:\n%s", s)
	}
	var doc map[string]any
	if err := json.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
}
