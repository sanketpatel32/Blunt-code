package tools

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"bluntcode/internal/build"
	"bluntcode/internal/process"
)

// buildVersion reports the Blunt Code build that performed an installation;
// recorded in each artifact's install metadata.
func buildVersion() string { return build.Version }

type Manager struct {
	Root       string
	Manifest   Manifest
	Client     *http.Client
	RunCommand func(context.Context, string, []string, string, []string) error
	// SmokeRunner executes one post-activation probe (args included) and
	// returns its combined output. Package-level injection point so tests can
	// simulate failing or lying binaries without compiling real ones; nil uses
	// the real child process.
	SmokeRunner func(context.Context, string, []string) (string, error)
}

func (m Manager) client() *http.Client {
	if m.Client != nil {
		return m.Client
	}
	// Managed server and JDK archives are large; keep a bounded timeout while
	// allowing slower reliable connections to complete checksum verification.
	return &http.Client{Timeout: 15 * time.Minute}
}
func (m Manager) Executable(a Artifact) string {
	return filepath.Join(m.Root, a.ToolID, a.Version, a.Executable)
}
func (m Manager) IsReady(a Artifact) bool {
	p := m.Executable(a)
	_, err := os.Stat(p)
	return err == nil
}

// smokeProbe runs the artifact's post-activation smoke test: the installed
// executable must exit 0 (and, when Expect is set, print the pinned version).
// A failing probe means the activated bytes are not trustworthy even though
// their digest matched — the caller rolls the previous version back.
func (m Manager) smokeProbe(ctx context.Context, executable string, s *SmokeTest) error {
	if s == nil || len(s.Args) == 0 {
		return nil
	}
	runner := m.SmokeRunner
	if runner == nil {
		runner = realSmokeRunner
	}
	output, err := runner(ctx, executable, s.Args)
	if err != nil {
		return fmt.Errorf("smoke test failed: %w", err)
	}
	if s.Expect != "" && !strings.Contains(strings.ToLower(output), strings.ToLower(s.Expect)) {
		return fmt.Errorf("smoke test output did not mention %q: %s", s.Expect, strings.TrimSpace(output))
	}
	return nil
}

// realSmokeRunner runs the probe as a child process with its own deadline and
// a capped output buffer: a probe that hangs or floods must not wedge the
// installer.
func realSmokeRunner(ctx context.Context, executable string, args []string) (string, error) {
	probeCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(probeCtx, executable, args...)
	cmd.WaitDelay = 5 * time.Second
	output := process.NewCappedBuffer(1 << 20)
	cmd.Stdout = output
	cmd.Stderr = output
	if err := cmd.Start(); err != nil {
		return "", err
	}
	if err := cmd.Wait(); err != nil {
		return string(output.Bytes()), err
	}
	return string(output.Bytes()), nil
}

// Download installs a single verified artifact atomically. Archive extraction
// is intentionally outside this primitive; callers must use an archive reader
// that rejects absolute and traversal entries before invoking final rename.
func (m Manager) Download(ctx context.Context, a Artifact) (string, error) {
	if err := a.Validate(); err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Join(m.Root, ".downloads"), 0o700); err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.SourceURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := m.client().Do(req)
	if err != nil {
		return "", fmt.Errorf("download %s: %w", a.ToolID, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download %s: unexpected status %s", a.ToolID, resp.Status)
	}
	tmp, err := os.CreateTemp(filepath.Join(m.Root, ".downloads"), a.ToolID+"-*")
	if err != nil {
		return "", err
	}
	tmpName := tmp.Name()
	if _, err := io.Copy(tmp, io.LimitReader(resp.Body, 2<<30)); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	if err := VerifySHA256(tmpName, a.SHA256); err != nil {
		_ = os.Remove(tmpName)
		return "", err
	}
	return tmpName, nil
}

