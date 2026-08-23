package reports

import (
	"bytes"
	"encoding/json"

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
	// SARIFFingerprintKey namespaces Blunt Code's finding fingerprint inside a
	// result's partialFingerprints bag so `bluntcode scan --baseline <sarif>`
	// can recognize its own exports (and foreign fingerprints stay readable
	// through the same property bag).
	SARIFFingerprintKey = "bluntcode/v1"
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
	RuleID              string            `json:"ruleId"`
	RuleIndex           int               `json:"ruleIndex"`
	Level               string            `json:"level"`
	Message             sarifText         `json:"message"`
	Locations           []sarifLocation   `json:"locations,omitempty"`
	PartialFingerprints map[string]string `json:"partialFingerprints,omitempty"`
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
// findings carry no location at all. Each result carries the finding's
// fingerprint under partialFingerprints[SARIFFingerprintKey] so the log can be
// fed back as a `bluntcode scan --baseline` baseline (see ReadSARIF).
//
// Analyzer-derived strings (rule ids, titles, messages, remediation, help
// URIs) pass through scrubControls before they are marshaled: encoding/json
// escapes NUL, BEL, and ESC as \u00XX but writes DEL through raw, so without
// the scrub a hostile message smuggles terminal escape codes and DEL bytes
// into every SARIF consumer. The scrub matches the Markdown, HTML, and CSV
// exports: the hostile C0 controls and DEL become spaces; tab, LF, and CR
// are kept because JSON quotes them losslessly.
func SARIF(m Model) sarifLog {
	rules := make([]sarifRule, 0)
	indexes := map[string]int{}
	results := make([]sarifResult, 0, len(m.Findings))
	for _, f := range m.Findings {
		index, seen := indexes[f.RuleID]
		if !seen {
			index = len(rules)
			indexes[f.RuleID] = index
			rules = append(rules, sarifRule{ID: scrubControls(f.RuleID), ShortDescription: sarifText{Text: ruleName(f)}})
		}
		if uri := scrubControls(strings.TrimSpace(f.DocumentationURL)); uri != "" && rules[index].HelpURI == "" {
			rules[index].HelpURI = uri
		}
		result := sarifResult{RuleID: scrubControls(f.RuleID), RuleIndex: index, Level: sarifLevel(f.Severity), Message: sarifText{Text: sarifMessage(f)}}
		// The fingerprint is Blunt Code's own sha256 hex, but it is scrubbed
		// like every other result string so a hostile finding can never smuggle
		// control bytes through the fingerprint bag.
		if fingerprint := scrubControls(strings.TrimSpace(f.Fingerprint)); fingerprint != "" {
			result.PartialFingerprints = map[string]string{SARIFFingerprintKey: fingerprint}
		}
		if uri := sarifArtifactURI(f.RelativePath); uri != "" {
			result.Locations = []sarifLocation{{PhysicalLocation: sarifPhysicalLocation{
				ArtifactLocation: sarifArtifactLocation{URI: uri},
				Region:           sarifRegionFor(f),
			}}}
		}
		results = append(results, result)
	}
	return sarifLog{Schema: SARIFSchemaURI, Version: sarifVersion, Runs: []sarifRun{{
		Tool:    sarifTool{Driver: sarifDriver{Name: "Blunt Code", Version: m.BluntCodeVersion, InformationURI: sarifDriverURI, Rules: rules}},
		Results: results,
	}}}
}

// SARIFBytes serializes the SARIF log exactly the way the API download route
// (GET /api/v1/scans/{id}/report.sarif) serves it: one json.Encoder pass with
// default settings — compact (no indentation), default HTML escaping —
// terminated by the single LF Encode appends. `bluntcode scan --format sarif`
// writes these same bytes to stdout, so a CLI-produced baseline file and a
// server download are interchangeable byte for byte, and consecutive
// watch-mode rescans come out as newline-separated complete documents (the
// same convention the JSON format's watch behavior uses). Like JSON() the
// render is infallible: the log holds only strings, numbers, and slices
// thereof, so Encode cannot fail (the error is ignored for parity with the
// route, which ignores it too).
func SARIFBytes(m Model) []byte {
	var buf bytes.Buffer
	_ = json.NewEncoder(&buf).Encode(SARIF(m))
	return buf.Bytes()
}

// sarifRegion projects a finding's stored positions onto the SARIF 2.1.0
// region constraints. Analyzer output is untrusted: a hostile or buggy
// analyzer can emit startColumn without startLine, end positions before their
// starts, or negative values, any of which produces an invalid log that SARIF
// consumers reject. Rather than refuse the export, contradictory values are
// dropped (never invented): startColumn and the end positions only survive
// when the coordinates they depend on are present and consistent.
func sarifRegionFor(f analyzers.Finding) sarifRegion {
	r := sarifRegion{StartLine: f.StartLine, StartColumn: f.StartColumn, EndLine: f.EndLine, EndColumn: f.EndColumn}
	if r.StartLine <= 0 {
		// Without a start line no other coordinate is expressible: SARIF
		// requires startLine whenever startColumn or endLine is present.
		return sarifRegion{}
	}
	if r.StartColumn < 0 {
		r.StartColumn = 0
	}
	if r.EndLine < 0 || r.EndLine < r.StartLine {
		// An end before the start is invalid per spec (endLine >= startLine);
		// drop both end coordinates rather than clamp to a position the
		// analyzer never reported.
		r.EndLine, r.EndColumn = 0, 0
	}
	switch {
	case r.EndLine == 0:
		// endColumn is only meaningful together with endLine.
		r.EndColumn = 0
	case r.EndColumn < 0:
		r.EndColumn = 0
	case r.EndLine == r.StartLine && r.StartColumn > 0 && r.EndColumn < r.StartColumn:
		// A same-line end column before the start column is invalid per spec.
		r.EndColumn = 0
	}
	return r
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
// rule id so shortDescription is never empty. Both forms are scrubbed so a
// hostile title cannot carry control bytes into the descriptor.
func ruleName(f analyzers.Finding) string {
	if title := strings.TrimSpace(f.Title); title != "" {
		return scrubControls(title)
	}
	return scrubControls(f.RuleID)
}

// sarifMessage keeps the finding message first and appends remediation as a
// second sentence when the analyzer supplied one. Both halves are scrubbed of
// hostile control bytes after trimming; the scrub may leave the interior
// spaces that replaced them.
func sarifMessage(f analyzers.Finding) string {
	text := scrubControls(strings.TrimSpace(f.Message))
	remediation := scrubControls(strings.TrimSpace(f.Remediation))
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
	if path == ".." || strings.HasPrefix(path, "../") || strings.Contains(path, "/../") {
		return ""
	}
	return (&url.URL{Path: path}).EscapedPath()
}
