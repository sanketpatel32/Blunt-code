package scans

import (
	"context"

	"bluntcode/internal/analyzers"
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
}

// AnalyzerStatuses projects the capability inventory through the live
// registry. It is the single backend source behind GET /api/v1/analyzers and
// the Tools page: count, categories, profiles, network use, and readiness all
// read from here instead of hand-maintained UI lists.
func (s *Service) AnalyzerStatuses(ctx context.Context) []AnalyzerStatus {
	byID := map[string]analyzers.Analyzer{}
	for _, adapter := range s.registry.All() {
		byID[adapter.ID()] = adapter
	}
	out := make([]AnalyzerStatus, 0, len(analyzers.Capabilities()))
	for _, cap := range analyzers.Capabilities() {
		status := AnalyzerStatus{Capability: cap, Languages: []string{}}
		if adapter, ok := byID[cap.ID]; ok {
			status.Registered = true
			for _, lang := range adapter.SupportedLanguages() {
				status.Languages = append(status.Languages, string(lang))
			}
			tool := adapter.Check(ctx, analyzers.ToolEnvironment{ToolsDir: s.toolsDir})
			status.Version = tool.Version
			status.Ready = tool.Ready
			status.Detail = tool.Detail
		}
		out = append(out, status)
	}
	return out
}
