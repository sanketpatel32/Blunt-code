package analyzers_test

// Routing-level tests for the broadened language classification. They wire
// the real adapters' SupportedLanguages through FilesForLanguages — exactly
// how scan orchestration builds each adapter's file set — to prove where the
// newly classified file types land: a Go file reaches both built-in
// detectors and none of the external linters, while a Python file still
// reaches every analyzer it always did. The external test package avoids an
// import cycle with the adapters that depend on analyzers.

import (
	"slices"
	"testing"

	"bluntcode/internal/analyzers"
	"bluntcode/internal/analyzers/biome"
	"bluntcode/internal/analyzers/ruff"
	"bluntcode/internal/analyzers/secrets"
	"bluntcode/internal/analyzers/semgrep"
	"bluntcode/internal/analyzers/sonarqube"
	"bluntcode/internal/analyzers/todo"
)

// routingAdapters collects every registered adapter with a dependency-free
// constructor. Only SupportedLanguages is exercised, so placeholder
// executables (and nil collaborators for the managed SonarQube lifecycle)
// are never touched.
func routingAdapters(t *testing.T) map[string]analyzers.Analyzer {
	t.Helper()
	return map[string]analyzers.Analyzer{
		"secrets":   secrets.New(),
		"todo":      todo.New(),
		"ruff":      ruff.New("ruff.exe", "test"),
		"biome":     biome.New("biome.exe", "test"),
		"semgrep":   semgrep.New("semgrep.exe", "test", "rules.yml"),
		"sonarqube": sonarqube.New(nil, nil, nil, nil),
	}
}

func TestRoutingGoFileReachesBuiltInsOnly(t *testing.T) {
	const goFile = `C:\ws\cmd\main.go`
	const envFile = `C:\ws\.env`
	files := []string{goFile, envFile}
	for name, adapter := range routingAdapters(t) {
		got := analyzers.FilesForLanguages(files, adapter.SupportedLanguages()...)
		receivesGo := slices.Contains(got, goFile)
		receivesEnv := slices.Contains(got, envFile)
		switch name {
		case "secrets":
			if !receivesGo || !receivesEnv {
				t.Fatalf("secrets must receive both %s and %s, got %v", goFile, envFile, got)
			}
		case "todo":
			if !receivesGo {
				t.Fatalf("todo must receive %s, got %v", goFile, got)
			}
			if receivesEnv {
				t.Fatalf("todo must not receive the .env credential blob, got %v", got)
			}
		default:
			// ruff, biome, semgrep, and sonarqube declare only the language
			// families their external tools actually lint.
			if receivesGo || receivesEnv {
				t.Fatalf("%s must not receive %s or %s, got %v", name, goFile, envFile, got)
			}
		}
	}
}

func TestRoutingPythonFileStillReachesAllItsAnalyzers(t *testing.T) {
	// The original routing matrix, unchanged by the broadening: a Python
	// file reaches ruff, semgrep, sonarqube, and both built-ins — but never
	// biome, which lints only the JS/TS family (and a TypeScript file
	// reaches biome but never ruff, the Python linter).
	cases := []struct {
		file    string
		reaches map[string]bool
	}{
		{
			file: `C:\ws\app.py`,
			reaches: map[string]bool{
				"secrets": true, "todo": true, "ruff": true, "semgrep": true, "sonarqube": true,
			},
		},
		{
			file: `C:\ws\app.ts`,
			reaches: map[string]bool{
				"secrets": true, "todo": true, "biome": true, "semgrep": true, "sonarqube": true,
			},
		},
	}
	for _, tc := range cases {
		for name, adapter := range routingAdapters(t) {
			got := analyzers.FilesForLanguages([]string{tc.file}, adapter.SupportedLanguages()...)
			if receives := slices.Contains(got, tc.file); receives != tc.reaches[name] {
				t.Errorf("%s: %s receives it = %v, want %v", tc.file, name, receives, tc.reaches[name])
			}
		}
	}
}

func TestRoutingCredentialAndConfigTypes(t *testing.T) {
	// One representative per newly classified family beyond Go: credentials,
	// shell, dockerfile, markdown, and a data format todo skips on purpose.
	files := []string{
		`C:\ws\server.pem`,
		`C:\ws\deploy.sh`,
		`C:\ws\Dockerfile`,
		`C:\ws\README.md`,
		`C:\ws\data.json`,
	}
	for name, adapter := range routingAdapters(t) {
		got := analyzers.FilesForLanguages(files, adapter.SupportedLanguages()...)
		switch name {
		case "secrets":
			// Secrets receives every one of them.
			if len(got) != len(files) {
				t.Fatalf("secrets must receive all of %v, got %v", files, got)
			}
		case "todo":
			// Markers plausibly live in shell scripts, Dockerfiles, and
			// markdown — but not in credential blobs or JSON.
			want := []string{`C:\ws\deploy.sh`, `C:\ws\Dockerfile`, `C:\ws\README.md`}
			if len(got) != len(want) || !slices.Contains(got, want[0]) || !slices.Contains(got, want[1]) || !slices.Contains(got, want[2]) {
				t.Fatalf("todo must receive exactly %v, got %v", want, got)
			}
		default:
			// ruff, biome, semgrep, and sonarqube keep ignoring the whole set.
			if len(got) != 0 {
				t.Fatalf("%s must not receive any of %v, got %v", name, files, got)
			}
		}
	}
}
