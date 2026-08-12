package tools

import (
	"archive/zip"
	"context"
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
)

type Manager struct {
	Root       string
	Manifest   Manifest
	Client     *http.Client
	RunCommand func(context.Context, string, []string, string, []string) error
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
	return os.Rename(stage, dest)
}

// SemgrepPaths keeps every uv-owned file below Blunt Code's private tools
// directory.  It intentionally does not use a user's PATH, uv data directory,
// cache, or installed Python.
type SemgrepPaths struct {
	Root       string
	Executable string
	RulesDir   string
	ToolDir    string
	CacheDir   string
	PythonDir  string
}

func (m Manager) SemgrepPaths(a Artifact) SemgrepPaths {
	root := filepath.Join(m.Root, a.ToolID, a.Version)
	return SemgrepPaths{
		Root:       root,
		Executable: filepath.Join(root, a.Executable),
		RulesDir:   filepath.Join(root, "rules"),
		ToolDir:    filepath.Join(root, "env"),
		CacheDir:   filepath.Join(root, "cache"),
		PythonDir:  filepath.Join(root, "python"),
	}
}

func (m Manager) InstallSemgrep(ctx context.Context, uv, semgrep Artifact) error {
	if uv.ToolID != "uv" || semgrep.InstallKind != "uv_tool" {
		return fmt.Errorf("invalid uv/Semgrep installation configuration")
	}
	if err := uv.Validate(); err != nil {
		return err
	}
	if err := semgrep.Validate(); err != nil {
		return err
	}
	if !m.IsReady(uv) {
		if err := m.InstallExecutable(ctx, uv); err != nil {
			return fmt.Errorf("install managed uv: %w", err)
		}
	}
	paths := m.SemgrepPaths(semgrep)
	for _, dir := range []string{paths.Root, paths.ToolDir, paths.CacheDir, paths.PythonDir} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
	}
	wheel, err := m.verifiedSemgrepWheel(ctx, semgrep, paths)
	if err != nil {
		return err
	}
	if err := m.runCommand(ctx, m.Executable(uv), []string{"tool", "install", "--managed-python", wheel}, paths.Root, semgrepUVEnv(paths)); err != nil {
		return fmt.Errorf("install %s: %w", semgrep.ToolID, err)
	}
	if _, err := os.Stat(paths.Executable); err != nil {
		return fmt.Errorf("install %s: expected executable missing: %w", semgrep.ToolID, err)
	}
	return nil
}

func (m Manager) verifiedSemgrepWheel(ctx context.Context, semgrep Artifact, paths SemgrepPaths) (string, error) {
	wheelDir := filepath.Join(paths.Root, "downloads")
	filename, err := wheelFilename(semgrep.SourceURL)
	if err != nil {
		return "", err
	}
	wheel := filepath.Join(wheelDir, filename)
	if err := VerifySHA256(wheel, semgrep.SHA256); err == nil {
		return wheel, nil
	}
	if err := os.MkdirAll(wheelDir, 0o700); err != nil {
		return "", err
	}
	temporary, err := m.Download(ctx, semgrep)
	if err != nil {
		return "", fmt.Errorf("download verified %s wheel: %w", semgrep.ToolID, err)
	}
	defer os.Remove(temporary)
	if err := os.Remove(wheel); err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("replace cached %s wheel: %w", semgrep.ToolID, err)
	}
	if err := os.Rename(temporary, wheel); err != nil {
		return "", fmt.Errorf("store verified %s wheel: %w", semgrep.ToolID, err)
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
		return "", fmt.Errorf("Semgrep wheel URL does not end in a wheel filename")
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
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func semgrepUVEnv(paths SemgrepPaths) []string {
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
	return replaceDirectory(stage, destDir)
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

func replaceDirectory(stage, dest string) error {
	backup := dest + ".previous"
	_ = os.RemoveAll(backup)
	if _, err := os.Stat(dest); err == nil {
		if err := os.Rename(dest, backup); err != nil {
			return err
		}
	}
	if err := os.Rename(stage, dest); err != nil {
		if _, restoreErr := os.Stat(backup); restoreErr == nil {
			_ = os.Rename(backup, dest)
		}
		return err
	}
	_ = os.RemoveAll(backup)
	return nil
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
