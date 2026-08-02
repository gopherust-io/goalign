package formatter

import (
	"encoding/json"
	"io"

	"github.com/gopherust-io/goalign/internal/analyzer"
)

// SARIF 2.1.0 minimal document for GitHub Code Scanning.
type sarifLog struct {
	Schema  string     `json:"$schema"`
	Version string     `json:"version"`
	Runs    []sarifRun `json:"runs"`
}

type sarifRun struct {
	Tool    sarifTool     `json:"tool"`
	Results []sarifResult `json:"results"`
}

type sarifTool struct {
	Driver sarifDriver `json:"driver"`
}

type sarifDriver struct {
	Name           string      `json:"name"`
	InformationURI string      `json:"informationUri"`
	Rules          []sarifRule `json:"rules"`
}

type sarifRule struct {
	ID               string `json:"id"`
	ShortDescription struct {
		Text string `json:"text"`
	} `json:"shortDescription"`
	HelpURI string `json:"helpUri,omitempty"`
}

type sarifResult struct {
	RuleID    string          `json:"ruleId"`
	Level     string          `json:"level"`
	Message   sarifMessage    `json:"message"`
	Locations []sarifLocation `json:"locations"`
}

type sarifMessage struct {
	Text string `json:"text"`
}

type sarifLocation struct {
	PhysicalLocation sarifPhysical `json:"physicalLocation"`
}

type sarifPhysical struct {
	ArtifactLocation sarifArtifact `json:"artifactLocation"`
	Region           sarifRegion   `json:"region"`
}

type sarifArtifact struct {
	URI string `json:"uri"`
}

type sarifRegion struct {
	StartLine int `json:"startLine"`
}

func formatSARIF(w io.Writer, results []analyzer.Result) error {
	rules := []sarifRule{{
		ID: "goalign/padding",
	}}
	rules[0].ShortDescription.Text = "Struct field order wastes padding bytes"
	rules[0].HelpURI = "https://github.com/gopherust-io/goalign"

	var out []sarifResult
	for _, r := range results {
		for _, iss := range r.Issues {
			out = append(out, sarifResult{
				RuleID:  "goalign/padding",
				Level:   sarifLevel(iss.Severity),
				Message: sarifMessage{Text: iss.Message},
				Locations: []sarifLocation{{
					PhysicalLocation: sarifPhysical{
						ArtifactLocation: sarifArtifact{URI: r.File},
						Region:           sarifRegion{StartLine: iss.Line},
					},
				}},
			})
		}
	}

	doc := sarifLog{
		Schema:  "https://json.schemastore.org/sarif-2.1.0.json",
		Version: "2.1.0",
		Runs: []sarifRun{{
			Tool: sarifTool{Driver: sarifDriver{
				Name:           "goalign",
				InformationURI: "https://github.com/gopherust-io/goalign",
				Rules:          rules,
			}},
			Results: out,
		}},
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(doc)
}

func sarifLevel(severity string) string {
	switch severity {
	case "high":
		return "error"
	case "medium":
		return "warning"
	default:
		return "note"
	}
}
