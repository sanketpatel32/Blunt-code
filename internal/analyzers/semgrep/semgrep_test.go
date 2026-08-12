package semgrep

import (
	"bluntcode/internal/analyzers"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestNormalizeFixture(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "tests", "fixtures", "semgrep", "results.json"))
	if err != nil {
		t.Fatal(err)
	}
	got, _, err := New("semgrep.exe", "test", "rules").Normalize(context.Background(), analyzers.AnalyzerResult{Stdout: b, Plan: analyzers.AnalyzerPlan{Commands: []analyzers.ProcessSpec{{Dir: "."}}}})
	if err != nil || len(got) != 1 || got[0].Category != analyzers.CategorySecurity {
		t.Fatalf("got %#v, err %v", got, err)
	}
}

func TestPlanUsesOnlyLocalRulesAndDisablesSemgrepNetworkSignals(t *testing.T) {
	rules := filepath.Join(t.TempDir(), "rules")
	adapter := New(`C:\\BluntCode\\tools\\semgrep\\1.172.0\\semgrep.exe`, "1.172.0", rules)
	plan, err := adapter.Plan(context.Background(), analyzers.ScanRequest{
		WorkspaceRoot: `C:\\source`,
		Files:         []string{`C:\\source\\app.py`},
		Languages:     []analyzers.Language{analyzers.LanguagePython},
	})
	if err != nil {
		t.Fatal(err)
	}
	command := plan.Commands[0]
	if command.Executable != adapter.Executable || command.Dir != `C:\\source` {
		t.Fatalf("unexpected managed process: %#v", command)
	}
	wantArgs := []string{"scan", "--json", "--config", rules, "--no-rewrite-rule-ids", "--metrics=off", "--disable-version-check", "--oss-only", `C:\\source\\app.py`}
	if !reflect.DeepEqual(command.Args, wantArgs) {
		t.Fatalf("args = %#v, want %#v", command.Args, wantArgs)
	}
	for key, want := range map[string]string{
		"SEMGREP_APP_TOKEN": "", "SEMGREP_ENABLE_VERSION_CHECK": "0", "SEMGREP_SEND_METRICS": "off",
		"SEMGREP_SETTINGS_FILE": filepath.Join(filepath.Dir(rules), "settings.yml"),
	} {
		if command.Env[key] != want {
			t.Fatalf("%s = %q, want %q", key, command.Env[key], want)
		}
	}
	if plan.Metadata["offline"] != true || plan.Metadata["rules_source"] != "bundled_local" {
		t.Fatalf("unexpected plan metadata: %#v", plan.Metadata)
	}
}
