// Package license implements Blunt Code's built-in license detector. Like the
// secrets and TODO trackers it needs no managed executable — the scan runs
// in-process — while keeping the adapter contract intact: Run serializes its
// diagnostics into AnalyzerResult.Stdout as JSON and Normalize parses that
// JSON into findings, so the pipeline treats it like any other analyzer.
//
// Two sources are reconciled: the workspace's license file (LICENSE,
// COPYING, and common variants, classified by content heuristics) and the
// license declared in dependency manifests (package.json, pyproject.toml,
// Cargo.toml, composer.json). Findings flag strong copyleft (AGPL/SSPL,
// GPL), weak copyleft (LGPL/MPL), conflicts between the two sources, and
// workspaces that declare no license at all.
package license

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"bluntcode/internal/analyzers"
)

const (
	ID      = "license-scan"
	version = "1.0.0"
)

// remediation strings per rule; every finding carries one so the next step is
// always written down next to the signal.
const (
	remediationCopyleft = "Review the copyleft obligations (source disclosure, derivative licensing) for this workspace's distribution model, and confirm the choice is intentional with whoever owns the legal call."
	remediationConflict = "Pick one license and make the file and the manifest agree; downstream consumers read the manifest first and will assume it is authoritative."
	remediationMissing  = "Add a LICENSE file and a manifest license field; without one, default copyright law forbids anyone (including your own future team) from reusing the code."
	remediationUnknown  = "Name the license explicitly (SPDX identifier in the manifest, or the canonical license text as the LICENSE file) so tooling and humans can rely on it."
)

type Adapter struct{}

func New() *Adapter { return &Adapter{} }

func (a *Adapter) ID() string          { return ID }
func (a *Adapter) DisplayName() string { return "License Scanner" }

// SupportedLanguages routes on the formats that carry licensing signal:
// json and toml for dependency manifests, markdown and text for license
// files themselves (a bare LICENSE has no extension and classifies as text).
// A workspace with none of these has no license signal to read.
func (a *Adapter) SupportedLanguages() []analyzers.Language {
	return []analyzers.Language{analyzers.LanguageJSON, analyzers.LanguageTOML, analyzers.LanguageMarkdown, analyzers.LanguageText}
}

func (a *Adapter) Check(context.Context, analyzers.ToolEnvironment) analyzers.ToolStatus {
	return analyzers.ToolStatus{Ready: true, Version: version, Detail: "built-in detector; no external tool required"}
}

func (a *Adapter) EnsureInstalled(context.Context, analyzers.ToolEnvironment) error { return nil }

const (
	planKeyManifests = "manifests"
	planKeyRoot      = "workspace_root"
)

// manifestBasenames are the dependency manifests whose license field is read.
var manifestBasenames = map[string]bool{
	"package.json":   true,
	"pyproject.toml": true,
	"Cargo.toml":     true,
	"composer.json":  true,
}

// licenseFileNames are probed at the workspace root, in order; every file
// that exists is classified (some projects carry LICENSE + LICENSE.APACHE).
var licenseFileNames = []string{
	"LICENSE", "LICENSE.md", "LICENSE.txt", "LICENSE.rst",
	"LICENCE", "LICENCE.md", "LICENCE.txt",
	"COPYING", "COPYING.md", "COPYING.txt", "COPYING.LESSER",
	"LICENSE-MIT", "LICENSE.APACHE-2.0", "LICENSE.GPL-2.0", "LICENSE.GPL-3.0",
}

func (a *Adapter) Plan(_ context.Context, req analyzers.ScanRequest) (analyzers.AnalyzerPlan, error) {
	// The orchestrator already routes by language, but the adapter filters
	// again (the convention ruff set) so no caller can smuggle unrelated
	// files into the scan.
	routed := analyzers.FilesForLanguages(req.Files, a.SupportedLanguages()...)
	manifests := make([]string, 0, len(routed))
	for _, file := range routed {
		if manifestBasenames[filepath.Base(filepath.ToSlash(file))] {
			manifests = append(manifests, file)
		}
	}
	// A license file at the root is read from disk directly (discovery may
	// not classify a bare LICENSE into the selection), so the plan applies
	// whenever there is a workspace root even without routed manifests.
	if len(manifests) == 0 && req.WorkspaceRoot == "" {
		return analyzers.AnalyzerPlan{}, fmt.Errorf("license scanner does not apply")
	}
	return analyzers.AnalyzerPlan{
		AnalyzerID: ID,
		Version:    version,
		Metadata:   map[string]any{planKeyManifests: manifests, planKeyRoot: req.WorkspaceRoot},
	}, nil
}

