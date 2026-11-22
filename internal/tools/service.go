package tools

import (
	"context"
	"errors"
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
	DiskBytes  int64  `json:"disk_bytes,omitempty"`
}

type Service struct {
	Manager Manager
	mu      sync.RWMutex
	offline bool
	// locks serializes installs per managed tool: two workspaces (or a scan
	// racing the tools page) may request the same missing tool, and
	// unserialized installs interleave staging renames and rules extraction on
	// the same files. Different tools install concurrently; uv-managed tools
	// additionally share uvToolMu because they share the managed uv bootstrap.
	locksMu  sync.Mutex
	locks    map[string]*sync.Mutex
	uvToolMu sync.Mutex
}

func NewService(root string, manifest Manifest, offline bool) *Service {
	return &Service{Manager: Manager{Root: root, Manifest: manifest}, offline: offline}
}
func (s *Service) Offline() bool             { s.mu.RLock(); defer s.mu.RUnlock(); return s.offline }
func (s *Service) SetOffline(value bool)     { s.mu.Lock(); s.offline = value; s.mu.Unlock() }
func (s *Service) SetDataDir(dataDir string) { s.Manager.DataDir = dataDir }
func (s *Service) DiskUsage(id string) int64 { return s.Manager.DiskUsage(id) }

// InstallEstimate is the pinned-download size needed to make one or more
// managed tools ready. Bytes sums only the manifest-pinned archives that are
// still missing; Estimated is false only when every needed artifact has a
// pinned size and no uv-managed dependency resolution is involved.
type InstallEstimate struct {
	Bytes     int64 `json:"bytes"`
	Estimated bool  `json:"estimated"`
}

// InstallEstimates sums the missing pinned downloads for the given managed
// tool ids (analyzer or tools ids; aliases are canonicalized). Shared
// dependencies such as managed uv are counted once.
func (s *Service) InstallEstimates(ids []string) InstallEstimate {
	var estimate InstallEstimate
	seen := map[string]struct{}{}
	complete := true
	add := func(a Artifact) {
		key := a.ToolID + "|" + a.Version + "|" + a.Platform
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		if a.DownloadBytes > 0 {
			estimate.Bytes += a.DownloadBytes
		} else {
			complete = false
		}
	}
	for _, raw := range ids {
		id := CanonicalToolID(raw)
		if id == "sonarqube" {
			artifacts, err := s.sonarQubeArtifacts()
			if err != nil {
				complete = false
				continue
			}
			for _, artifact := range artifacts {
				if !s.Manager.IsReady(artifact) {
					add(artifact)
				}
			}
			continue
		}
		artifact, ok := s.Manager.Manifest.Find(id, platform())
		if !ok {
			complete = false
			continue
		}
		if artifact.InstallKind == "uv_tool" {
			paths := s.Manager.UvToolPaths(artifact)
			if _, err := os.Stat(paths.Executable); err == nil {
				continue
			}
			if !uvWheelCached(artifact, paths) {
				add(artifact)
			}
			// uv resolves the managed Python and wheel dependencies beyond the
			// pinned wheel, so its total download is not pinned in the manifest.
			complete = false
			if uv, ok := s.Manager.Manifest.Find("uv", platform()); ok && !s.Manager.IsReady(uv) {
				add(uv)
			}
			continue
		}
		if !s.Manager.IsReady(artifact) {
			add(artifact)
		}
	}
	estimate.Estimated = !complete
	return estimate
}

// uvWheelCached reports whether the verified wheel is already on disk, in which
// case InstallUvTool reuses it instead of downloading it again.
func uvWheelCached(artifact Artifact, paths UvToolPaths) bool {
	filename, err := wheelFilename(artifact.SourceURL)
	if err != nil {
		return false
	}
	wheel := filepath.Join(paths.Root, "downloads", filename)
	return VerifySHA256(wheel, artifact.SHA256) == nil
}

