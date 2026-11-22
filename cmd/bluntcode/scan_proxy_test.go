package main

import (
	"testing"
)

// Flags the API cannot express stay refused — the refusal names them, so
// they must be detected before any remote call happens.
func TestRemoteUnsupportedNamesUnproxyableFlags(t *testing.T) {
	got := remoteUnsupported(scanConfig{watch: true, watchPoll: 2, format: scanFormatText})
	if len(got) != 2 {
		t.Fatalf("want 2 unsupported flags, got %v", got)
	}
	if !contains(got, "--watch") || !contains(got, "--watch-poll") {
		t.Fatalf("unsupported list must name the flags: %v", got)
	}
}

func TestRemoteUnsupportedPassesExpressibleFlags(t *testing.T) {
	cfg := scanConfig{
		profile: "deep", target: "https://localhost:3000", format: scanFormatSARIF,
		output: "out.sarif", saveBaseline: "base.sarif", jobs: 4, incremental: true,
	}
	if got := remoteUnsupported(cfg); len(got) != 0 {
		t.Fatalf("expressible flags must not block the proxy: %v", got)
	}
}

func TestRemoteUnsupportedRejectsGitHubFormat(t *testing.T) {
	if got := remoteUnsupported(scanConfig{format: scanFormatGitHub}); !contains(got, "--format github") {
		t.Fatalf("github format must be unproxyable: %v", got)
	}
}

func contains(list []string, want string) bool {
	for _, item := range list {
		if item == want {
			return true
		}
	}
	return false
}

func TestRemoteDocumentPathCoversExportFormats(t *testing.T) {
	cases := map[string]string{
		scanFormatJSON:     "findings.json",
		scanFormatSARIF:    "report.sarif",
		scanFormatCSV:      "findings.csv",
		scanFormatJSONL:    "findings.jsonl",
		scanFormatMarkdown: "report.md",
	}
	for format, want := range cases {
		if got := remoteDocumentPath(format); got != want {
			t.Errorf("remoteDocumentPath(%q) = %q, want %q", format, got, want)
		}
	}
	for _, format := range []string{"", scanFormatText, scanFormatGitHub} {
		if got := remoteDocumentPath(format); got != "" {
			t.Errorf("remoteDocumentPath(%q) = %q, want empty", format, got)
		}
	}
}

func TestAPIErrorEnvelopeParsing(t *testing.T) {
	body := []byte(`{"error":{"code":"SCAN_ALREADY_ACTIVE","message":"A scan is already active.","details":{"scan_id":"abc-123"}}}`)
	if got := apiErrorCode(body); got != "SCAN_ALREADY_ACTIVE" {
		t.Fatalf("apiErrorCode = %q", got)
	}
	if got := apiConflictScanID(body); got != "abc-123" {
		t.Fatalf("apiConflictScanID = %q", got)
	}
	if msg := apiErrorMessage(body, 409); msg != "A scan is already active. (SCAN_ALREADY_ACTIVE)" {
		t.Fatalf("apiErrorMessage = %q", msg)
	}
	// Non-envelope bodies fall back to the HTTP status text.
	if got := apiErrorCode([]byte(`not json`)); got != "" {
		t.Fatalf("apiErrorCode on non-JSON = %q", got)
	}
	if msg := apiErrorMessage([]byte(`not json`), 503); msg != "Service Unavailable" {
		t.Fatalf("apiErrorMessage fallback = %q", msg)
	}
}
