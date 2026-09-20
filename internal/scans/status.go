package scans

import (
	"context"
	"sync"
	"time"

	"bluntcode/internal/analyzers"
	"bluntcode/internal/tools"
)

// AnalyzerStatus is the capability inventory entry of one analyzer merged with
// its live registry state: the languages the adapter actually routes, the
// installed version, and readiness. Unregistered entries (built-ins withheld
// in offline mode) are still returned — with Registered false — so the UI can
// show the complete capability inventory instead of an undercount.
type AnalyzerStatus struct {
	analyzers.Capability
	Languages  []string `json:"languages"`
	Version    string   `json:"version,omitempty"`
	Ready      bool     `json:"ready"`
	Detail     string   `json:"detail,omitempty"`
	Registered bool     `json:"registered"`
	DiskBytes  int64    `json:"disk_bytes,omitempty"`
}

// analyzerStatusCacheTTL bounds how long one status snapshot is reused.
// Probing every tool binary costs seconds (each Check may exec --version) and
// the Tools page polls this endpoint, so a short cache keeps it cheap while
// staying fresh enough for install feedback.
const analyzerStatusCacheTTL = 30 * time.Second

// AnalyzerStatuses projects the capability inventory through the live
// registry. It is the single backend source behind GET /api/v1/analyzers and
// the Tools page: count, categories, profiles, network use, and readiness all
// read from here instead of hand-maintained UI lists. Results are probed
// concurrently and memoized for analyzerStatusCacheTTL (guarded by a mutex;
// each probe writes only its own row).
func (s *Service) AnalyzerStatuses(ctx context.Context) []AnalyzerStatus {
	s.statusMu.Lock()
	if s.statusCache != nil && time.Since(s.statusCachedAt) < analyzerStatusCacheTTL {
		cached := s.statusCache
		s.statusMu.Unlock()
		return cached
	}
	s.statusMu.Unlock()
	items := s.probeAnalyzerStatuses(ctx)
	s.statusMu.Lock()
	s.statusCache = items
	s.statusCachedAt = time.Now()
	s.statusMu.Unlock()
	return items
}

// probeAnalyzerStatuses builds one snapshot: every capability row is pre-
// placed in order, and registered adapters are probed in parallel goroutines,
// each writing only its own row, so the returned order is the capability
// inventory's order regardless of probe timing.
func (s *Service) probeAnalyzerStatuses(ctx context.Context) []AnalyzerStatus {
	byID := map[string]analyzers.Analyzer{}
	for _, adapter := range s.registry.All() {
		byID[adapter.ID()] = adapter
	}
	capabilities := analyzers.Capabilities()
	out := make([]AnalyzerStatus, len(capabilities))
	var wg sync.WaitGroup
	for i, capability := range capabilities {
		out[i] = AnalyzerStatus{Capability: capability, Languages: []string{}}
		adapter, ok := byID[capability.ID]
		if !ok {
			continue
		}
		out[i].Registered = true
		for _, lang := range adapter.SupportedLanguages() {
			out[i].Languages = append(out[i].Languages, string(lang))
		}
		status := &out[i]
		wg.Add(1)
		go func(status *AnalyzerStatus, adapter analyzers.Analyzer) {
			defer wg.Done()
			tool := adapter.Check(ctx, analyzers.ToolEnvironment{ToolsDir: s.toolsDir})
			status.Version = tool.Version
			status.Ready = tool.Ready
			status.Detail = tool.Detail
			if tool.Ready && s.tools != nil && status.ManagedTool != "" {
				status.DiskBytes = s.tools.DiskUsage(status.ManagedTool)
			}
		}(status, adapter)
	}
	wg.Wait()
	return out
}

// InvalidateStatusCache clears the memoized analyzer readiness cache, forcing
// the next request to probe tool binaries fresh.
func (s *Service) InvalidateStatusCache() {
	s.statusMu.Lock()
	s.statusCache = nil
	s.statusMu.Unlock()
}

// StopAnalyzer halts any managed long-running analyzer process (e.g. SonarQube JVM)
// before it is uninstalled or reconfigured.
func (s *Service) StopAnalyzer(ctx context.Context, id string) error {
	if s.registry == nil {
		return nil
	}
	adapter, ok := s.registry.Get(id)
	if !ok {
		adapter, ok = s.registry.Get(tools.CanonicalToolID(id))
		if !ok {
			return nil
		}
	}
	if shutdowner, ok := adapter.(interface{ Shutdown(context.Context) error }); ok {
		return shutdowner.Shutdown(ctx)
	}
	return nil
}
