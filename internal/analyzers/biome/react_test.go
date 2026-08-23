package biome

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"bluntcode/internal/analyzers"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestDetectReact(t *testing.T) {
	cases := []struct {
		name             string
		arrange          func(t *testing.T, root string) []string
		want             bool
		wantReasonSubstr string
	}{
		{
			name: "react in dependencies",
			arrange: func(t *testing.T, root string) []string {
				writeFile(t, filepath.Join(root, "package.json"), `{"name":"app","dependencies":{"react":"^18.2.0","vue":"no"}}`)
				return []string{filepath.Join(root, "src", "app.ts")}
			},
			want:             true,
			wantReasonSubstr: "react ^18.2.0 found in package.json",
		},
		{
			name: "react-dom in devDependencies of a monorepo package",
			arrange: func(t *testing.T, root string) []string {
				writeFile(t, filepath.Join(root, "package.json"), `{"name":"mono","dependencies":{"left-pad":"1.0.0"}}`)
				writeFile(t, filepath.Join(root, "packages", "web", "package.json"), `{"name":"web","devDependencies":{"react-dom":"19.0.0"}}`)
				return []string{filepath.Join(root, "packages", "web", "src", "main.tsx")}
			},
			want:             true,
			wantReasonSubstr: "react-dom 19.0.0 found in packages/web/package.json",
		},
		{
			name: "malformed package.json falls back to import heuristic",
			arrange: func(t *testing.T, root string) []string {
				writeFile(t, filepath.Join(root, "package.json"), `{ not json at all`)
				app := filepath.Join(root, "src", "App.tsx")
				writeFile(t, app, "import { useEffect } from \"react\";\nexport function App() { useEffect(() => {}); }\n")
				return []string{app}
			},
			want:             true,
			wantReasonSubstr: "react imported in App.tsx",
		},
		{
			name: "next import counts as react",
			arrange: func(t *testing.T, root string) []string {
				page := filepath.Join(root, "app", "page.tsx")
				writeFile(t, page, "import Link from \"next/link\";\nexport default function Page() { return <Link href=\"/\"/>; }\n")
				return []string{page}
			},
			want:             true,
			wantReasonSubstr: "next imported in page.tsx",
		},
		{
			name: "clean vue project is not detected",
			arrange: func(t *testing.T, root string) []string {
				writeFile(t, filepath.Join(root, "package.json"), `{"name":"vue-app","dependencies":{"vue":"^3.4.0"}}`)
				main := filepath.Join(root, "src", "main.ts")
				writeFile(t, main, "import { createApp } from \"vue\";\n")
				comp := filepath.Join(root, "src", "comp.tsx")
				writeFile(t, comp, "import { defineComponent } from \"vue\";\nexport const C = defineComponent({});\n")
				return []string{main, comp}
			},
			want: false,
		},
		{
			name: "jsx without any package.json is detected",
			arrange: func(t *testing.T, root string) []string {
				return []string{filepath.Join(root, "src", "Widget.jsx")}
			},
			want:             true,
			wantReasonSubstr: "without a package.json",
		},
		{
			name: "empty workspace is not detected",
			arrange: func(t *testing.T, root string) []string {
				return nil
			},
			want: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			files := tc.arrange(t, root)
			got := detectReact(root, files)
			if got.Detected != tc.want {
				t.Fatalf("detectReact = %+v, want Detected=%v", got, tc.want)
			}
			if tc.wantReasonSubstr != "" && !strings.Contains(got.Reason, tc.wantReasonSubstr) {
				t.Fatalf("reason %q does not contain %q", got.Reason, tc.wantReasonSubstr)
			}
		})
	}
}

func TestReactDomainConfigContainsReactKnob(t *testing.T) {
	data := reactDomainConfig()
	var parsed struct {
		Linter struct {
			Enabled bool              `json:"enabled"`
			Domains map[string]string `json:"domains"`
		} `json:"linter"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("generated config is not valid JSON: %v (%s)", err, data)
	}
	if !parsed.Linter.Enabled || parsed.Linter.Domains["react"] != "recommended" {
		t.Fatalf("generated config = %s, want linter.enabled with domains.react=recommended", data)
	}
	again := reactDomainConfig()
	if !reflect.DeepEqual(data, again) {
		t.Fatalf("generated config is not deterministic:\n%s\n%s", data, again)
	}
}

func TestWriteInjectedConfigIsStableAndReadable(t *testing.T) {
	first, ok := writeInjectedConfig()
	if !ok {
		t.Fatal("writeInjectedConfig failed")
	}
	second, ok := writeInjectedConfig()
	if !ok || first != second {
		t.Fatalf("writeInjectedConfig is not stable: %q vs %q (ok=%v)", first, second, ok)
	}
	data, err := os.ReadFile(first)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"react": "recommended"`) {
		t.Fatalf("written config = %s, want the react domain knob", data)
	}
}