func (m Manager) InstallExecutable(ctx context.Context, a Artifact) error {
	if err := a.Validate(); err != nil {
		return err
	}
	if a.ArchiveType == "zip" {
		return m.installZIP(ctx, a)
	}
	if a.ArchiveType != "exe" {
		return fmt.Errorf("%s has unsupported artifact type %q", a.ToolID, a.ArchiveType)
	}
	tmp, err := m.Download(ctx, a)
	if err != nil {
		return err
	}
	defer os.Remove(tmp)
	destDir := filepath.Join(m.Root, a.ToolID, a.Version)
	if err := os.MkdirAll(destDir, 0o700); err != nil {
		return err
	}
	dest := m.Executable(a)
	stage := dest + ".new"
	if err := copyFile(stage, tmp, 0o700); err != nil {
		return err
	}
	if err := m.writeInstallMetadata(filepath.Join(destDir, installMetadataName), a); err != nil {
		return err
	}
	// A same-version reinstall replaces an existing working binary: preserve
	// it until the smoke test passes, restore it on any failure.
	backup := dest + ".previous"
	_ = os.Remove(backup)
	hadPrevious := false
	if _, err := os.Stat(dest); err == nil {
		if err := os.Rename(dest, backup); err != nil {
			return err
		}
		hadPrevious = true
	}
	if err := os.Rename(stage, dest); err != nil {
		if hadPrevious {
			_ = os.Rename(backup, dest)
		}
		return err
	}
	if err := m.smokeProbe(ctx, dest, a.Smoke); err != nil {
		_ = os.Remove(dest)
		if hadPrevious {
			if restoreErr := os.Rename(backup, dest); restoreErr != nil {
				return fmt.Errorf("%v; restoring the previous binary also failed: %w", err, restoreErr)
			}
		}
		return fmt.Errorf("%s %s: %w", a.ToolID, a.Version, err)
	}
	_ = os.Remove(backup)
	return nil
}

// UvToolPaths keeps every uv-owned file below Blunt Code's private tools
// directory.  It intentionally does not use a user's PATH, uv data directory,
// cache, or installed Python.
type UvToolPaths struct {
	Root       string
	Executable string
	RulesDir   string
	ToolDir    string
	CacheDir   string
	PythonDir  string
}

func (m Manager) UvToolPaths(a Artifact) UvToolPaths {
	root := filepath.Join(m.Root, a.ToolID, a.Version)
	return UvToolPaths{
		Root:       root,
		Executable: filepath.Join(root, a.Executable),
		RulesDir:   filepath.Join(root, "rules"),
		ToolDir:    filepath.Join(root, "env"),
		CacheDir:   filepath.Join(root, "cache"),
		PythonDir:  filepath.Join(root, "python"),
	}
}

func (m Manager) InstallUvTool(ctx context.Context, uv, tool Artifact) error {
	if uv.ToolID != "uv" || tool.InstallKind != "uv_tool" {
		return fmt.Errorf("invalid uv tool installation configuration")
	}
	if err := uv.Validate(); err != nil {
		return err
	}
	if err := tool.Validate(); err != nil {
		return err
	}
	if !m.IsReady(uv) {
		if err := m.InstallExecutable(ctx, uv); err != nil {
			return fmt.Errorf("install managed uv: %w", err)
		}
	}
	paths := m.UvToolPaths(tool)
	for _, dir := range []string{paths.Root, paths.ToolDir, paths.CacheDir, paths.PythonDir} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
	}
	wheel, err := m.verifiedWheel(ctx, tool, paths)
	if err != nil {
		return err
	}
	if err := m.runCommand(ctx, m.Executable(uv), []string{"tool", "install", "--managed-python", wheel}, paths.Root, uvToolEnv(paths)); err != nil {
		return fmt.Errorf("install %s: %w", tool.ToolID, err)
	}
	if _, err := os.Stat(paths.Executable); err != nil {
		return fmt.Errorf("install %s: expected executable missing: %w", tool.ToolID, err)
	}
	if err := m.writeInstallMetadata(filepath.Join(paths.Root, installMetadataName), tool); err != nil {
		return err
	}
	// A venv cannot be relocated after creation, so uv installs have no
	// in-place rollback: a failed smoke test removes the fresh tree outright.
	// The version-pinned layout keeps any other installed version untouched.
	if err := m.smokeProbe(ctx, paths.Executable, tool.Smoke); err != nil {
		_ = os.RemoveAll(paths.Root)
		return fmt.Errorf("%s %s: %w", tool.ToolID, tool.Version, err)
	}
	return nil
}

func (m Manager) verifiedWheel(ctx context.Context, tool Artifact, paths UvToolPaths) (string, error) {
	wheelDir := filepath.Join(paths.Root, "downloads")
	filename, err := wheelFilename(tool.SourceURL)
	if err != nil {
		return "", err
	}
	wheel := filepath.Join(wheelDir, filename)
	if err := VerifySHA256(wheel, tool.SHA256); err == nil {
		return wheel, nil
	}
	if err := os.MkdirAll(wheelDir, 0o700); err != nil {
		return "", err
	}
	temporary, err := m.Download(ctx, tool)
	if err != nil {
		return "", fmt.Errorf("download verified %s wheel: %w", tool.ToolID, err)
	}
	defer os.Remove(temporary)
	if err := os.Remove(wheel); err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("replace cached %s wheel: %w", tool.ToolID, err)
	}
	if err := os.Rename(temporary, wheel); err != nil {
		return "", fmt.Errorf("store verified %s wheel: %w", tool.ToolID, err)
	}
	return wheel, nil
}

