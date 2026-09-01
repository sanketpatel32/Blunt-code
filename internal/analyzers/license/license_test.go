package license

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"bluntcode/internal/analyzers"
)

func TestClassifyText(t *testing.T) {
	cases := map[string]string{
		"Permission is hereby granted, free of charge, to any person obtaining a copy":                                "MIT",
		"                                   Apache License\n                           Version 2.0, January 2004":     "Apache-2.0",
		"                    GNU GENERAL PUBLIC LICENSE\n                       Version 3, 29 June 2007":              "GPL-3.0",
		"                    GNU GENERAL Public LICENSE\n                       Version 2, June 1991":                 "GPL-2.0",
		"GNU AFFERO GENERAL PUBLIC LICENSE\nVersion 3, 19 November 2007":                                              "AGPL-3.0",
		"GNU LESSER GENERAL PUBLIC LICENSE\nVersion 2.1, February 1999":                                               "LGPL-2.1",
		"Mozilla Public License Version 2.0":                                                                          "MPL-2.0",
		"Redistribution and use in source and binary forms... may not be used to endorse or promote derived products": "BSD-3-Clause",
		"Permission to use, copy, modify, and/or distribute this software":                                            "ISC",
		"This is free and unencumbered software released into the public domain":                                      "Unlicense",
		"CREATIVE COMMONS ZERO / CC0 1.0 UNIVERSAL":                                                                   "CC0-1.0",
		"SPDX-License-Identifier: Apache-2.0":                                                                         "Apache-2.0",
		"totally novel license text nobody recognizes":                                                                "",
	}
	for text, want := range cases {
		if got := classifyText(text); got != want {
			t.Errorf("classifyText(%q) = %q, want %q", text[:min(len(text), 40)], got, want)
		}
	}
}