func TestPlanInjectsReactDomainConfig(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "package.json"), `{"name":"app","dependencies":{"react":"18.2.0"}}`)
	app := filepath.Join(root, "src", "App.tsx")
	writeFile(t, app, "import { useState } from \"react\";\nexport function App() { return useState(0); }\n")
	plan, err := New("biome.exe", "test").Plan(context.Background(), analyzers.ScanRequest{WorkspaceRoot: root, Files: []string{app}, Languages: []analyzers.Language{analyzers.LanguageTypeScript}})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Commands) != 1 {
		t.Fatalf("commands = %#v", plan.Commands)
	}
	args := plan.Commands[0].Args
	if !reflect.DeepEqual(args[:3], []string{"lint", "--reporter=json", "--no-errors-on-unmatched"}) {
		t.Fatalf("base prefix changed: %#v", args[:3])
	}
	if len(args) != 5 || !strings.HasPrefix(args[3], "--config-path=") || args[4] != app {
		t.Fatalf("args = %#v, want --config-path followed by the selected file", args)
	}
	configPath := strings.TrimPrefix(args[3], "--config-path=")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("injected config %q is unreadable: %v", configPath, err)
	}
	if !strings.Contains(string(data), `"react": "recommended"`) {
		t.Fatalf("injected config = %s, want the react domain knob", data)
	}
	reason, _ := plan.Metadata["react_domain"].(string)
	if reason == "" {
		t.Fatalf("plan metadata = %#v, want the detection reason", plan.Metadata)
	}
	if !strings.Contains(reason, "react 18.2.0 found in package.json") {
		t.Fatalf("metadata reason = %q", reason)
	}
}

func TestPlanPlainTypeScriptWorkspaceKeepsPreReactCommandLine(t *testing.T) {
	// Regression guard: a workspace that detection classifies as non-React
	// must produce the exact command line the adapter built before the
	// react domain existed, or findings and fingerprints could drift.
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "package.json"), `{"name":"plain","dependencies":{"typescript":"5.6.0"}}`)
	main := filepath.Join(root, "src", "main.ts")
	writeFile(t, main, "export const answer = 42;\n")
	plan, err := New("biome.exe", "test").Plan(context.Background(), analyzers.ScanRequest{WorkspaceRoot: root, Files: []string{main}, Languages: []analyzers.Language{analyzers.LanguageTypeScript}})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"lint", "--reporter=json", "--no-errors-on-unmatched", main}
	if len(plan.Commands) != 1 || !reflect.DeepEqual(plan.Commands[0].Args, want) {
		t.Fatalf("commands = %#v, want single command with args %#v", plan.Commands, want)
	}
	if plan.Metadata != nil {
		t.Fatalf("metadata = %#v, want nil for non-React workspaces", plan.Metadata)
	}
}

func TestPlanKeepsWorkspaceOwnedBiomeConfig(t *testing.T) {
	// A workspace that manages its own biome.json already decides which
	// domains apply; --config-path would disable its resolution, so the
	// adapter must not inject even for a detected React project.
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "package.json"), `{"name":"app","dependencies":{"react":"18.2.0"}}`)
	writeFile(t, filepath.Join(root, "biome.json"), `{"linter":{"enabled":true}}`)
	app := filepath.Join(root, "src", "App.tsx")
	writeFile(t, app, "import { useState } from \"react\";\n")
	plan, err := New("biome.exe", "test").Plan(context.Background(), analyzers.ScanRequest{WorkspaceRoot: root, Files: []string{app}, Languages: []analyzers.Language{analyzers.LanguageTypeScript}})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"lint", "--reporter=json", "--no-errors-on-unmatched", app}
	if len(plan.Commands) != 1 || !reflect.DeepEqual(plan.Commands[0].Args, want) {
		t.Fatalf("commands = %#v, want single command with args %#v", plan.Commands, want)
	}
	if plan.Metadata != nil {
		t.Fatalf("metadata = %#v, want nil when the workspace config governs", plan.Metadata)
	}
}
