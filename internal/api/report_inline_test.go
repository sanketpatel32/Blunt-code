package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"bluntcode/internal/analyzers"
	"bluntcode/internal/core"
	"bluntcode/internal/database"
)

// TestReportsOpenInlineWhenAsked: ?inline=1 serves each full-report export
// for a browser tab (sandboxed), the default stays a download, and
// report.json is the findings.json document under the report.* name.
func TestReportsOpenInlineWhenAsked(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	work, err := s.db.CreateWorkspace(ctx, core.Workspace{RootPath: t.TempDir(), Name: "Example"})
	if err != nil {
		t.Fatal(err)
	}
	scan, err := s.db.CreateScan(ctx, core.Scan{WorkspaceID: work.ID, State: "completed", Profile: "standard",
		Snapshot: &core.ScanSnapshot{BluntCodeVersion: "9.9.9", CapturedAt: time.Now()}})
	if err != nil {
		t.Fatal(err)
	}
	f := finding("ruff", "F401", "src/main.py", "unused import", analyzers.SeverityHigh, analyzers.CategoryCorrectness)
	if _, err := s.db.SaveAnalyzerResult(ctx, scan.ID, database.AnalyzerRunInput{AnalyzerID: "ruff", Version: "test", State: "succeeded"}, []analyzers.Finding{f}, nil); err != nil {
		t.Fatal(err)
	}
	get := func(path string) *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		s.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/v1/scans/"+scan.ID+path, nil))
		return response
	}
	for _, format := range []string{"report.html", "report.sarif", "report.json"} {
		inline := get("/" + format + "?inline=1")
		if inline.Code != http.StatusOK || !strings.HasPrefix(inline.Header().Get("Content-Disposition"), "inline;") {
			t.Fatalf("%s inline: %d %q", format, inline.Code, inline.Header().Get("Content-Disposition"))
		}
		if !strings.HasPrefix(inline.Header().Get("Content-Security-Policy"), "sandbox") {
			t.Fatalf("%s inline must be sandboxed: %q", format, inline.Header().Get("Content-Security-Policy"))
		}
		if download := get("/" + format); !strings.HasPrefix(download.Header().Get("Content-Disposition"), "attachment;") {
			t.Fatalf("%s default must download: %q", format, download.Header().Get("Content-Disposition"))
		}
	}
	html := get("/report.html").Body.String()
	for _, want := range []string{"brand-mark", "Blunt Code", "Grade B", "1 of 1 analyzer ran"} {
		if !strings.Contains(html, want) {
			t.Fatalf("html report missing %q", want)
		}
	}
	if a, b := get("/report.json").Body.String(), get("/findings.json").Body.String(); a != b {
		t.Fatal("report.json must match findings.json")
	}
}
