package main

import (
	"bytes"
	"regexp"
	"strings"
	"testing"
)

func TestPrintHelpListsEverySubcommand(t *testing.T) {
	var out bytes.Buffer
	printHelp(&out)
	text := out.String()
	for _, want := range []string{scanUsage, pruneUsage, doctorUsage, configUsage, "bluntcode [path]", version} {
		if !strings.Contains(text, want) {
			t.Fatalf("help output missing %q:\n%s", want, text)
		}
	}
}

func TestParseScanFlagsGithubCapValidation(t *testing.T) {
	var errOut bytes.Buffer
	cfg, err := parseScanFlags([]string{"--format", "github", "--github-cap", "3", `C:\proj`}, &errOut)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if cfg.githubCap != 3 {
		t.Fatalf("githubCap = %d, want 3", cfg.githubCap)
	}
	if _, err := parseScanFlags([]string{"--format", "github", "--github-cap", "0", `C:\proj`}, &errOut); err == nil {
		t.Fatalf("cap below 1 must be rejected")
	}
	if _, err := parseScanFlags([]string{"--format", "github", "--github-cap", "99", `C:\proj`}, &errOut); err == nil {
		t.Fatalf("cap above 50 must be rejected")
	}
}

// TestScanUsageAndManualDocumentEveryScanFlag is the flag-contract test: the
// FlagSet's own defaults dump is ground truth for accepted flags, and every
// one of them must appear in both the usage line and the CLI manual's scan
// section so the interactive manual can never drift from the parser again.
func TestScanUsageAndManualDocumentEveryScanFlag(t *testing.T) {
	var errOut bytes.Buffer
	if _, err := parseScanFlags([]string{"-h"}, &errOut); err == nil {
		t.Fatal("-h must surface flag.ErrHelp, not parse successfully")
	}
	names := regexp.MustCompile(`(?m)^\s+-([a-z][a-z0-9-]+)`).FindAllStringSubmatch(errOut.String(), -1)
	if len(names) < 15 {
		t.Fatalf("expected the full scan flag set in the defaults dump, got %d flags:\n%s", len(names), errOut.String())
	}
	var stdout, stderr bytes.Buffer
	if code := runCLIDocs([]string{"scan"}, &stdout, &stderr); code != 0 {
		t.Fatalf("runCLIDocs(scan) returned %d", code)
	}
	manual := stdout.String()
	for _, match := range names {
		name := match[1]
		if !strings.Contains(scanUsage, "--"+name) {
			t.Errorf("flag --%s is accepted by parseScanFlags but missing from scanUsage", name)
		}
		if !strings.Contains(manual, "--"+name) {
			t.Errorf("flag --%s is accepted by parseScanFlags but missing from the CLI manual scan section", name)
		}
	}
}
