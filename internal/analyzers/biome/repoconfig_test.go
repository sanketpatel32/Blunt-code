package biome

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"bluntcode/internal/analyzers"
)

func TestPlanPinsBiomeConfigOverWorkspaceConfigByDefault(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "biome.json"), `{"linter":{"enabled":false}}`)
	main := filepath.Join(root, "src", "main.ts")
	writeFile(t, main, "export const answer = 42;\n")
	plan, err := New("biome", "2.5.6").Plan(context.Background(), analyzers.ScanRequest{WorkspaceRoot: root, Files: []string{main}, Languages: []analyzers.Language{analyzers.LanguageTypeScript}})
	if err != nil {
		t.Fatal(err)
	}
	var configPath string
	for _, arg := range plan.Commands[0].Args {
		if strings.HasPrefix(arg, "--config-path=") {
			configPath = strings.TrimPrefix(arg, "--config-path=")
		}
	}
	if configPath == "" || strings.HasPrefix(configPath, root) {
		t.Fatalf("args %#v must pin a Blunt Code-owned biome config", plan.Commands[0].Args)
	}
	if data, err := os.ReadFile(configPath); err != nil || string(data) != string(defaultConfig()) {
		t.Fatalf("pinned config = %q, %v", data, err)
	}
}