func wheelFilename(sourceURL string) (string, error) {
	u, err := url.Parse(sourceURL)
	if err != nil {
		return "", fmt.Errorf("parse wheel URL: %w", err)
	}
	name := path.Base(u.Path)
	if !strings.HasSuffix(strings.ToLower(name), ".whl") || name == ".whl" {
		return "", fmt.Errorf("wheel URL does not end in a wheel filename")
	}
	return name, nil
}

func (m Manager) runCommand(ctx context.Context, executable string, args []string, dir string, env []string) error {
	if m.RunCommand != nil {
		return m.RunCommand(ctx, executable, args, dir, env)
	}
	cmd := exec.CommandContext(ctx, executable, args...)
	cmd.Dir = dir
	cmd.Env = env
	// A cancelled installer child (uv, Python) can leave pipe-holding
	// grandchildren behind; bound the wait so install cancellation returns.
	cmd.WaitDelay = 5 * time.Second
	// Installer output feeds error messages, not parsing, so it is capped
	// instead of collected wholesale: a broken installer dumping gigabytes
	// must not grow this process without bound.
	output := process.NewCappedBuffer(2 << 20)
	cmd.Stdout = output
	cmd.Stderr = output
	if err := cmd.Start(); err != nil {
		return err
	}
	if err := cmd.Wait(); err != nil {
		text := strings.TrimSpace(string(output.Bytes()))
		if text == "" {
			return err
		}
		if output.Truncated() {
			text += " (installer output truncated)"
		}
		return fmt.Errorf("%w: %s", err, text)
	}
	return nil
}

func uvToolEnv(paths UvToolPaths) []string {
	return mergeEnv(os.Environ(), map[string]string{
		"UV_CACHE_DIR":          paths.CacheDir,
		"UV_TOOL_DIR":           paths.ToolDir,
		"UV_TOOL_BIN_DIR":       paths.Root,
		"UV_PYTHON_INSTALL_DIR": paths.PythonDir,
		"UV_MANAGED_PYTHON":     "1",
	})
}

func mergeEnv(base []string, overrides map[string]string) []string {
	values := make(map[string]string, len(base)+len(overrides))
	for _, entry := range base {
		key, value, ok := strings.Cut(entry, "=")
		if ok {
			values[key] = value
		}
	}
	for key, value := range overrides {
		values[key] = value
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		out = append(out, key+"="+values[key])
	}
	return out
}

func (m Manager) installZIP(ctx context.Context, a Artifact) error {
	tmp, err := m.Download(ctx, a)
	if err != nil {
		return err
	}
	defer os.Remove(tmp)
	zr, err := zip.OpenReader(tmp)
	if err != nil {
		return fmt.Errorf("open %s archive: %w", a.ToolID, err)
	}
	defer zr.Close()
	destDir := filepath.Join(m.Root, a.ToolID, a.Version)
	if m.IsReady(a) {
		return nil
	}
	parent := filepath.Dir(destDir)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(parent, a.Version+".new-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	var extracted int64
	for _, f := range zr.File {
		name, err := safeArchivePath(f.Name)
		if err != nil {
			return fmt.Errorf("unsafe archive path %q", f.Name)
		}
		if f.FileInfo().Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("unsafe archive symlink %q", f.Name)
		}
		dest := filepath.Join(stage, filepath.FromSlash(name))
		if rel, err := filepath.Rel(stage, dest); err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return fmt.Errorf("unsafe archive path %q", f.Name)
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(dest, 0o700); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0o700); err != nil {
			return err
		}
		in, err := f.Open()
		if err != nil {
			return err
		}
		n, copyErr := copyReader(dest, in, 0o700)
		closeErr := in.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		extracted += n
		if extracted > 4<<30 {
			return fmt.Errorf("archive for %s exceeds extraction limit", a.ToolID)
		}
	}
	if _, err := os.Stat(filepath.Join(stage, filepath.FromSlash(a.Executable))); err != nil {
		return fmt.Errorf("archive for %s lacks %s", a.ToolID, a.Executable)
	}
	if err := m.writeInstallMetadata(filepath.Join(stage, installMetadataName), a); err != nil {
		return err
	}
	// The staged tree swaps in atomically; a configured smoke test then
	// verifies the activated binary, and any failure restores the previous
	// installation. Bat-launcher artifacts (SonarQube) declare no smoke test —
	// probing them would start the server.
	return m.activate(stage, destDir, func() error {
		return m.smokeProbe(ctx, m.Executable(a), a.Smoke)
	})
}

