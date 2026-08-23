package biome

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Biome 2.x groups framework-specific rules into "domains"; the react domain
// bundles the hooks rules (useExhaustiveDependencies, useHookAtTopLevel, ...).
// Biome enables the domain by itself only when its dependency sniff finds
// react in a scanned package.json, so React code in workspaces without that
// manifest entry (missing or malformed package.json, monorepo manifests the
// sniff does not reach, react hoisted implicitly) lints with the domain off.
// The config generated here forces the domain on for those workspaces.
//
// Target schema: Biome 2.5.6, the version pinned in internal/tools/manifest.json.
// "linter.domains" maps a domain name to "none" | "recommended" | "all" and was
// introduced in Biome 2.0. The Biome 1.x equivalent was the "linter.rules.react"
// group with "recommended": true; it does not apply to the pinned binary.
//
// Verified against the pinned binary: with this config the default recommended
// rules keep firing (noExplicitAny probe) and the react rules fire even when no
// package.json declares react, while --config-path leaves all other defaults
// untouched.
const reactDomainLevel = "recommended"

// maxHeuristicFileBytes caps every file the detection heuristics read, so a
// pathological package.json or a generated megafile cannot stall planning.
const maxHeuristicFileBytes = 2 << 20

// importScanFileLimit bounds how many selected files the import fallback
// opens. Fifty matches keep the heuristic cheap even on huge workspaces.
const importScanFileLimit = 50

// importReactModules are module specifiers that only appear in React projects;
// next is included because Next.js apps always render React.
var importReactModules = []string{"react", "react-dom", "next"}

// ReactDetection explains whether and why React is in play for a workspace.
type ReactDetection struct {
	Detected bool
	Reason   string
}

// detectReact reports whether the workspace should lint with Biome's react
// domain. It never fails: unreadable or malformed manifests fall back to
// cheaper heuristics, because detection must not break a scan.
//
// Precedence:
//  1. a package.json at the workspace root, or one level down in the common
//     monorepo directories packages/* and apps/*, listing react or react-dom
//     in dependencies/devDependencies;
//  2. a selected .jsx/.tsx file importing react, react-dom, or next (first 50
//     matches only), which covers manifests that are missing, malformed, or
//     simply omit the dependency;
//  3. .jsx/.tsx files in the selection when no package.json exists at all.
func detectReact(root string, files []string) ReactDetection {
	dep, sawManifest := manifestReact(root)
	if dep.Detected {
		return dep
	}
	if d, ok := importedReact(files); ok {
		return d
	}
	if !sawManifest && hasJSXFile(files) {
		return ReactDetection{Detected: true, Reason: ".jsx/.tsx files selected without a package.json"}
	}
	return ReactDetection{}
}

// manifestReact looks for react/react-dom in dependencies or devDependencies
// of the workspace-root package.json plus one level of packages/* and apps/*.
// The second return reports whether any package.json existed, even a malformed
// one, so callers can distinguish "declared not-React" from "no manifest".
func manifestReact(root string) (ReactDetection, bool) {
	candidates := []string{filepath.Join(root, "package.json")}
	for _, pattern := range []string{
		filepath.Join(root, "packages", "*", "package.json"),
		filepath.Join(root, "apps", "*", "package.json"),
	} {
		if matches, err := filepath.Glob(pattern); err == nil {
			candidates = append(candidates, matches...)
		}
	}
	sawManifest := false
	for _, path := range candidates {
		data, ok := readBounded(path)
		if !ok {
			continue // absent or unreadable
		}
		sawManifest = true
		var manifest struct {
			Dependencies    map[string]string `json:"dependencies"`
			DevDependencies map[string]string `json:"devDependencies"`
		}
		if err := json.Unmarshal(data, &manifest); err != nil {
			continue // malformed: fall through to the import heuristic
		}
		for _, group := range []map[string]string{manifest.Dependencies, manifest.DevDependencies} {
			for _, name := range []string{"react", "react-dom"} {
				if version, ok := group[name]; ok {
					return ReactDetection{Detected: true, Reason: fmt.Sprintf("%s %s found in %s", name, version, manifestLabel(root, path))}, true
				}
			}
		}
	}
	return ReactDetection{}, sawManifest
}

// importedReact scans the .jsx/.tsx slice of the selection for React imports.
// Unreadable files are skipped, never fatal.
func importedReact(files []string) (ReactDetection, bool) {
	scanned := 0
	for _, file := range files {
		ext := strings.ToLower(filepath.Ext(file))
		if ext != ".jsx" && ext != ".tsx" {
			continue
		}
		scanned++
		if scanned > importScanFileLimit {
			return ReactDetection{}, false
		}
		if module := importsReactModule(file); module != "" {
			return ReactDetection{Detected: true, Reason: fmt.Sprintf("%s imported in %s", module, filepath.Base(file))}, true
		}
	}
	return ReactDetection{}, false
}

