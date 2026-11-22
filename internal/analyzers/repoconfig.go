package analyzers

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// Repository-owned analyzer configuration is an audit-integrity boundary.
// Upstream tools auto-load config files from the scanned tree (.gitleaks.toml,
// .gitleaksignore, trivy.yaml, .trivyignore, trivy-secret.yaml, .checkov.yaml,
// osv-scanner.toml, ruff.toml/pyproject.toml, biome.json), and those files can
// disable rules or exclude paths. A repository under audit must not be able to
// configure its own audit, so by default every external adapter runs with
// Blunt Code-owned configuration and ignores in-repo analyzer config.
// ScanRequest.TrustRepoConfig is the explicit, per-workspace opt-in that
// restores the tools' native config discovery.

// repoConfigBasenames are the file names external analyzers auto-load from the
// scanned tree. pyproject.toml only counts when it carries a ruff section.
var repoConfigBasenames = map[string]bool{
	".gitleaks.toml":    true,
	".gitleaksignore":   true,
	"trivy.yaml":        true,
	".trivyignore":      true,
	".trivyignore.yaml": true,
	"trivy-secret.yaml": true,
	".checkov.yaml":     true,
	".checkov.yml":      true,
	"osv-scanner.toml":  true,
	"ruff.toml":         true,
	".ruff.toml":        true,
	"biome.json":        true,
	"biome.jsonc":       true,
}

// IsRepoAnalyzerConfig reports whether a workspace-relative path names an
// analyzer config file an external tool would load from the scanned tree.
// root is used to inspect pyproject.toml for a [tool.ruff] section.
func IsRepoAnalyzerConfig(root, relPath string) bool {
	base := path.Base(filepath.ToSlash(relPath))
	if repoConfigBasenames[base] {
		return true
	}
	if base != "pyproject.toml" || root == "" {
		return false
	}
	data, ok := readSmallFile(filepath.Join(root, filepath.FromSlash(relPath)), 1<<20)
	return ok && strings.Contains(string(data), "[tool.ruff")
}

func readSmallFile(path string, limit int64) ([]byte, bool) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > limit {
		return nil, false
	}
	data, err := os.ReadFile(path)
	return data, err == nil
}

// PinnedConfigFile writes Blunt Code-owned analyzer configuration to a
// content-addressed file under the system temp directory (never inside the
// scanned tree) and returns its path. Identical content maps to the same path,
// so repeated scans share one file.
func PinnedConfigFile(prefix, ext string, data []byte) (string, error) {
	sum := sha256.Sum256(data)
	dir := filepath.Join(os.TempDir(), "bluntcode-analyzer-config")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create pinned analyzer config dir: %w", err)
	}
	target := filepath.Join(dir, fmt.Sprintf("%s-%s%s", prefix, hex.EncodeToString(sum[:8]), ext))
	if existing, err := os.ReadFile(target); err == nil && string(existing) == string(data) {
		return target, nil
	}
	tmp, err := os.CreateTemp(dir, prefix+"-*.tmp")
	if err != nil {
		return "", fmt.Errorf("write pinned analyzer config: %w", err)
	}
	name := tmp.Name()
	_, writeErr := tmp.Write(data)
	closeErr := tmp.Close()
	if writeErr != nil || closeErr != nil {
		os.Remove(name)
		return "", fmt.Errorf("write pinned analyzer config: %v", firstErr(writeErr, closeErr))
	}
	if err := os.Rename(name, target); err != nil {
		// On Windows a concurrent reader can hold the target open; the name
		// encodes the content hash, so an existing file is already correct.
		os.Remove(name)
		if existing, readErr := os.ReadFile(target); readErr != nil || string(existing) != string(data) {
			return "", fmt.Errorf("install pinned analyzer config: %w", err)
		}
	}
	return target, nil
}

// IsolatedWorkDir creates an empty Blunt Code-owned working directory for one
// analyzer run, so tools that resolve config relative to the process working
// directory find nothing the scanned repository controls. The caller removes
// it after the run.
func IsolatedWorkDir(prefix string) (string, error) {
	dir, err := os.MkdirTemp(os.TempDir(), "bluntcode-"+prefix+"-cwd-*")
	if err != nil {
		return "", fmt.Errorf("create isolated analyzer working directory: %w", err)
	}
	return dir, nil
}

func firstErr(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}
