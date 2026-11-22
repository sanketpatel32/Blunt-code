package dastprobe

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"bluntcode/internal/analyzers"
)

func TestPlanRequiresPentestProfileAndTarget(t *testing.T) {
	a := New()
	if _, err := a.Plan(context.Background(), analyzers.ScanRequest{Profile: analyzers.ProfileDeep, TargetURL: "http://localhost:1"}); err == nil {
		t.Error("dast-probe must not plan outside the pentest profile")
	}
	if _, err := a.Plan(context.Background(), analyzers.ScanRequest{Profile: analyzers.ProfilePentest}); err == nil {
		t.Error("dast-probe must not plan without a target URL")
	}
	plan, err := a.Plan(context.Background(), analyzers.ScanRequest{Profile: analyzers.ProfilePentest, TargetURL: "example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Metadata[planKeyTarget] != "http://example.com" {
		t.Errorf("plan target = %v, want the normalized URL", plan.Metadata[planKeyTarget])
	}
}

func TestOfflinePlanAllowsLoopbackOnly(t *testing.T) {
	a := New()
	a.Offline = func() bool { return true }
	if _, err := a.Plan(context.Background(), analyzers.ScanRequest{Profile: analyzers.ProfilePentest, TargetURL: "https://example.com"}); err == nil || !strings.Contains(err.Error(), "offline") {
		t.Errorf("offline plan for an external host = %v, want an offline refusal", err)
	}
	if _, err := a.Plan(context.Background(), analyzers.ScanRequest{Profile: analyzers.ProfilePentest, TargetURL: "http://127.0.0.1:8080"}); err != nil {
		t.Errorf("offline plan for loopback: %v", err)
	}
}

// TestRunProbesTargetAndNormalizesFindings drives a real HTTP probe against
// a deliberately weak local server and checks the findings reach the
// analyzer contract as security findings tagged with the target.
func TestRunProbesTargetAndNormalizesFindings(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if r.URL.Path == "/.env" {
			_, _ = w.Write([]byte("DB_PASSWORD=x"))
			return
		}
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Server", "nginx/1.18.0")
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	a := New()
	plan, err := a.Plan(context.Background(), analyzers.ScanRequest{Profile: analyzers.ProfilePentest, TargetURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	result, err := a.Run(context.Background(), plan, nil)
	if err != nil {
		t.Fatal(err)
	}
	findings, metrics, err := a.Normalize(context.Background(), result)
	if err != nil {
		t.Fatal(err)
	}
	rules := map[string]analyzers.Finding{}
	for _, f := range findings {
		rules[f.RuleID] = f
		if f.AnalyzerID != ID || f.Category != analyzers.CategorySecurity || f.Fingerprint == "" {
			t.Errorf("finding %s not normalized: %+v", f.RuleID, f)
		}
		if f.Metadata["target_url"] != server.URL {
			t.Errorf("finding %s target = %v, want %s", f.RuleID, f.Metadata["target_url"], server.URL)
		}
	}
	for _, want := range []string{"dast.headers.missing-csp", "dast.info.server-banner-leak", "dast.path.exposed-.env"} {
		if _, ok := rules[want]; !ok {
			t.Errorf("missing finding %s; got %v", want, keys(rules))
		}
	}
	if len(metrics) == 0 {
		t.Error("dast-probe should report a risk score metric")
	}
	if !contains(paths, "/.env") {
		t.Errorf("full-scope probe did not request sensitive paths: %v", paths)
	}
}

func TestRunFailsWhenTargetUnreachable(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	target := server.URL
	server.Close()
	a := New()
	plan, err := a.Plan(context.Background(), analyzers.ScanRequest{Profile: analyzers.ProfilePentest, TargetURL: target})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Run(context.Background(), plan, nil); err == nil {
		t.Error("an unreachable target probed nothing; the run must fail, not report a clean result")
	}
}

func keys(m map[string]analyzers.Finding) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func contains(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}