type rawDiagnostic struct {
	Rule    string `json:"rule"`
	SPDX    string `json:"spdx"`
	Source  string `json:"source"` // file | manifest
	Path    string `json:"path"`
	Message string `json:"message"`
}

type rawEnvelope struct {
	AnalyzerID  string          `json:"analyzer"`
	Version     string          `json:"version"`
	Notes       []string        `json:"notes,omitempty"`
	Diagnostics []rawDiagnostic `json:"diagnostics"`
}

// Run reads the manifests and root license files in-process and serializes
// the diagnostics as JSON into Stdout, preserving the Run -> Normalize
// contract.
func (a *Adapter) Run(ctx context.Context, plan analyzers.AnalyzerPlan, _ analyzers.EventEmitter) (analyzers.AnalyzerResult, error) {
	started := time.Now()
	manifests, root, err := planSelection(plan)
	if err != nil {
		return analyzers.AnalyzerResult{Plan: plan}, err
	}
	env := rawEnvelope{AnalyzerID: ID, Version: version, Diagnostics: []rawDiagnostic{}}

	fileSPDX := ""
	filePath := ""
	for _, name := range licenseFileNames {
		if err := ctx.Err(); err != nil {
			return analyzers.AnalyzerResult{Plan: plan}, err
		}
		full := filepath.Join(root, name)
		data, readErr := os.ReadFile(full)
		if readErr != nil {
			continue // not every variant exists; probing is the point
		}
		spdx := classifyText(string(data))
		env.Diagnostics = append(env.Diagnostics, rawDiagnostic{
			Rule:    ruleForSPDX(spdx),
			SPDX:    spdx,
			Source:  "file",
			Path:    name,
			Message: messageForFile(name, spdx),
		})
		// The first recognized file is the workspace's primary license for
		// conflict detection; later files still report their own findings.
		if fileSPDX == "" {
			fileSPDX, filePath = spdx, name
		}
	}

	manifestSPDX := ""
	manifestPath := ""
	for _, manifest := range manifests {
		if err := ctx.Err(); err != nil {
			return analyzers.AnalyzerResult{Plan: plan}, err
		}
		data, readErr := os.ReadFile(manifest)
		if readErr != nil {
			env.Notes = append(env.Notes, fmt.Sprintf("%s unreadable: %v", relative(root, manifest), readErr))
			continue
		}
		declared := declaredLicense(filepath.Base(filepath.ToSlash(manifest)), data)
		if declared == "" {
			continue
		}
		spdx := normalizeSPDX(declared)
		if manifestSPDX == "" {
			manifestSPDX, manifestPath = spdx, relative(root, manifest)
		}
		// An explicit "no license" marker (UNLICENSED, Proprietary, SEE
		// LICENSE IN) only matters when no readable license file backs it.
		if isUnlicensedMarker(spdx) {
			if fileSPDX == "" {
				env.Diagnostics = append(env.Diagnostics, rawDiagnostic{
					Rule:    "license-undeclared",
					SPDX:    spdx,
					Source:  "manifest",
					Path:    manifestPath,
					Message: fmt.Sprintf("%s declares no reusable license (%s)", manifestPath, spdx),
				})
			}
			continue
		}
		env.Diagnostics = append(env.Diagnostics, rawDiagnostic{
			Rule:    ruleForSPDX(spdx),
			SPDX:    spdx,
			Source:  "manifest",
			Path:    manifestPath,
			Message: fmt.Sprintf("%s declares %s", manifestPath, spdx),
		})
	}

	// Conflict: both sources carry a real license and they disagree. The
	// dangerous direction is a permissive manifest riding on a copyleft file
	// (users believe MIT while the code is GPL); both directions mislead.
	if fileSPDX != "" && manifestSPDX != "" && !isUnlicensedMarker(fileSPDX) && !isUnlicensedMarker(manifestSPDX) && fileSPDX != manifestSPDX {
		env.Diagnostics = append(env.Diagnostics, rawDiagnostic{
			Rule:    "license-conflict",
			SPDX:    fileSPDX + " vs " + manifestSPDX,
			Source:  "file",
			Path:    filePath,
			Message: fmt.Sprintf("license file %s says %s but %s declares %s", filePath, fileSPDX, manifestPath, manifestSPDX),
		})
	}

	// Nothing anywhere: an unlicensed workspace is worth one quiet finding.
	if fileSPDX == "" && manifestSPDX == "" {
		env.Diagnostics = append(env.Diagnostics, rawDiagnostic{
			Rule:    "license-undeclared",
			SPDX:    "",
			Source:  "file",
			Path:    ".",
			Message: "no license file or manifest license declaration found in this workspace",
		})
	}

	out, err := json.Marshal(env)
	if err != nil {
		return analyzers.AnalyzerResult{Plan: plan}, err
	}
	return analyzers.AnalyzerResult{Plan: plan, StartedAt: started, FinishedAt: time.Now(), ExitCode: 0, Stdout: out}, nil
}