func safeArchivePath(name string) (string, error) {
	name = strings.ReplaceAll(name, "\\", "/")
	if name == "" || strings.HasPrefix(name, "/") || strings.Contains(name, ":") {
		return "", fmt.Errorf("absolute archive path")
	}
	clean := path.Clean(name)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("archive traversal")
	}
	return clean, nil
}

// activate swaps a fully staged tree into place and keeps the previous
// installation until the optional smoke test passes: dest → dest.previous,
// stage → dest, verify, and on any failure restore. A crash between the two
// renames leaves the pair on disk for SweepStaging to recover at next start.
func (m Manager) activate(stage, dest string, smoke func() error) error {
	backup := dest + ".previous"
	_ = os.RemoveAll(backup)
	hadPrevious := false
	if _, err := os.Stat(dest); err == nil {
		if err := os.Rename(dest, backup); err != nil {
			return err
		}
		hadPrevious = true
	}
	if err := os.Rename(stage, dest); err != nil {
		if hadPrevious {
			_ = os.Rename(backup, dest)
		}
		return err
	}
	if smoke != nil {
		if err := smoke(); err != nil {
			_ = os.RemoveAll(dest)
			if hadPrevious {
				if restoreErr := os.Rename(backup, dest); restoreErr != nil {
					return fmt.Errorf("%v; restoring the previous installation also failed: %w", err, restoreErr)
				}
			}
			return err
		}
	}
	_ = os.RemoveAll(backup)
	return nil
}

// installMetadataName is the provenance record written inside every installed
// artifact directory: what was installed, from where, verified how, and by
// which Blunt Code build. It travels with the staged tree, so activation is
// still a single rename.
const installMetadataName = "blunt-install.json"

func (m Manager) writeInstallMetadata(path string, a Artifact) error {
	record := map[string]any{
		"tool_id":           a.ToolID,
		"version":           a.Version,
		"platform":          a.Platform,
		"source_url":        a.SourceURL,
		"sha256":            a.SHA256,
		"archive_type":      a.ArchiveType,
		"verification":      "pinned sha256 digest comparison (compiled-in manifest); not a signature",
		"installed_at_utc":  time.Now().UTC().Format(time.RFC3339),
		"bluntcode_version": buildVersion(),
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o600)
}

// SweepStaging recovers from interrupted updates. Download temps under
// .downloads and extraction staging directories (<version>.new-*) are
// re-downloadable, so they are simply removed; a version directory that lost
// its activation race (dest renamed to .previous, process died before the
// staged rename) is restored from its backup. Existing installations are
// never touched. Call once at startup, before any readiness decision.
func (m Manager) SweepStaging() {
	if entries, err := os.ReadDir(filepath.Join(m.Root, ".downloads")); err == nil {
		for _, entry := range entries {
			_ = os.RemoveAll(filepath.Join(m.Root, ".downloads", entry.Name()))
		}
	}
	toolDirs, err := os.ReadDir(m.Root)
	if err != nil {
		return
	}
	for _, tool := range toolDirs {
		if !tool.IsDir() {
			continue
		}
		toolDir := filepath.Join(m.Root, tool.Name())
		versions, err := os.ReadDir(toolDir)
		if err != nil {
			continue
		}
		for _, entry := range versions {
			name := entry.Name()
			switch {
			case strings.Contains(name, ".new-"):
				// Stale extraction staging; nothing references it.
				_ = os.RemoveAll(filepath.Join(toolDir, name))
			case strings.HasSuffix(name, ".previous"):
				versionDir := filepath.Join(toolDir, strings.TrimSuffix(name, ".previous"))
				if _, err := os.Stat(versionDir); err == nil {
					// Activation completed but cleanup died: the backup is stale.
					_ = os.RemoveAll(filepath.Join(toolDir, name))
				} else {
					// Activation died between the two renames: restore.
					_ = os.Rename(filepath.Join(toolDir, name), versionDir)
				}
			}
		}
	}
}
func copyFile(dst, src string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	closeErr := out.Close()
	if err != nil {
		return err
	}
	return closeErr
}

func copyReader(dst string, in io.Reader, mode os.FileMode) (int64, error) {
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return 0, err
	}
	n, err := io.Copy(out, in)
	closeErr := out.Close()
	if err != nil {
		return n, err
	}
	return n, closeErr
}
