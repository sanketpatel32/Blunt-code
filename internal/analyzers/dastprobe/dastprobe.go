// Package dastprobe adapts the dynamic HTTP probe (internal/dast) to the
// analyzer contract so the pentest scan profile performs real DAST when the
// scan names a target URL. Findings are project-level (no file path): they
// describe the running application, not a line of source.
package dastprobe

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"bluntcode/internal/analyzers"
	"bluntcode/internal/dast"
)

const (
	ID      = "dast-probe"
	version = "1.0.0"
	// probeScope runs the header/CORS checks plus the same-host sensitive
	// path GETs (/.env, /.git/HEAD, ...): a pentest scan is the full sweep.
	probeScope = "full"

	planKeyTarget = "target_url"
)

type Adapter struct {
	// Offline reports the live offline-mode setting; when it returns true
	// probing is restricted to loopback targets so offline mode never sends
	// a request off the machine. Read per plan, so toggling offline mode in
	// Settings applies without a restart. Nil means online.
	Offline func() bool
}

func New() *Adapter { return &Adapter{} }

func (a *Adapter) offline() bool { return a.Offline != nil && a.Offline() }

func (a *Adapter) ID() string          { return ID }
func (a *Adapter) DisplayName() string { return "DAST Probe" }

// SupportedLanguages is every language: the probe is driven by the scan's
// target URL, not by files, and the orchestrator gates it on that target.
func (a *Adapter) SupportedLanguages() []analyzers.Language { return analyzers.AllLanguages() }

func (a *Adapter) Check(context.Context, analyzers.ToolEnvironment) analyzers.ToolStatus {
	detail := "built-in HTTP probe; runs on pentest scans given a target URL"
	if a.offline() {
		detail = "built-in HTTP probe; offline mode allows loopback targets only"
	}
	return analyzers.ToolStatus{Ready: true, Version: version, Detail: detail}
}

func (a *Adapter) EnsureInstalled(context.Context, analyzers.ToolEnvironment) error { return nil }

func (a *Adapter) Plan(_ context.Context, req analyzers.ScanRequest) (analyzers.AnalyzerPlan, error) {
	if req.Profile != analyzers.ProfilePentest {
		return analyzers.AnalyzerPlan{}, fmt.Errorf("dast-probe does not apply")
	}
	target, err := dast.NormalizeTarget(req.TargetURL)
	if err != nil {
		return analyzers.AnalyzerPlan{}, fmt.Errorf("dast-probe: %w", err)
	}
	if a.offline() && !dast.IsLoopbackTarget(target.String()) {
		return analyzers.AnalyzerPlan{}, fmt.Errorf("offline mode: dast-probe only probes loopback targets (localhost, 127.0.0.1, ::1); %s would leave this machine", target.Host)
	}
	return analyzers.AnalyzerPlan{
		AnalyzerID: ID,
		Version:    version,
		Metadata:   map[string]any{planKeyTarget: target.String(), "scope": probeScope},
	}, nil
}

func (a *Adapter) Run(ctx context.Context, p analyzers.AnalyzerPlan, _ analyzers.EventEmitter) (analyzers.AnalyzerResult, error) {
	started := time.Now()
	fail := func(err error) (analyzers.AnalyzerResult, error) {
		return analyzers.AnalyzerResult{Plan: p, ExitCode: 1, StartedAt: started, FinishedAt: time.Now()}, err
	}
	target, _ := p.Metadata[planKeyTarget].(string)
	if target == "" {
		return fail(fmt.Errorf("dast-probe plan has no target URL"))
	}
	resp, err := dast.Run(ctx, dast.ProbeRequest{TargetURL: target, Scope: probeScope})
	if err != nil {
		return fail(err)
	}
	// An unreachable target means nothing was probed. That is a failed run
	// (the scan completes with warnings), not a "high" finding about the app.
	for _, f := range resp.Findings {
		if f.ID == "dast.endpoint.unreachable" {
			return fail(fmt.Errorf("dast-probe could not reach %s: %s", target, f.Evidence))
		}
	}
	payload, err := json.Marshal(resp)
	if err != nil {
		return fail(fmt.Errorf("marshal dast-probe results: %w", err))
	}
	return analyzers.AnalyzerResult{Plan: p, Stdout: payload, StartedAt: started, FinishedAt: time.Now()}, nil
}

func (a *Adapter) Normalize(_ context.Context, result analyzers.AnalyzerResult) ([]analyzers.Finding, []analyzers.Metric, error) {
	if len(result.Stdout) == 0 {
		return nil, nil, nil
	}
	var resp dast.ProbeResponse
	if err := json.Unmarshal(result.Stdout, &resp); err != nil {
		return nil, nil, fmt.Errorf("unmarshal dast-probe results: %w", err)
	}
	findings := make([]analyzers.Finding, 0, len(resp.Findings))
	for _, d := range resp.Findings {
		severity := analyzers.Severity(d.Severity)
		if !validSeverity(severity) {
			severity = analyzers.SeverityInfo
		}
		f := analyzers.Finding{
			AnalyzerID:  ID,
			RuleID:      d.ID,
			Title:       d.Title,
			Message:     d.Message,
			Severity:    severity,
			Category:    analyzers.CategorySecurity,
			Remediation: d.Remediation,
			RawSeverity: d.Severity,
			Metadata: map[string]any{
				"target_url": resp.TargetURL,
				"owasp":      d.OWASP,
				"cwe":        d.CWE,
				"kind":       d.Category,
				"evidence":   d.Evidence,
			},
		}
		if !validSeverity(analyzers.Severity(d.Severity)) {
			analyzers.MarkSeverityUnmapped(&f)
		}
		findings = append(findings, f)
	}
	analyzers.SetFingerprints(findings)
	metrics := []analyzers.Metric{
		{AnalyzerID: ID, Key: "dast_risk_score", Label: "DAST risk score", Value: float64(resp.RiskScore)},
		{AnalyzerID: ID, Key: "dast_response_time", Label: "Target response time", Value: float64(resp.ResponseTimeMS), Unit: "ms"},
	}
	return findings, metrics, nil
}

func validSeverity(s analyzers.Severity) bool {
	switch s {
	case analyzers.SeverityCritical, analyzers.SeverityHigh, analyzers.SeverityMedium, analyzers.SeverityLow, analyzers.SeverityInfo:
		return true
	}
	return false
}
