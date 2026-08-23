package reports

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"bluntcode/internal/analyzers"
)

// fingerprinted builds a finding with the V1 fingerprint the rest of the
// pipeline would pin on it.
func fingerprinted(rule, path, message string, severity analyzers.Severity) analyzers.Finding {
	f := analyzers.Finding{AnalyzerID: "ruff", RuleID: rule, RelativePath: path, Message: message, Severity: severity, Category: analyzers.CategoryCorrectness}
	f.SetFingerprint()
	return f
}

// marshalSARIF renders findings through the export pipeline (Build + SARIF)
// and returns the JSON bytes a CI would store as a baseline file.
func marshalSARIF(t *testing.T, findings []analyzers.Finding) []byte {
	t.Helper()
	data, err := json.Marshal(SARIF(Build(Input{Findings: findings})))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// TestSARIFWritesPartialFingerprints pins the writer half of the baseline
// contract: every fingerprinted result carries its exact fingerprint under the
// Blunt Code key, and findings without a fingerprint omit the bag entirely.
func TestSARIFWritesPartialFingerprints(t *testing.T) {
	withFingerprint := fingerprinted("F401", "src/a.py", "unused import", analyzers.SeverityMedium)
	bare := analyzers.Finding{AnalyzerID: "ruff", RuleID: "E501", Severity: analyzers.SeverityLow, Message: "m", RelativePath: "src/b.py"}
	results := sarifAllResults(t, sarifFirstRun(t, sarifDocument(t, Input{Findings: []analyzers.Finding{withFingerprint, bare}})))
	if len(results) != 2 {
		t.Fatalf("unexpected results: %#v", results)
	}
	first, ok := results[0]["partialFingerprints"].(map[string]any)
	if !ok {
		t.Fatalf("fingerprinted result must carry partialFingerprints: %#v", results[0])
	}
	if first[SARIFFingerprintKey] != withFingerprint.Fingerprint {
		t.Fatalf("fingerprint = %v, want %q", first[SARIFFingerprintKey], withFingerprint.Fingerprint)
	}
	if _, present := results[1]["partialFingerprints"]; present {
		t.Fatalf("finding without a fingerprint must omit the bag: %#v", results[1])
	}
}

// TestReadSARIFRoundTripMatchesFindingFingerprints is the write-to-read
// contract `bluntcode scan --baseline <sarif>` depends on: every fingerprint
// the writer emits survives a reader pass byte for byte.
func TestReadSARIFRoundTripMatchesFindingFingerprints(t *testing.T) {
	findings := []analyzers.Finding{
		fingerprinted("F401", "src/a.py", "unused import", analyzers.SeverityMedium),
		fingerprinted("S1", "src/deep/nested b.py", "hardcoded credential", analyzers.SeverityCritical),
		fingerprinted("project:coverage", "", "coverage below threshold", analyzers.SeverityInfo),
	}
	parsed, err := ReadSARIF(bytes.NewReader(marshalSARIF(t, findings)))
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if len(parsed) != len(findings) {
		t.Fatalf("parsed %d findings, want %d: %#v", len(parsed), len(findings), parsed)
	}
	wantLevels := map[string]string{
		findings[0].Fingerprint: "warning",
		findings[1].Fingerprint: "error",
		findings[2].Fingerprint: "note",
	}
	seen := map[string]bool{}
	for _, result := range parsed {
		seen[result.Fingerprint] = true
		if level := wantLevels[result.Fingerprint]; level != "" && result.Level != level {
			t.Errorf("level for %s = %q, want %q", result.Fingerprint, result.Level, level)
		}
	}
	for _, finding := range findings {
		if !seen[finding.Fingerprint] {
			t.Errorf("fingerprint of rule %s missing from parsed results", finding.RuleID)
		}
	}
}

func TestReadSARIFAcceptsForeignLogs(t *testing.T) {
	cases := []struct {
		name string
		log  string
		want []string // fingerprints in first-seen order
	}{
		{
			name: "stable fingerprints property",
			log:  `{"version":"2.1.0","runs":[{"results":[{"level":"error","fingerprints":{"primaryLocationLineHash":"abc"}},{"level":"note"}]}]}`,
			want: []string{"abc"},
		},
		{
			name: "bluntcode key wins over other keys",
			log:  `{"version":"2.1.0","runs":[{"results":[{"partialFingerprints":{"aaa":"other","bluntcode/v1":"ours"}}]}]}`,
			want: []string{"ours"},
		},
		{
			name: "bluntcode key in stable bag wins over partial bag",
			log:  `{"version":"2.1.0","runs":[{"results":[{"fingerprints":{"bluntcode/v1":"stable"},"partialFingerprints":{"zzz":"partial"}}]}]}`,
			want: []string{"stable"},
		},
		{
			name: "stable bag preferred over partial bag without the key",
			log:  `{"version":"2.1.0","runs":[{"results":[{"fingerprints":{"zzz":"stable-any"},"partialFingerprints":{"aaa":"partial-any"}}]}]}`,
			want: []string{"stable-any"},
		},
		{
			name: "first sorted key is deterministic fallback",
			log:  `{"version":"2.1.0","runs":[{"results":[{"partialFingerprints":{"b":"second","a":"first"}}]}]}`,
			want: []string{"first"},
		},
		{
			name: "multiple runs and blank fingerprints skipped",
			log:  `{"version":"2.1.0","runs":[{"results":[{"fingerprints":{"x":"one"}},{"fingerprints":{"x":"  "}}]},{"results":[{"fingerprints":{"y":"two"}}]},{"results":null}]}`,
			want: []string{"one", "two"},
		},
		{
			name: "no runs and no results are empty but valid",
			log:  `{"version":"2.1.0","runs":[]}`,
			want: nil,
		},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			parsed, err := ReadSARIF(strings.NewReader(item.log))
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			if len(parsed) != len(item.want) {
				t.Fatalf("parsed %#v, want %v", parsed, item.want)
			}
			for i, want := range item.want {
				if parsed[i].Fingerprint != want {
					t.Errorf("parsed[%d] = %#v, want fingerprint %q", i, parsed[i], want)
				}
			}
		})
	}
}

func TestReadSARIFRejectsInvalidLogs(t *testing.T) {
	cases := []struct {
		name string
		log  string
		want string
	}{
		{"not json", "this is not json", "invalid SARIF"},
		{"json but not a log", `{"runs":[]}`, `version "" is not 2.1.0`},
		{"wrong version", `{"version":"2.0.0","runs":[]}`, `version "2.0.0" is not 2.1.0`},
		{"empty input", "", "invalid SARIF"},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			_, err := ReadSARIF(strings.NewReader(item.log))
			if err == nil {
				t.Fatalf("%q accepted", item.log)
			}
			if !strings.Contains(err.Error(), item.want) {
				t.Errorf("error = %q, want substring %q", err.Error(), item.want)
			}
		})
	}
}
