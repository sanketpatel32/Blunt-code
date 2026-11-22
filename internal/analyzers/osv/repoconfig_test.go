package osv

import (
	"context"
	"os"
	"strings"
	"testing"

	"bluntcode/internal/analyzers"
)

func TestPlanPinsConfigByDefault(t *testing.T) {
	root := t.TempDir()
	req := analyzers.ScanRequest{WorkspaceRoot: root, Profile: analyzers.ProfileDeep, DependencyInputs: []string{"go.mod"}}
	plan, err := New("osv-scanner", "2.5.1").Plan(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	args := plan.Commands[0].Args
	config := ""
	for i := 0; i+1 < len(args); i++ {
		if args[i] == "--config" {
			config = args[i+1]
		}
	}
	if config == "" || strings.HasPrefix(config, root) {
		t.Fatalf("--config = %q, want a Blunt Code-owned file", config)
	}
	if data, err := os.ReadFile(config); err != nil || string(data) != pinnedConfig {
		t.Fatalf("pinned config = %q, %v", data, err)
	}
	req.TrustRepoConfig = true
	trusted, err := New("osv-scanner", "2.5.1").Plan(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	for _, arg := range trusted.Commands[0].Args {
		if arg == "--config" {
			t.Fatalf("trusted plan must keep per-directory osv-scanner.toml discovery: %#v", trusted.Commands[0].Args)
		}
	}
}
