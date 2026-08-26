package main

import (
	"bytes"
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
