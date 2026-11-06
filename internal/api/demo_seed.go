package api

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"bluntcode/internal/analyzers"
	"bluntcode/internal/core"
	"bluntcode/internal/database"
)

// seedDemo inserts clearly tagged synthetic workspaces and scans for UI testing.
// It only runs when the application data directory path contains "demo" (case-insensitive),
// so production installs are never auto-filled.
func (s *Server) seedDemo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		fail(w, 405, "METHOD_NOT_ALLOWED", "Method not allowed.")
		return
	}
	dataDir := strings.ToLower(filepath.Clean(s.paths.DataDir))
	if !strings.Contains(dataDir, "demo") {
		fail(w, 403, "DEMO_DISABLED", "Demo seed is only allowed when the app data directory path contains \"demo\". This never runs on a normal install.")
		return
	}
	var input struct {
		Preset string `json:"preset"`
	}
	if err := decode(r, &input); err != nil {
		fail(w, 400, "INVALID_JSON", invalidJSONMessage)
		return
	}
	preset := strings.ToLower(strings.TrimSpace(input.Preset))
	if preset == "" {
		preset = "small"
	}
	ctx := r.Context()
	var summary map[string]any
	var err error
	switch preset {
	case "small":
		summary, err = seedDemoSmall(ctx, s.db)
	case "large", "stress":
		summary, err = seedDemoLarge(ctx, s.db, preset == "stress")
	default:
		fail(w, 400, "INVALID_PRESET", `preset must be "small", "large", or "stress".`)
		return
	}
	if err != nil {
		fail(w, 500, "DEMO_SEED_FAILED", err.Error())
		return
	}
	writeJSON(w, 200, summary)
}

func seedDemoSmall(ctx context.Context, db *database.DB) (map[string]any, error) {
	const n = 3
	ids := make([]string, 0, n)
	for i := 0; i < n; i++ {
		dir, err := os.MkdirTemp("", "bluntcode-demo-small-*")
		if err != nil {
			return nil, err
		}
		name := fmt.Sprintf("[Demo UI] Project %d", i+1)
		w, err := db.CreateWorkspace(ctx, core.Workspace{Name: name, RootPath: dir})
		if err != nil {
			return nil, err
		}
		if err := db.SetWorkspaceTags(ctx, w.ID, []string{"demo-ui", fmt.Sprintf("batch-%d", i%2)}); err != nil {
			return nil, err
		}
		scan, err := db.CreateScan(ctx, core.Scan{WorkspaceID: w.ID, State: "queued", Profile: "standard"})
		if err != nil {
			return nil, err
		}
		f := analyzers.Finding{AnalyzerID: "semgrep", RuleID: "demo-rule", RelativePath: fmt.Sprintf("src/deep/nested/path/component_%d/extra_long_segment_for_visual_wrap/service.go", i), Message: "Synthetic demo finding for layout testing.", Severity: analyzers.SeverityHigh, Category: analyzers.CategorySecurity}
		f.SetFingerprint()
		findings := []analyzers.Finding{f}
		if _, err := db.SaveAnalyzerResult(ctx, scan.ID, database.AnalyzerRunInput{AnalyzerID: "semgrep", Version: "demo", State: "succeeded", StartedAt: time.Now().UTC().Add(-2 * time.Second), FinishedAt: time.Now().UTC()}, findings, nil); err != nil {
			return nil, err
		}
		if err := db.CompleteScan(ctx, scan.ID, "completed", ""); err != nil {
			return nil, err
		}
		ids = append(ids, w.ID)
	}
	return map[string]any{"preset": "small", "workspaces": len(ids), "workspace_ids": ids, "label": "Synthetic demo dataset — tagged demo-ui; safe to delete workspaces from the app."}, nil
}

func seedDemoLarge(ctx context.Context, db *database.DB, stress bool) (map[string]any, error) {
	nWorkspaces := 80
	perScanFindings := 350
	if stress {
		perScanFindings = 12000
		nWorkspaces = 1
	}
	ids := make([]string, 0, nWorkspaces)
	totalFindings := 0
	severities := []analyzers.Severity{analyzers.SeverityCritical, analyzers.SeverityHigh, analyzers.SeverityMedium, analyzers.SeverityLow, analyzers.SeverityInfo}
	for w := 0; w < nWorkspaces; w++ {
		dir, err := os.MkdirTemp("", "bluntcode-demo-large-*")
		if err != nil {
			return nil, err
		}
		longRoot := filepath.Join(dir, "very", "long", "synthetic", "path", fmt.Sprintf("repo_segment_%03d", w), "checkout")
		if err := os.MkdirAll(longRoot, 0o755); err != nil {
			return nil, err
		}
		name := fmt.Sprintf("[Demo UI] Corpus %03d — %s", w+1, strings.Repeat("wide-name-", 3))
		created, err := db.CreateWorkspace(ctx, core.Workspace{Name: name, RootPath: longRoot})
		if err != nil {
			return nil, err
		}
		tags := []string{"demo-ui", fmt.Sprintf("squad-%d", w%7), "sample-data"}
		if err := db.SetWorkspaceTags(ctx, created.ID, tags); err != nil {
			return nil, err
		}
		scansToMake := 1
		if !stress && w < 18 {
			scansToMake = 24
		}
		for s := 0; s < scansToMake; s++ {
			st := "completed"
			if !stress && s == 0 && w%11 == 0 {
				st = "failed"
			}
			scan, err := db.CreateScan(ctx, core.Scan{WorkspaceID: created.ID, State: "queued", Profile: "standard"})
			if err != nil {
				return nil, err
			}
			nFind := perScanFindings
			if !stress && s < scansToMake-1 {
				nFind = 12 + (s+w)%20
			}
			findings := make([]analyzers.Finding, 0, nFind)
			for i := 0; i < nFind; i++ {
				sev := severities[(i+w+s)%len(severities)]
				path := fmt.Sprintf("src/packages/module_%03d/deeply/nested/file_%05d_%s.ts", (w+i)%40, i, strings.Repeat("x", (i%5)*3))
				msg := fmt.Sprintf("[Synthetic demo %d] Issue body for pagination and filter testing.", i+10000*w)
				f := analyzers.Finding{AnalyzerID: "ruff", RuleID: fmt.Sprintf("DEMO-%04d", i%500), RelativePath: path, Message: msg, Severity: sev, Category: analyzers.CategoryCorrectness}
				f.SetFingerprint()
				findings = append(findings, f)
			}
			if _, err := db.SaveAnalyzerResult(ctx, scan.ID, database.AnalyzerRunInput{AnalyzerID: "ruff", Version: "demo", State: "succeeded", StartedAt: time.Now().UTC().Add(-time.Duration(s+1) * time.Second), FinishedAt: time.Now().UTC()}, findings, nil); err != nil {
				return nil, err
			}
			if err := db.CompleteScan(ctx, scan.ID, st, ""); err != nil {
				return nil, err
			}
			if s == scansToMake-1 {
				totalFindings += nFind
			}
		}
		ids = append(ids, created.ID)
	}
	preset := "large"
	if stress {
		preset = "stress"
	}
	return map[string]any{
		"preset":          preset,
		"workspaces":      len(ids),
		"workspace_ids":   ids,
		"approx_findings": totalFindings,
		"label":           "Synthetic demo dataset — workspaces tagged demo-ui; remove from Blunt Code when finished testing.",
	}, nil
}
