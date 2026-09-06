// Package tools manages private, pinned analyzer binaries. It never searches
// PATH and it never builds a shell command from a workspace path.
//
// Trust mechanism, stated precisely: every artifact carries a SHA-256 digest
// pinned in the manifest compiled into this binary, and installation verifies
// the downloaded bytes against it over HTTPS before anything is activated.
// This is pinned-digest comparison, not a signed manifest — the trust root is
// the Blunt Code build itself, and changing any pin requires a new build.
// ChecksumURL is provenance for humans; the installer never consults it.
package tools

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

//go:embed manifest.json
var defaultManifestJSON []byte

func DefaultManifest() (Manifest, error) {
	var manifest Manifest
	if err := json.Unmarshal(defaultManifestJSON, &manifest); err != nil {
		return Manifest{}, err
	}
	return manifest, manifest.Validate()
}

// SmokeTest is the post-activation probe of one artifact: the installed
// executable is run with Args and must exit 0; when Expect is set, the
// combined output must mention it (case-insensitive) — normally the pinned
// version. Artifacts whose executable is a launcher with side effects
// (SonarQube's StartSonar.bat) declare none, and a missing probe is always
// valid: hash verification stays the mandatory gate, the smoke test is the
// extra "the bytes actually run" check.
type SmokeTest struct {
	Args   []string `json:"args"`
	Expect string   `json:"expect,omitempty"`
}

type Artifact struct {
	ToolID      string `json:"tool_id"`
	Version     string `json:"version"`
	Platform    string `json:"platform"`
	SourceURL   string `json:"source_url"`
	SHA256      string `json:"sha256"`
	ArchiveType string `json:"archive_type"` // exe, zip, or uv_tool wheel
	// Executable is relative to this artifact's private version directory.
	// It may point into an archive's top-level directory.
	Executable  string     `json:"executable"`
	LicenseURL  string     `json:"license_url"`
	ChecksumURL string     `json:"checksum_url,omitempty"`
	InstallKind string     `json:"install_kind,omitempty"` // artifact or uv_tool
	Package     string     `json:"package,omitempty"`
	Smoke       *SmokeTest `json:"smoke,omitempty"`
}

func (a Artifact) Validate() error {
	if a.ToolID == "" || a.Version == "" || a.Platform == "" || a.SourceURL == "" || a.Executable == "" {
		return fmt.Errorf("tool artifact has required fields missing")
	}
	if strings.EqualFold(a.Version, "latest") {
		return fmt.Errorf("tool %s cannot use latest", a.ToolID)
	}
	if a.InstallKind == "uv_tool" {
		if a.Package == "" || !strings.Contains(a.Package, "==") || strings.Contains(strings.ToLower(a.Package), "latest") {
			return fmt.Errorf("tool %s must pin its uv package", a.ToolID)
		}
	}
	if a.InstallKind != "" && a.InstallKind != "artifact" && a.InstallKind != "uv_tool" {
		return fmt.Errorf("tool %s has unsupported install kind %q", a.ToolID, a.InstallKind)
	}
	if len(a.SHA256) != 64 {
		return fmt.Errorf("tool %s has no pinned sha256", a.ToolID)
	}
	if _, err := hex.DecodeString(a.SHA256); err != nil {
		return fmt.Errorf("tool %s sha256: %w", a.ToolID, err)
	}
	if a.ArchiveType != "exe" && a.ArchiveType != "zip" && !(a.InstallKind == "uv_tool" && a.ArchiveType == "wheel") {
		return fmt.Errorf("tool %s has unsupported archive type %q", a.ToolID, a.ArchiveType)
	}
	if a.Smoke != nil && len(a.Smoke.Args) == 0 {
		return fmt.Errorf("tool %s declares a smoke test without arguments", a.ToolID)
	}
	if err := validateRelativeArchivePath(a.Executable); err != nil {
		return fmt.Errorf("tool %s executable: %w", a.ToolID, err)
	}
	return nil
}

func validateRelativeArchivePath(value string) error {
	value = strings.ReplaceAll(value, "\\", "/")
	if value == "" || strings.HasPrefix(value, "/") || strings.HasPrefix(value, "../") || strings.Contains(value, ":") {
		return fmt.Errorf("must be a relative path")
	}
	for _, part := range strings.Split(value, "/") {
		if part == "" || part == "." || part == ".." {
			return fmt.Errorf("must not contain traversal")
		}
	}
	return nil
}

type Manifest struct {
	Artifacts []Artifact `json:"artifacts"`
}

func (m Manifest) Validate() error {
	for _, a := range m.Artifacts {
		if err := a.Validate(); err != nil {
			return err
		}
	}
	return nil
}
func LoadManifest(path string) (Manifest, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, err
	}
	var m Manifest
	if err := json.Unmarshal(b, &m); err != nil {
		return Manifest{}, err
	}
	return m, m.Validate()
}
func (m Manifest) Find(toolID, platform string) (Artifact, bool) {
	for _, a := range m.Artifacts {
		if a.ToolID == toolID && a.Platform == platform {
			return a, true
		}
	}
	return Artifact{}, false
}

func VerifySHA256(path, expected string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err = io.Copy(h, f); err != nil {
		return err
	}
	actual := hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(actual, expected) {
		return fmt.Errorf("checksum mismatch for %s: got %s", filepath.Base(path), actual)
	}
	return nil
}