// Normalize converts the JSON envelope into findings. License findings are
// file-level: no line or column information exists for a license declaration.
func (a *Adapter) Normalize(_ context.Context, r analyzers.AnalyzerResult) ([]analyzers.Finding, []analyzers.Metric, error) {
	if r.ExitCode != 0 {
		return nil, nil, fmt.Errorf("license scanner exited %d: %s", r.ExitCode, strings.TrimSpace(string(r.Stderr)))
	}
	if len(r.Stdout) == 0 {
		return nil, nil, nil
	}
	var env rawEnvelope
	if err := json.Unmarshal(r.Stdout, &env); err != nil {
		return nil, nil, fmt.Errorf("parse license scanner JSON: %w", err)
	}
	out := make([]analyzers.Finding, 0, len(env.Diagnostics))
	for _, d := range env.Diagnostics {
		f := analyzers.Finding{
			AnalyzerID:   ID,
			RuleID:       d.Rule,
			Severity:     severityForRule(d.Rule),
			Category:     analyzers.CategoryOther,
			Title:        d.Rule,
			Message:      d.Message,
			RelativePath: d.Path,
			Remediation:  remediationForRule(d.Rule),
			Metadata:     map[string]any{"source": d.Source},
		}
		if d.SPDX != "" {
			f.Metadata["spdx"] = d.SPDX
		}
		f.SetFingerprint()
		out = append(out, f)
	}
	return out, nil, nil
}

// severityForRule keeps the alarm level proportional: strong copyleft
// reshapes what a company may ship, weak copyleft is a conditional duty,
// conflicts and unrecognized text are hygiene, and a missing license is
// informational.
func severityForRule(rule string) analyzers.Severity {
	switch rule {
	case "license-copyleft-agpl":
		return analyzers.SeverityHigh
	case "license-copyleft-gpl":
		return analyzers.SeverityMedium
	case "license-weak-copyleft", "license-conflict":
		return analyzers.SeverityLow
	default:
		return analyzers.SeverityInfo
	}
}

func ruleForSPDX(spdx string) string {
	switch {
	case spdx == "":
		return "license-unrecognized"
	case strings.HasPrefix(spdx, "AGPL") || strings.HasPrefix(spdx, "SSPL"):
		return "license-copyleft-agpl"
	case strings.HasPrefix(spdx, "GPL"):
		return "license-copyleft-gpl"
	case strings.HasPrefix(spdx, "LGPL") || spdx == "MPL-2.0":
		return "license-weak-copyleft"
	default:
		return "license-permissive"
	}
}

func remediationForRule(rule string) string {
	switch rule {
	case "license-copyleft-agpl", "license-copyleft-gpl", "license-weak-copyleft":
		return remediationCopyleft
	case "license-conflict":
		return remediationConflict
	case "license-undeclared":
		return remediationMissing
	default:
		return remediationUnknown
	}
}

func messageForFile(name, spdx string) string {
	if spdx == "" {
		return fmt.Sprintf("license file %s exists but its text is not recognized", name)
	}
	return fmt.Sprintf("license file %s is %s", name, spdx)
}

func isUnlicensedMarker(spdx string) bool {
	switch spdx {
	case "UNLICENSED", "Proprietary", "SEE-LICENSE-IN", "UNKNOWN":
		return true
	}
	return false
}

func planSelection(plan analyzers.AnalyzerPlan) ([]string, string, error) {
	manifests, _ := plan.Metadata[planKeyManifests].([]string)
	if manifests == nil {
		list, ok := plan.Metadata[planKeyManifests].([]any)
		if !ok {
			return nil, "", fmt.Errorf("license plan is missing its manifest list")
		}
		for _, v := range list {
			s, ok := v.(string)
			if !ok {
				return nil, "", fmt.Errorf("license plan manifest entry is not a string")
			}
			manifests = append(manifests, s)
		}
	}
	root, _ := plan.Metadata[planKeyRoot].(string)
	if root == "" {
		return nil, "", fmt.Errorf("license plan is missing its workspace root")
	}
	return manifests, root, nil
}

