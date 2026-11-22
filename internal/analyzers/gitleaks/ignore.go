package gitleaks

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"bluntcode/internal/analyzers"
)

// gitleaks unconditionally reads <source>/.gitleaksignore (verified against
// v8.30.1: --gitleaks-ignore-path adds a second file, it never replaces the
// source one), and every fingerprint in it suppresses a finding. When the
// workspace does not trust its in-repo analyzer config, the files those
// fingerprints name are copied into a Blunt Code-owned staging directory that
// has no .gitleaksignore and re-scanned there with the same pinned config.
// The staged results replace the first pass's results for those files, so a
// fingerprint in the scanned repository can no longer hide a secret.

const (
	maxIgnoreEntries    = 10000
	maxStagedFileBytes  = 64 << 20
	ignoreFileName      = ".gitleaksignore"
	stageDirPrefix      = "gitleaks-stage"
	stagedReportPostfix = "-staged.json"
)

// ignoreTargets returns the workspace-relative paths (slash form, sorted by
// first appearance) of the regular files named by the workspace's root
// .gitleaksignore. Fingerprints are "file:rule:line" or
// "commit:file:rule:line"; the file part may itself contain colons (a Windows
// drive letter), so it is everything before the last two fields, with an
// optional leading commit field also tried.
func ignoreTargets(root string) []string {
	f, err := os.Open(filepath.Join(root, ignoreFileName))
	if err != nil {
		return nil
	}
	defer f.Close()
	realRoot, _ := filepath.EvalSymlinks(root)
	seen := make(map[string]bool)
	var out []string
	scanner := bufio.NewScanner(io.LimitReader(f, 16<<20))
	scanner.Buffer(make([]byte, 0, 64<<10), 1<<20)
	for lines := 0; scanner.Scan() && lines < maxIgnoreEntries; lines++ {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, ":")
		if len(parts) < 3 {
			continue
		}
		candidates := []string{strings.Join(parts[:len(parts)-2], ":")}
		if len(parts) >= 4 {
			candidates = append(candidates, strings.Join(parts[1:len(parts)-2], ":"))
		}
		for _, c := range candidates {
			rel, ok := insideRoot(root, c)
			if !ok || seen[rel] || rel == ignoreFileName {
				continue
			}
			full := filepath.Join(root, filepath.FromSlash(rel))
			info, err := os.Lstat(full)
			if err != nil || !info.Mode().IsRegular() || info.Size() > maxStagedFileBytes {
				continue
			}
			// Lstat only checks the last element; a directory link earlier in
			// the path could still lead outside the workspace.
			if real, err := filepath.EvalSymlinks(full); err != nil || realRoot == "" || real != filepath.Join(realRoot, filepath.FromSlash(rel)) {
				continue
			}
			seen[rel] = true
			out = append(out, rel)
		}
	}
	return out
}

// insideRoot resolves an ignore-file path (absolute, or relative to the
// workspace root) to a clean workspace-relative slash path, rejecting
// anything that escapes the root.
func insideRoot(root, path string) (string, bool) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", false
	}
	native := filepath.FromSlash(path)
	if !filepath.IsAbs(native) {
		native = filepath.Join(root, native)
	}
	rel, err := filepath.Rel(root, filepath.Clean(native))
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", false
	}
	return filepath.ToSlash(rel), true
}

// rescanIgnoredFiles runs the staged second pass described above and merges
// its report into result.Stdout. It is a no-op when the workspace has no
// root .gitleaksignore naming files that exist.
func (a *Adapter) rescanIgnoredFiles(ctx context.Context, plan analyzers.AnalyzerPlan, emit analyzers.EventEmitter, result analyzers.AnalyzerResult) (analyzers.AnalyzerResult, error) {
	root, _ := plan.Metadata[planKeySource].(string)
	if root == "" || len(plan.Commands) != 1 {
		return result, nil
	}
	targets := ignoreTargets(root)
	if len(targets) == 0 {
		return result, nil
	}
	stage, err := os.MkdirTemp(os.TempDir(), "bluntcode-"+stageDirPrefix+"-*")
	if err != nil {
		return result, fmt.Errorf("create gitleaks staging directory: %w", err)
	}
	defer os.RemoveAll(stage)
	for _, rel := range targets {
		if err := copyFile(filepath.Join(root, filepath.FromSlash(rel)), filepath.Join(stage, filepath.FromSlash(rel))); err != nil {
			return result, fmt.Errorf("stage %s for gitleaks: %w", rel, err)
		}
	}
	mainReport, _ := plan.Metadata[planKeyReportPath].(string)
	stagedReport := strings.TrimSuffix(mainReport, ".json") + stagedReportPostfix
	defer os.Remove(stagedReport)
	staged := plan
	command := plan.Commands[0]
	command.Args = replaceFlagValue(replaceFlagValue(command.Args, "--source", stage), "--report-path", stagedReport)
	staged.Commands = []analyzers.ProcessSpec{command}
	second, err := analyzers.RunDirect(ctx, staged, emit)
	if err != nil {
		return result, err
	}
	if second.ExitCode != 0 {
		second.Plan = plan
		return second, nil
	}
	stagedBytes, err := os.ReadFile(stagedReport)
	if err != nil && !os.IsNotExist(err) {
		return result, fmt.Errorf("read staged gitleaks report: %w", err)
	}
	merged, err := mergeReports(result.Stdout, stagedBytes, root, stage, targets)
	if err != nil {
		return result, err
	}
	result.Stdout = merged
	result.FinishedAt = second.FinishedAt
	return result, nil
}

func replaceFlagValue(args []string, flag, value string) []string {
	out := append([]string(nil), args...)
	for i := 0; i+1 < len(out); i++ {
		if out[i] == flag {
			out[i+1] = value
		}
	}
	return out
}

// mergeReports drops the first pass's findings for staged files and appends
// the staged pass's findings with their paths mapped back under root. Records
// stay generic maps so every gitleaks field survives untouched.
func mergeReports(mainReport, stagedReport []byte, root, stage string, targets []string) ([]byte, error) {
	staged := make(map[string]bool, len(targets))
	for _, rel := range targets {
		staged[rel] = true
	}
	var mainFindings, stagedFindings []map[string]any
	if len(strings.TrimSpace(string(mainReport))) > 0 {
		if err := json.Unmarshal(mainReport, &mainFindings); err != nil {
			return nil, fmt.Errorf("parse gitleaks report: %w", err)
		}
	}
	if len(strings.TrimSpace(string(stagedReport))) > 0 {
		if err := json.Unmarshal(stagedReport, &stagedFindings); err != nil {
			return nil, fmt.Errorf("parse staged gitleaks report: %w", err)
		}
	}
	out := make([]map[string]any, 0, len(mainFindings)+len(stagedFindings))
	for _, f := range mainFindings {
		file, _ := f["File"].(string)
		if rel, ok := insideRoot(root, file); ok && staged[rel] {
			continue
		}
		out = append(out, f)
	}
	for _, f := range stagedFindings {
		file, _ := f["File"].(string)
		rel, ok := insideRoot(stage, file)
		if !ok || !staged[rel] {
			continue
		}
		mapped := filepath.Join(root, filepath.FromSlash(rel))
		f["File"] = mapped
		if fp, _ := f["Fingerprint"].(string); strings.HasPrefix(fp, file) {
			f["Fingerprint"] = mapped + strings.TrimPrefix(fp, file)
		}
		out = append(out, f)
	}
	return json.Marshal(out)
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, io.LimitReader(in, maxStagedFileBytes)); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