// SweepStaging recovers the tools directory from interrupted updates (see
// Manager.SweepStaging); run once at startup.
func (s *Service) SweepStaging() { s.Manager.SweepStaging() }
func platform() string           { return runtime.GOOS + "-" + runtime.GOARCH }
func (s *Service) Status(id string) (status Status) {
	defer func() {
		status.Name = toolName(id)
		if status.Ready {
			status.DiskBytes = s.Manager.DiskUsage(id)
		}
	}()
	if id == "sonarqube" {
		return s.sonarQubeStatus()
	}
	artifact, ok := s.Manager.Manifest.Find(id, platform())
	if !ok {
		return Status{ID: id, Detail: "Not installed. Blunt Code has no download for " + platform() + "; install it yourself and put it on PATH.", CanInstall: false}
	}
	if artifact.InstallKind == "uv_tool" {
		if artifact.ToolID == "semgrep" {
			return s.semgrepStatus(artifact)
		}
		return s.uvToolStatus(artifact)
	}
	ready := s.Manager.IsReady(artifact)
	detail := "Ready"
	if !ready {
		detail = "Not installed yet. Install it here or start a scan to set it up."
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
	case "iac-checkov":
		return "Checkov"
	case "semgrep":
		return "Semgrep"
	case "sonarqube":
		return "SonarQube"
	default:
		return id
	}
}
func (s *Service) All() []Status {
	ids := []string{"ruff", "biome", "gitleaks-secrets", "osv-dependencies", "container-trivy", "iac-checkov", "semgrep", "sonarqube"}
	out := make([]Status, 0, len(ids))
	for _, id := range ids {
		out = append(out, s.Status(id))
	}
	return out
}

// NotInstallableError reports a tool Blunt Code cannot set up by itself on
// this computer (no managed download for the platform). Scans record it as
// "not installed" so the UI can group every such tool into one notice.
type NotInstallableError struct {
	ID     string
	Detail string
}

func (e *NotInstallableError) Error() string { return toolName(e.ID) + ": " + e.Detail }

func (s *Service) Ensure(ctx context.Context, id string) error {
	lock := s.lockFor(id)
	lock.Lock()
	defer lock.Unlock()
	return s.ensure(ctx, id)
}

func (s *Service) lockFor(id string) *sync.Mutex {
	key := CanonicalToolID(id)
	s.locksMu.Lock()
	defer s.locksMu.Unlock()
	if s.locks == nil {
		s.locks = map[string]*sync.Mutex{}
	}
	lock, ok := s.locks[key]
	if !ok {
		lock = &sync.Mutex{}
		s.locks[key] = lock
	}
	return lock
}

// EnsureMany installs missing managed tools concurrently. Per-tool locks keep
// same-tool installs serialized, and uv-managed tools serialize on the shared
// uv bootstrap inside installUvTool, so independent downloads (for example
// Semgrep and the SonarQube bundle) no longer queue behind each other.
func (s *Service) EnsureMany(ctx context.Context, ids []string) error {
	unique := make([]string, 0, len(ids))
	seen := map[string]bool{}
	for _, raw := range ids {
		id := CanonicalToolID(raw)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		unique = append(unique, id)
	}
	if len(unique) == 0 {
		return nil
	}
	const maxParallelInstalls = 4
	sem := make(chan struct{}, maxParallelInstalls)
	var wg sync.WaitGroup
	errs := make([]error, len(unique))
	for i, id := range unique {
		wg.Add(1)
		go func(i int, id string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			errs[i] = s.Ensure(ctx, id)
		}(i, id)
	}
	wg.Wait()
	return errors.Join(errs...)
}

func (s *Service) ensure(ctx context.Context, id string) error {
	if id == "sonarqube" {
		return s.ensureSonarQube(ctx)
	}
	status := s.Status(id)
	if !status.CanInstall {
		return &NotInstallableError{ID: id, Detail: status.Detail}
	}
	if status.Ready {
		return nil
	}
	artifact, _ := s.Manager.Manifest.Find(id, platform())
	if artifact.InstallKind == "uv_tool" {
		paths := s.Manager.UvToolPaths(artifact)
		if _, err := os.Stat(paths.Executable); err == nil {
			if artifact.ToolID == "semgrep" {
				return ExtractSemgrepRules(paths.RulesDir)
			}
			return nil
		}
	}
	if s.Offline() {
		return fmt.Errorf("%s is not installed and offline mode is enabled", id)
	}
	if artifact.InstallKind == "uv_tool" {
		return s.installUvTool(ctx, artifact)
	}
	if artifact.ToolID == "uv" {
		s.uvToolMu.Lock()
		defer s.uvToolMu.Unlock()
		if s.Manager.IsReady(artifact) {
			return nil
		}
	}
	return s.Manager.InstallExecutable(ctx, artifact)
}