func relative(root, path string) string {
	if root != "" {
		if rel, err := filepath.Rel(root, path); err == nil {
			return filepath.ToSlash(rel)
		}
	}
	return filepath.ToSlash(path)
}

// declaredLicense extracts the license declaration from one manifest type.
// json manifests are parsed for real; toml is scanned line-wise because a
// full toml parser is not otherwise needed in this codebase.
func declaredLicense(basename string, data []byte) string {
	switch basename {
	case "package.json", "composer.json":
		var doc map[string]any
		if err := json.Unmarshal(data, &doc); err != nil {
			return ""
		}
		return jsonLicenseField(doc)
	case "pyproject.toml", "Cargo.toml":
		return tomlLicenseLine(data)
	}
	return ""
}

// jsonLicenseField reads package.json's license (string or {type}) and
// composer.json's license (string or array; the first entry wins).
func jsonLicenseField(doc map[string]any) string {
	if v, ok := doc["license"]; ok {
		switch t := v.(type) {
		case string:
			return t
		case map[string]any:
			if s, ok := t["type"].(string); ok {
				return s
			}
		case []any:
			if len(t) > 0 {
				if s, ok := t[0].(string); ok {
					return s
				}
			}
		}
	}
	return ""
}

// tomlLicenseLine finds `license = "..."` (and the pyproject table form
// license = { text = "..." }) anywhere in the file; the project table is the
// only conventional carrier, so a naive line scan cannot be spoofed into
// missing a real declaration by ordering.
func tomlLicenseLine(data []byte) string {
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "license") {
			continue
		}
		rest := strings.TrimSpace(strings.TrimPrefix(trimmed, "license"))
		if !strings.HasPrefix(rest, "=") {
			continue
		}
		value := strings.TrimSpace(strings.TrimPrefix(rest, "="))
		if s, ok := unquote(value); ok {
			return s
		}
		// license = { text = "MIT" } style
		if strings.HasPrefix(value, "{") {
			for _, part := range strings.Split(value, ",") {
				kv := strings.SplitN(part, ":", 2)
				if len(kv) != 2 {
					kv = strings.SplitN(part, "=", 2)
				}
				if len(kv) == 2 && strings.TrimSpace(kv[0]) == "text" {
					if s, ok := unquote(strings.TrimSuffix(strings.TrimSpace(kv[1]), "}")); ok {
						return s
					}
				}
			}
		}
	}
	return ""
}

func unquote(v string) (string, bool) {
	v = strings.TrimSpace(v)
	if len(v) >= 2 && (v[0] == '"' || v[0] == '\'') && v[len(v)-1] == v[0] {
		return v[1 : len(v)-1], true
	}
	return "", false
}

// maxLicenseFileRead caps how much of a license file is classified; the
// canonical texts all fit comfortably, and a giant "LICENSE" that is really
// a data dump needs no more than its head to classify anyway.
const maxLicenseFileRead = 64 * 1024