func TestNormalizeSPDX(t *testing.T) {
	cases := map[string]string{
		"MIT License":               "MIT",
		"Apache 2.0":                "Apache-2.0",
		"GPLv3":                     "GPL-3.0",
		"AGPL-3.0-or-later":         "AGPL-3.0",
		"UNLICENSED":                "UNLICENSED",
		"SEE LICENSE IN license.md": "SEE-LICENSE-IN",
		"Proprietary-Internal":      "Proprietary",
	}
	for in, want := range cases {
		if got := normalizeSPDX(in); got != want {
			t.Errorf("normalizeSPDX(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPlanRequiresRootOrManifests(t *testing.T) {
	a := New()
	if _, err := a.Plan(context.Background(), analyzers.ScanRequest{}); err == nil {
		t.Fatal("expected plan to refuse a request with no root and no manifests")
	}
	plan, err := a.Plan(context.Background(), analyzers.ScanRequest{WorkspaceRoot: t.TempDir()})
	if err != nil {
		t.Fatalf("plan with root: %v", err)
	}
	if plan.AnalyzerID != ID {
		t.Fatalf("plan analyzer %q", plan.AnalyzerID)
	}
}

// runScan drives Plan -> Run -> Normalize over a real temp workspace.
func runScan(t *testing.T, files map[string]string) ([]analyzers.Finding, []string) {
	t.Helper()
	root := t.TempDir()
	for name, content := range files {
		full := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	req := analyzers.ScanRequest{WorkspaceRoot: root}
	for name := range files {
		// Discovery supplies absolute paths; the adapter reads manifests as given.
		req.Files = append(req.Files, filepath.Join(root, name))
	}
	a := New()
	plan, err := a.Plan(context.Background(), req)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	result, err := a.Run(context.Background(), plan, nil)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	findings, _, err := a.Normalize(context.Background(), result)
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	rules := make([]string, 0, len(findings))
	for _, f := range findings {
		rules = append(rules, f.RuleID)
	}
	return findings, rules
}

func hasRule(rules []string, rule string) bool {
	for _, r := range rules {
		if r == rule {
			return true
		}
	}
	return false
}

func TestCopyleftFileAndPermissiveManifestConflict(t *testing.T) {
	findings, rules := runScan(t, map[string]string{
		"LICENSE":      "GNU GENERAL PUBLIC LICENSE\nVersion 3, 29 June 2007",
		"package.json": `{"name": "x", "version": "1.0.0", "license": "MIT"}`,
	})
	if !hasRule(rules, "license-copyleft-gpl") {
		t.Fatalf("expected license-copyleft-gpl in %v", rules)
	}
	if !hasRule(rules, "license-conflict") {
		t.Fatalf("expected license-conflict in %v", rules)
	}
	if !hasRule(rules, "license-permissive") {
		t.Fatalf("expected manifest permissive finding in %v", rules)
	}
	for _, f := range findings {
		if f.RuleID == "license-conflict" && f.Severity != analyzers.SeverityLow {
			t.Fatalf("conflict severity = %v", f.Severity)
		}
		if f.RuleID == "license-copyleft-gpl" && f.Severity != analyzers.SeverityMedium {
			t.Fatalf("gpl severity = %v", f.Severity)
		}
	}
}

func TestAGPLFileIsHigh(t *testing.T) {
	_, rules := runScan(t, map[string]string{
		"LICENSE": "GNU AFFERO GENERAL PUBLIC LICENSE\nVersion 3, 19 November 2007",
	})
	if !hasRule(rules, "license-copyleft-agpl") {
		t.Fatalf("expected license-copyleft-agpl in %v", rules)
	}
	findings, _ := runScan(t, map[string]string{
		"LICENSE": "GNU AFFERO GENERAL PUBLIC LICENSE\nVersion 3, 19 November 2007",
	})
	for _, f := range findings {
		if f.RuleID == "license-copyleft-agpl" && f.Severity != analyzers.SeverityHigh {
			t.Fatalf("agpl severity = %v", f.Severity)
		}
	}
}

func TestUndeclaredWorkspace(t *testing.T) {
	_, rules := runScan(t, map[string]string{
		"src/main.go": "package main",
	})
	if !hasRule(rules, "license-undeclared") {
		t.Fatalf("expected license-undeclared in %v", rules)
	}
}

func TestUnlicensedManifestMarkerWithoutFile(t *testing.T) {
	_, rules := runScan(t, map[string]string{
		"package.json": `{"name": "x", "private": true, "license": "UNLICENSED"}`,
	})
	if !hasRule(rules, "license-undeclared") {
		t.Fatalf("expected license-undeclared for UNLICENSED marker in %v", rules)
	}
	if hasRule(rules, "license-permissive") {
		t.Fatalf("UNLICENSED must not surface as permissive: %v", rules)
	}
}

func TestConsistentPermissiveNoConflict(t *testing.T) {
	_, rules := runScan(t, map[string]string{
		"LICENSE":      "Permission is hereby granted, free of charge, to any person obtaining a copy",
		"package.json": `{"name": "x", "license": "MIT"}`,
	})
	if hasRule(rules, "license-conflict") {
		t.Fatalf("MIT file + MIT manifest must not conflict: %v", rules)
	}
	if hasRule(rules, "license-undeclared") {
		t.Fatalf("declared workspace must not be undeclared: %v", rules)
	}
}

func TestUnrecognizedLicenseFileIsLowSignal(t *testing.T) {
	findings, rules := runScan(t, map[string]string{
		"LICENSE": "some bespoke proprietary agreement",
	})
	if !hasRule(rules, "license-unrecognized") {
		t.Fatalf("expected license-unrecognized in %v", rules)
	}
	for _, f := range findings {
		if f.RuleID == "license-unrecognized" && f.Severity != analyzers.SeverityInfo {
			t.Fatalf("unrecognized severity = %v", f.Severity)
		}
	}
}

func TestPyprojectAndCargoDeclarations(t *testing.T) {
	_, rules := runScan(t, map[string]string{
		"pyproject.toml": "[project]\nname = 'x'\nlicense = { text = 'Apache-2.0' }\n",
		"Cargo.toml":     "[package]\nname = \"x\"\nlicense = \"MIT\"\n",
	})
	if !hasRule(rules, "license-permissive") {
		t.Fatalf("expected manifest permissive findings in %v", rules)
	}
	if hasRule(rules, "license-conflict") {
		t.Fatalf("Apache file-less + MIT manifest are both permissive; conflict should not fire: %v", rules)
	}
}

func TestNormalizeRejectsNonzeroExit(t *testing.T) {
	a := New()
	_, _, err := a.Normalize(context.Background(), analyzers.AnalyzerResult{ExitCode: 2, Stderr: []byte("boom")})
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("expected exit-code error with stderr, got %v", err)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
