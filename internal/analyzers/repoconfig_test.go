package analyzers

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsRepoAnalyzerConfig(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "pyproject.toml"), []byte("[tool.ruff]\nignore = [\"F401\"]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "pkg"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "pkg", "pyproject.toml"), []byte("[project]\nname = \"x\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{".gitleaks.toml", ".gitleaksignore", "trivy.yaml", ".trivyignore", "trivy-secret.yaml", ".checkov.yaml", "svc/.checkov.yml", "svc/osv-scanner.toml", "ruff.toml", "web/biome.json", "pyproject.toml"} {
		if !IsRepoAnalyzerConfig(root, rel) {
			t.Errorf("%s should be recognized as in-repo analyzer config", rel)
		}
	}
	for _, rel := range []string{"main.go", "pkg/pyproject.toml", "config.yaml", ".bluntcodeignore"} {
		if IsRepoAnalyzerConfig(root, rel) {
			t.Errorf("%s should not be recognized as in-repo analyzer config", rel)
		}
	}
}

func TestPinnedConfigFileIsContentAddressedOutsideWorkspace(t *testing.T) {
	first, err := PinnedConfigFile("bluntcode-test", ".toml", []byte("a = 1\n"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := PinnedConfigFile("bluntcode-test", ".toml", []byte("a = 1\n"))
	if err != nil {
		t.Fatal(err)
	}
	other, err := PinnedConfigFile("bluntcode-test", ".toml", []byte("a = 2\n"))
	if err != nil {
		t.Fatal(err)
	}
	if first != second || first == other {
		t.Fatalf("paths = %q %q %q, want stable per content", first, second, other)
	}
	if rel, err := filepath.Rel(os.TempDir(), first); err != nil || rel == "" || rel[0] == '.' {
		t.Fatalf("pinned config %q must live under the temp dir", first)
	}
	if data, err := os.ReadFile(first); err != nil || string(data) != "a = 1\n" {
		t.Fatalf("content = %q, %v", data, err)
	}
}