// classifyText maps license file content onto an SPDX id. The probes are
// distinctive phrases from each canonical text, ordered so the most specific
// (and most consequential) licenses win ties.
func classifyText(content string) string {
	head := content
	if len(head) > maxLicenseFileRead {
		head = head[:maxLicenseFileRead]
	}
	upper := strings.ToUpper(head)
	switch {
	case strings.Contains(upper, "GNU AFFERO GENERAL PUBLIC LICENSE"):
		if strings.Contains(upper, "VERSION 3") {
			return "AGPL-3.0"
		}
		return "AGPL"
	case strings.Contains(upper, "SERVER SIDE PUBLIC LICENSE"):
		return "SSPL-1.0"
	case strings.Contains(upper, "MOZILLA PUBLIC LICENSE"):
		if strings.Contains(upper, "VERSION 2.0") {
			return "MPL-2.0"
		}
		return "MPL"
	case strings.Contains(upper, "LESSER GENERAL PUBLIC LICENSE"):
		if strings.Contains(upper, "VERSION 3") {
			return "LGPL-3.0"
		}
		if strings.Contains(upper, "VERSION 2.1") {
			return "LGPL-2.1"
		}
		return "LGPL"
	case strings.Contains(upper, "GNU GENERAL PUBLIC LICENSE"):
		if strings.Contains(upper, "VERSION 3") {
			return "GPL-3.0"
		}
		if strings.Contains(upper, "VERSION 2") {
			return "GPL-2.0"
		}
		return "GPL"
	case strings.Contains(upper, "APACHE LICENSE"):
		if strings.Contains(upper, "VERSION 2.0") || strings.Contains(upper, "VERSION 2, JANUARY 2004") {
			return "Apache-2.0"
		}
		return "Apache"
	case strings.Contains(upper, "PERMISSION IS HEREBY GRANTED, FREE OF CHARGE"), strings.Contains(upper, "MIT LICENSE"):
		return "MIT"
	case strings.Contains(upper, "REDISTRIBUTION AND USE IN SOURCE AND BINARY FORMS"):
		if strings.Contains(upper, "MAY NOT BE USED TO ENDORSE OR PROMOTE") {
			return "BSD-3-Clause"
		}
		return "BSD-2-Clause"
	case strings.Contains(upper, "PERMISSION TO USE, COPY, MODIFY, AND/OR DISTRIBUTE THIS SOFTWARE"), strings.Contains(upper, "ISC LICENSE"):
		return "ISC"
	case strings.Contains(upper, "FREE AND UNENCUMBERED SOFTWARE RELEASED INTO THE PUBLIC DOMAIN"), strings.Contains(upper, "THE UNLICENSE"):
		return "Unlicense"
	case strings.Contains(upper, "CREATIVE COMMONS ZERO"), strings.Contains(upper, "CC0 1.0 UNIVERSAL"):
		return "CC0-1.0"
	case strings.Contains(upper, "ZLIB LICENSE"), strings.Contains(upper, "SOFTWARE IS PROVIDED 'AS-IS'"):
		return "Zlib"
	case strings.Contains(head, "SPDX-License-Identifier:"):
		field := head[strings.Index(head, "SPDX-License-Identifier:")+len("SPDX-License-Identifier:"):]
		fields := strings.Fields(field)
		if len(fields) > 0 {
			return normalizeSPDX(fields[0])
		}
		return ""
	}
	return ""
}

// normalizeSPDX maps common declared forms onto canonical SPDX ids so a
// manifest's "MIT License" and the file's "MIT" compare equal. Unknown
// values pass through unchanged; conflict detection then flags them.
func normalizeSPDX(declared string) string {
	d := strings.TrimSpace(declared)
	d = strings.TrimPrefix(strings.TrimPrefix(d, "("), "\"")
	d = strings.TrimSuffix(strings.TrimSuffix(d, ")"), "\"")
	switch strings.ToLower(d) {
	case "mit", "mit license", "x11":
		return "MIT"
	case "apache-2.0", "apache 2.0", "apache2", "apache license 2.0", "asl 2.0":
		return "Apache-2.0"
	case "gpl-3.0", "gpl-3.0-only", "gpl-3.0-or-later", "gplv3", "gnu gpl v3", "gpl-3", "gnu general public license v3.0":
		return "GPL-3.0"
	case "gpl-2.0", "gpl-2.0-only", "gpl-2.0-or-later", "gplv2", "gnu gpl v2", "gpl-2", "gnu general public license v2.0":
		return "GPL-2.0"
	case "agpl-3.0", "agpl-3.0-only", "agpl-3.0-or-later", "agplv3", "gnu agpl v3":
		return "AGPL-3.0"
	case "lgpl-3.0", "lgpl-3.0-only", "lgpl-3.0-or-later", "lgplv3", "gnu lgpl v3":
		return "LGPL-3.0"
	case "lgpl-2.1", "lgpl-2.1-only", "lgplv2.1", "gnu lgpl v2.1":
		return "LGPL-2.1"
	case "mpl-2.0", "mpl 2.0", "mozilla public license 2.0":
		return "MPL-2.0"
	case "bsd-2-clause", "bsd-2", "simplified bsd":
		return "BSD-2-Clause"
	case "bsd-3-clause", "bsd-3", "new bsd", "modified bsd":
		return "BSD-3-Clause"
	case "isc":
		return "ISC"
	case "unlicense", "the unlicense":
		return "Unlicense"
	case "cc0-1.0", "cc0 1.0", "cc0":
		return "CC0-1.0"
	case "zlib":
		return "Zlib"
	case "unlicensed", "all-rights-reserved", "none", "no-license":
		return "UNLICENSED"
	case "proprietary", "commercial", "proprietary-", "see license in license", "see-license-in":
		return "Proprietary"
	case "":
		return ""
	default:
		if strings.HasPrefix(strings.ToLower(d), "see license in") {
			return "SEE-LICENSE-IN"
		}
		if strings.HasPrefix(strings.ToLower(d), "proprietary") {
			return "Proprietary"
		}
		return d
	}
}
