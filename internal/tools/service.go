package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
)

type Status struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Version    string `json:"version,omitempty"`
	Ready      bool   `json:"ready"`
	Detail     string `json:"detail,omitempty"`
	CanInstall bool   `json:"can_install"`
}

type Service struct {
	Manager Manager
	mu      sync.RWMutex
	offline bool
	// installMu serializes installs into the shared tools directory: two
	// workspaces scanning concurrently (or a scan racing the tools page) may
	// request the same missing tool, and unserialized installs interleave
	// staging renames and rules extraction on the same files.
	installMu sync.Mutex
}

func NewService(root string, manifest Manifest, offline bool) *Service {
	return &Service{Manager: Manager{Root: root, Manifest: manifest}, offline: offline}
}
func (s *Service) Offline() bool         { s.mu.RLock(); defer s.mu.RUnlock(); return s.offline }
func (s *Service) SetOffline(value bool) { s.mu.Lock(); s.offline = value; s.mu.Unlock() }
func platform() string                   { return runtime.GOOS + "-" + runtime.GOARCH }
func (s *Service) Status(id string) (status Status) {
	defer func() { status.Name = toolName(id) }()
	if id == "sonarqube" {
		return s.sonarQubeStatus()
	}
	artifact, ok := s.Manager.Manifest.Find(id, platform())
	if !ok {
		return Status{ID: id, Detail: "No pinned managed artifact is configured.", CanInstall: false}
	}
	if artifact.InstallKind == "uv_tool" {
		return s.semgrepStatus(artifact)
	}
	ready := s.Manager.IsReady(artifact)
	detail := "Ready"
	if !ready {
		detail = "Pinned managed artifact is not installed."
	}
	return Status{ID: id, Version: artifact.Version, Ready: ready, Detail: detail, CanInstall: true}
}

func toolName(id string) string {
	switch id {
	case "ruff":
		return "Ruff"
	case "biome":
		return "Biome"
	case "gitleaks-secrets":
		return "Gitleaks"
	case "osv-dependencies":
		return "OSV Scanner"
	case "container-trivy":
		return "Trivy"
	case "semgrep":
		return "Semgrep"
	case "sonarqube":
		return "SonarQube"
	default:
		return id
	}
}
func (s *Service) All() []Status {
	ids := []string{"ruff", "biome", "gitleaks-secrets", "osv-dependencies", "container-trivy", "semgrep", "sonarqube"}
	out := make([]Status, 0, len(ids))
	for _, id := range ids {
		out = append(out, s.Status(id))
	}
	return out
}
func (s *Service) Ensure(ctx context.Context, id string) error {
	s.installMu.Lock()
	defer s.installMu.Unlock()
	if id == "sonarqube" {
		return s.ensureSonarQube(ctx)
	}
	status := s.Status(id)
	if !status.CanInstall {
		return fmt.Errorf("%s: %s", id, status.Detail)
	}
	if status.Ready {
		return nil
	}
	artifact, _ := s.Manager.Manifest.Find(id, platform())
	if artifact.InstallKind == "uv_tool" {
		paths := s.Manager.SemgrepPaths(artifact)
		if _, err := os.Stat(paths.Executable); err == nil {
			return ExtractSemgrepRules(paths.RulesDir)
		}
	}
	if s.Offline() {
		return fmt.Errorf("%s is not installed and offline mode is enabled", id)
	}
	if artifact.InstallKind == "uv_tool" {
		return s.installSemgrep(ctx, artifact)
	}
	return s.Manager.InstallExecutable(ctx, artifact)
}

func (s *Service) sonarQubeStatus() Status {
	artifacts, err := s.sonarQubeArtifacts()
	if err != nil {
		return Status{ID: "sonarqube", Detail: err.Error()}
	}
	for _, artifact := range artifacts {
		if !s.Manager.IsReady(artifact) {
			return Status{ID: "sonarqube", Version: artifacts[0].Version, Detail: "Pinned managed SonarQube bundle is not installed.", CanInstall: true}
		}
	}
	return Status{ID: "sonarqube", Version: artifacts[0].Version, Ready: true, Detail: "Ready (server, scanner, and Java).", CanInstall: true}
}