// Uninstall removes the tool's files and directories from disk.
// For sonarqube, it cleans up all sonarqube-related tool directories (sonarqube,
// sonarqube-server, sonar-scanner, java) as well as the sonarqube runtime/data directory in DataDir.
func (s *Service) Uninstall(ctx context.Context, id string) error {
	lock := s.lockFor(id)
	lock.Lock()
	defer lock.Unlock()
	return s.Manager.Uninstall(ctx, id)
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
	missing := make([]Artifact, 0, len(artifacts))
	for _, artifact := range artifacts {
		if !s.Manager.IsReady(artifact) {
			missing = append(missing, artifact)
		}
	}
	// The SonarQube bundle's server, scanner, and JDK archives are independent
	// downloads; installing them together removes the serial wait without
	// sharing any staging path.
	if len(missing) == 1 {
		if err := s.Manager.InstallExecutable(ctx, missing[0]); err != nil {
			return fmt.Errorf("install managed %s: %w", missing[0].ToolID, err)
		}
		return nil
	}
	var wg sync.WaitGroup
	errs := make([]error, len(missing))
	for i, artifact := range missing {
		wg.Add(1)
		go func(i int, artifact Artifact) {
			defer wg.Done()
			if err := s.Manager.InstallExecutable(ctx, artifact); err != nil {
				errs[i] = fmt.Errorf("install managed %s: %w", artifact.ToolID, err)
			}
		}(i, artifact)
	}
	wg.Wait()
	return errors.Join(errs...)
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
	paths := s.Manager.UvToolPaths(semgrep)
	if _, err := os.Stat(paths.Executable); err != nil {
		return Status{ID: semgrep.ToolID, Version: semgrep.Version, Detail: "Pinned Semgrep is not installed.", CanInstall: true}
	}
	if err := verifySemgrepRules(paths.RulesDir); err != nil {
		return Status{ID: semgrep.ToolID, Version: semgrep.Version, Detail: "Local offline rules are unavailable: " + err.Error(), CanInstall: true}
	}
	return Status{ID: semgrep.ToolID, Version: semgrep.Version, Ready: true, Detail: "Ready with bundled local rules " + SemgrepRulesVersion + ".", CanInstall: true}
}

// installUvTool installs any uv_tool artifact (semgrep, checkov) through the
// managed uv; semgrep additionally re-extracts its bundled local rules.
func (s *Service) installUvTool(ctx context.Context, artifact Artifact) error {
	s.uvToolMu.Lock()
	defer s.uvToolMu.Unlock()
	uv, ok := s.Manager.Manifest.Find("uv", platform())
	if !ok {
		return fmt.Errorf("%s requires a pinned managed uv artifact", artifact.ToolID)
	}
	if err := s.Manager.InstallUvTool(ctx, uv, artifact); err != nil {
		return err
	}
	if artifact.ToolID == "semgrep" {
		return ExtractSemgrepRules(s.Manager.UvToolPaths(artifact).RulesDir)
	}
	return nil
}

// uvToolStatus is the generic readiness check for non-semgrep uv tools: the
// manifest executable (for checkov, the tool venv's python.exe) existing is
// the whole contract.
func (s *Service) uvToolStatus(artifact Artifact) Status {
	paths := s.Manager.UvToolPaths(artifact)
	if _, err := os.Stat(paths.Executable); err != nil {
		return Status{ID: artifact.ToolID, Version: artifact.Version, Detail: "Pinned " + toolName(artifact.ToolID) + " is not installed.", CanInstall: true}
	}
	return Status{ID: artifact.ToolID, Version: artifact.Version, Ready: true, Detail: "Ready", CanInstall: true}
}

func (s *Service) SemgrepPaths() (UvToolPaths, bool) {
	artifact, ok := s.Manager.Manifest.Find("semgrep", platform())
	if !ok {
		return UvToolPaths{}, false
	}
	return s.Manager.UvToolPaths(artifact), true
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
