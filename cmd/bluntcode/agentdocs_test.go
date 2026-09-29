package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"bluntcode/internal/build"
)

// The agent guide ships in seven tracked copies, and the binary serves three
// of them: cmd/bluntcode/llm.txt is embedded for `bluntcode llm`, while
// cmd/bluntcode/static/{llm,llms}.txt are embedded into the static file
// server for GET /llm.txt and GET /llms.txt (build.ps1 regenerates the static
// pair from web/public, which mirrors the repo-root canon). Three releases
// shipped with these copies quoting stale versions before this guard existed,
// so an agent asking the binary for its own docs got a Version line many
// releases behind build.Version. Root llm.txt/llms.txt are canonical; every
// mirror must match them modulo line endings and every copy must quote the
// current version. Same contract for CHANGELOG.md and web/package.json: a
// version bump that misses one of them fails here instead of in a release.

const agentDocRepoRoot = "../.."

var agentDocLLMCopies = []string{
	"llm.txt",
	"cmd/bluntcode/llm.txt",
	"cmd/bluntcode/static/llm.txt",
	"web/public/llm.txt",
}

var agentDocLLMSCopies = []string{
	"llms.txt",
	"cmd/bluntcode/static/llms.txt",
	"web/public/llms.txt",
}

func readAgentDoc(t *testing.T, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(agentDocRepoRoot, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return normalizeDoc(string(b))
}

func normalizeDoc(s string) string {
	return strings.ReplaceAll(s, "\r\n", "\n")
}

func agentDocVersionLine(doc string) string {
	for _, line := range strings.Split(doc, "\n") {
		if strings.Contains(line, "**Version:**") {
			return line
		}
	}
	return ""
}

func TestAgentDocsMirrorCanonical(t *testing.T) {
	canonLLM := readAgentDoc(t, "llm.txt")
	if normalizeDoc(llmText) != canonLLM {
		t.Error("embedded llmText (go:embed of cmd/bluntcode/llm.txt) differs from canonical llm.txt")
	}
	for _, rel := range agentDocLLMCopies[1:] {
		if got := readAgentDoc(t, rel); got != canonLLM {
			t.Errorf("%s has drifted from canonical llm.txt; copy the root file over it", rel)
		}
	}
	canonLLMS := readAgentDoc(t, "llms.txt")
	for _, rel := range agentDocLLMSCopies[1:] {
		if got := readAgentDoc(t, rel); got != canonLLMS {
			t.Errorf("%s has drifted from canonical llms.txt; copy the root file over it", rel)
		}
	}
}

func TestAgentDocsQuoteCurrentVersion(t *testing.T) {
	want := "- **Version:** " + build.Version + " "
	for _, rel := range append(append([]string{}, agentDocLLMCopies...), agentDocLLMSCopies...) {
		got := agentDocVersionLine(readAgentDoc(t, rel))
		if !strings.HasPrefix(got, want) {
			t.Errorf("%s Version line is %q, want prefix %q — bump it whenever build.Version moves", rel, got, want)
		}
	}
	zipLine := "BluntCode-" + build.Version + "-windows-amd64.zip"
	if !strings.Contains(readAgentDoc(t, "llm.txt"), zipLine) {
		t.Errorf("llm.txt does not name the current release zip %s", zipLine)
	}
}

func TestVersionSurfacesAgree(t *testing.T) {
	if changelog := readAgentDoc(t, "CHANGELOG.md"); !strings.Contains(changelog, "## ["+build.Version+"]") {
		t.Errorf("CHANGELOG.md has no ## [%s] section — promote [Unreleased] when bumping build.Version", build.Version)
	}
	b, err := os.ReadFile(filepath.Join(agentDocRepoRoot, "web", "package.json"))
	if err != nil {
		t.Fatalf("read web/package.json: %v", err)
	}
	var pkg map[string]any
	if err := json.Unmarshal(b, &pkg); err != nil {
		t.Fatalf("parse web/package.json: %v", err)
	}
	ver, _ := pkg["version"].(string)
	if ver != build.Version {
		t.Errorf("web/package.json version is %q, want %q", ver, build.Version)
	}
}