// importsReactModule reports the first React-ish module the file imports, or
// "". The patterns are deliberately literal: quoted specifier plus import,
// require, or from context, so comments merely mentioning react do not match.
func importsReactModule(path string) string {
	data, ok := readBounded(path)
	if !ok {
		return ""
	}
	source := string(data)
	for _, module := range importReactModules {
		for _, pattern := range []string{
			`from "` + module + `"`, `from '` + module + `'`,
			`require("` + module + `")`, `require('` + module + `')`,
			`import "` + module + `"`, `import '` + module + `'`,
			`from "` + module + `/`, `from '` + module + `/`,
			`require("` + module + `/`, `require('` + module + `/`,
		} {
			if strings.Contains(source, pattern) {
				return module
			}
		}
	}
	return ""
}

// hasJSXFile reports whether any selected file carries a JSX extension.
func hasJSXFile(files []string) bool {
	for _, file := range files {
		if ext := strings.ToLower(filepath.Ext(file)); ext == ".jsx" || ext == ".tsx" {
			return true
		}
	}
	return false
}

// readBounded reads a file if it exists and is small enough for a heuristic.
func readBounded(path string) ([]byte, bool) {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() || info.Size() > maxHeuristicFileBytes {
		return nil, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	return data, true
}

// manifestLabel renders a package.json path relative to the workspace root for
// log lines and metadata.
func manifestLabel(root, path string) string {
	if rel, err := filepath.Rel(root, path); err == nil {
		return filepath.ToSlash(rel)
	}
	return filepath.Base(path)
}

// hasWorkspaceBiomeConfig reports whether the workspace ships its own Biome
// configuration. Those files already decide which domains apply, and
// --config-path would disable their resolution, so Blunt Code must not inject
// its own config on top of them.
func hasWorkspaceBiomeConfig(root string) bool {
	for _, name := range []string{"biome.json", "biome.jsonc"} {
		if _, err := os.Stat(filepath.Join(root, name)); err == nil {
			return true
		}
	}
	return false
}

// injectedConfig is the exact biome.json Blunt Code writes for React
// workspaces. It states the linter defaults explicitly and adds only the
// react domain, so the pinned binary's default recommended rules keep running
// and non-React findings cannot shift.
type injectedConfig struct {
	Linter injectedLinter `json:"linter"`
}
type injectedLinter struct {
	Enabled bool              `json:"enabled"`
	Domains map[string]string `json:"domains"`
}

// reactDomainConfig renders the injected biome.json. The bytes are constant by
// design: every workspace gets the same config, which keeps the temp file name
// below stable and repeated scans sharing one file.
func reactDomainConfig() []byte {
	config := injectedConfig{Linter: injectedLinter{Enabled: true, Domains: map[string]string{"react": reactDomainLevel}}}
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		// Unreachable for these fixed values, but a marshal bug must not
		// silently disable the domain: fall back to the equivalent literal.
		return []byte(`{"linter":{"enabled":true,"domains":{"react":"` + reactDomainLevel + `"}}}`)
	}
	return append(data, '\n')
}

// writeInjectedConfig stores the rendered config under a content-hashed name
// in the system temp directory and returns its path. Identical content always
// maps to the same path, so scans share one file instead of piling configs up.
// Any write failure returns ok=false; the caller then falls back to today's
// config-less run rather than failing the scan.
func writeInjectedConfig() (string, bool) {
	data := reactDomainConfig()
	sum := sha256.Sum256(data)
	path := filepath.Join(os.TempDir(), fmt.Sprintf("bluntcode-biome-%s.json", hex.EncodeToString(sum[:8])))
	if existing, err := os.ReadFile(path); err == nil && bytes.Equal(existing, data) {
		return path, true
	}
	tmp, err := os.CreateTemp(os.TempDir(), "bluntcode-biome-*.json")
	if err != nil {
		return "", false
	}
	name := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(name)
		return "", false
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return "", false
	}
	// Rename is atomic for readers. If it fails (on Windows the target can be
	// held open by a concurrent biome), a plain overwrite is still correct
	// because the name encodes the content hash.
	if err := os.Rename(name, path); err != nil {
		if writeErr := os.WriteFile(path, data, 0o644); writeErr != nil {
			os.Remove(name)
			return "", false
		}
		os.Remove(name)
	}
	return path, true
}