func (s *Service) ensureSonarQube(ctx context.Context) error {
	artifacts, err := s.sonarQubeArtifacts()
	if err != nil {
		return err
	}
	if s.Offline() {
		for _, artifact := range artifacts {
			if !s.Manager.IsReady(artifact) {
				return fmt.Errorf("sonarqube is not installed and offline mode is enabled")
			}
		}
		return nil
	}
	for _, artifact := range artifacts {
		if s.Manager.IsReady(artifact) {
			continue
		}
		if err := s.Manager.InstallExecutable(ctx, artifact); err != nil {
			return fmt.Errorf("install managed %s: %w", artifact.ToolID, err)
		}
	}
	return nil
}

func (s *Service) sonarQubeArtifacts() ([]Artifact, error) {
	ids := []string{"sonarqube-server", "sonar-scanner", "java"}
	artifacts := make([]Artifact, 0, len(ids))
	for _, id := range ids {
		artifact, ok := s.Manager.Manifest.Find(id, platform())
		if !ok {
			return nil, fmt.Errorf("SonarQube requires pinned managed %s artifact", id)
		}
		artifacts = append(artifacts, artifact)
	}
	return artifacts, nil
}

// SonarQubeArtifacts provides the adapter with the exact private, pinned
// bundle selected by the manifest. Callers cannot substitute PATH binaries.
func (s *Service) SonarQubeArtifacts() ([]Artifact, bool) {
	artifacts, err := s.sonarQubeArtifacts()
	return artifacts, err == nil
}

func (s *Service) semgrepStatus(semgrep Artifact) Status {
	paths := s.Manager.SemgrepPaths(semgrep)
	if _, err := os.Stat(paths.Executable); err != nil {
		return Status{ID: semgrep.ToolID, Version: semgrep.Version, Detail: "Pinned Semgrep is not installed.", CanInstall: true}
	}
	if err := verifySemgrepRules(paths.RulesDir); err != nil {
		return Status{ID: semgrep.ToolID, Version: semgrep.Version, Detail: "Local offline rules are unavailable: " + err.Error(), CanInstall: true}
	}
	return Status{ID: semgrep.ToolID, Version: semgrep.Version, Ready: true, Detail: "Ready with bundled local rules " + SemgrepRulesVersion + ".", CanInstall: true}
}

func (s *Service) installSemgrep(ctx context.Context, semgrep Artifact) error {
	uv, ok := s.Manager.Manifest.Find("uv", platform())
	if !ok {
		return fmt.Errorf("Semgrep requires a pinned managed uv artifact")
	}
	if err := s.Manager.InstallSemgrep(ctx, uv, semgrep); err != nil {
		return err
	}
	return ExtractSemgrepRules(s.Manager.SemgrepPaths(semgrep).RulesDir)
}

func (s *Service) SemgrepPaths() (SemgrepPaths, bool) {
	artifact, ok := s.Manager.Manifest.Find("semgrep", platform())
	if !ok {
		return SemgrepPaths{}, false
	}
	return s.Manager.SemgrepPaths(artifact), true
}

func verifySemgrepRules(dir string) error {
	version, err := os.ReadFile(filepath.Join(dir, "RULES_VERSION"))
	if err != nil {
		return fmt.Errorf("rules version marker missing")
	}
	if string(version) != SemgrepRulesVersion+"\n" {
		return fmt.Errorf("unsupported rules version")
	}
	if _, err := os.Stat(filepath.Join(dir, "blunt-code-local.yaml")); err != nil {
		return fmt.Errorf("rules file missing")
	}
	return nil
}
