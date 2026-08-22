package reports

import (
	"bluntcode/internal/analyzers"
	"net/url"
	"path/filepath"
	"strings"
)

// SARIFSchemaURI and sarifVersion identify the emitted log as SARIF 2.1.0,
// the version consumed by VS Code, GitHub code scanning, and SonarQube.
const (
	SARIFSchemaURI = "https://json.schemastore.org/sarif-2.1.0.json"
	sarifVersion   = "2.1.0"
	sarifDriverURI = "https://github.com/sanketpatel32/Blunt-code"
)

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
	Version        string      `json:"version,omitempty"`
	InformationURI string      `json:"informationUri"`
	Rules          []sarifRule `json:"rules"`
}
type sarifRule struct {
	ID               string    `json:"id"`
	ShortDescription sarifText `json:"shortDescription"`
	HelpURI          string    `json:"helpUri,omitempty"`
}
type sarifResult struct {
	RuleID    string          `json:"ruleId"`
	RuleIndex int             `json:"ruleIndex"`
	Level     string          `json:"level"`
	Message   sarifText       `json:"message"`
	Locations []sarifLocation `json:"locations,omitempty"`
}
type sarifText struct {
	Text string `json:"text"`
}
type sarifLocation struct {
	PhysicalLocation sarifPhysicalLocation `json:"physicalLocation"`
}
type sarifPhysicalLocation struct {
	ArtifactLocation sarifArtifactLocation `json:"artifactLocation"`
	Region           sarifRegion           `json:"region"`
}
type sarifArtifactLocation struct {
	URI string `json:"uri"`
}
type sarifRegion struct {
	StartLine   int `json:"startLine,omitempty"`
	StartColumn int `json:"startColumn,omitempty"`
	EndLine     int `json:"endLine,omitempty"`
	EndColumn   int `json:"endColumn,omitempty"`
}

// SARIF renders the report model as a SARIF 2.1.0 log. Artifact URIs are
// workspace-relative with forward slashes and no leading slash, which is what
// GitHub code scanning resolves against the repository root; project-level
// findings carry no location at all.
func SARIF(m Model) sarifLog {
	rules := make([]sarifRule, 0)
	indexes := map[string]int{}
	results := make([]sarifResult, 0, len(m.Findings))
	for _, f := range m.Findings {
		index, seen := indexes[f.RuleID]
		if !seen {
			index = len(rules)
			indexes[f.RuleID] = index
			rules = append(rules, sarifRule{ID: f.RuleID, ShortDescription: sarifText{Text: ruleName(f)}})
		}
		if uri := strings.TrimSpace(f.DocumentationURL); uri != "" && rules[index].HelpURI == "" {
			rules[index].HelpURI = uri
		}
		result := sarifResult{RuleID: f.RuleID, RuleIndex: index, Level: sarifLevel(f.Severity), Message: sarifText{Text: sarifMessage(f)}}
		if uri := sarifArtifactURI(f.RelativePath); uri != "" {
			result.Locations = []sarifLocation{{PhysicalLocation: sarifPhysicalLocation{
				ArtifactLocation: sarifArtifactLocation{URI: uri},
				Region:           sarifRegion{StartLine: f.StartLine, StartColumn: f.StartColumn, EndLine: f.EndLine, EndColumn: f.EndColumn},
			}}}
		}
		results = append(results, result)
	}
	return sarifLog{Schema: SARIFSchemaURI, Version: sarifVersion, Runs: []sarifRun{{
		Tool:    sarifTool{Driver: sarifDriver{Name: "Blunt Code", Version: m.BluntCodeVersion, InformationURI: sarifDriverURI, Rules: rules}},
		Results: results,
	}}}
}

// sarifLevel maps the five normalized severities onto the three SARIF levels;
// an unmapped severity stays visible as a warning rather than being dropped.
func sarifLevel(severity analyzers.Severity) string {
	switch severity {
	case analyzers.SeverityCritical, analyzers.SeverityHigh:
		return "error"
	case analyzers.SeverityMedium, analyzers.SeverityLow:
		return "warning"
	case analyzers.SeverityInfo:
		return "note"
	default:
		return "warning"
	}
}

// ruleName prefers a human title for the rule descriptor and falls back to the
// rule id so shortDescription is never empty.
func ruleName(f analyzers.Finding) string {
	if title := strings.TrimSpace(f.Title); title != "" {
		return title
	}
	return f.RuleID
}

// sarifMessage keeps the finding message first and appends remediation as a
// second sentence when the analyzer supplied one.
func sarifMessage(f analyzers.Finding) string {
	text := strings.TrimSpace(f.Message)
	remediation := strings.TrimSpace(f.Remediation)
	if remediation == "" {
		return text
	}
	switch {
	case text == "":
		return remediation
	case strings.HasSuffix(text, ".") || strings.HasSuffix(text, "!") || strings.HasSuffix(text, "?"):
		return text + " " + remediation
	default:
		return text + ". " + remediation
	}
}

// sarifArtifactURI returns the workspace-relative, URI-encoded artifact path,
// or an empty string for project-level findings that name no file. "." is also
// treated as project-level because some analyzers use it for whole-project
// results.
func sarifArtifactURI(path string) string {
	path = strings.TrimPrefix(filepath.ToSlash(strings.TrimSpace(path)), "./")
	path = strings.TrimPrefix(path, "/")
	if path == "" || path == "." {
		return ""
	}
	return (&url.URL{Path: path}).EscapedPath()
}
