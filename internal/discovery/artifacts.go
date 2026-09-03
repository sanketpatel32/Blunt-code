package discovery

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Generated-artifact detection: the "smart skip" layer. Scanning a project
// means scanning what a person can edit and fix; build output, vendored
// dependencies, caches, logs, lockfiles, and minified bundles waste analyzer
// time and drown real findings in noise about files nobody maintains (found
// while dogfooding: a scan of this very repository read its own compiled web
// bundle, cmd/bluntcode/static/assets/index-*.js). Everything in this file
// answers one question — "is this path generated?" — from three angles:
//
//   - where it lives  (artifact directories such as dist, target, vendor)
//   - what it is called (hash-suffixed bundles, *.min.css, lockfiles, *.pb.go)
//   - what it contains (a 500+-character line inside a >128 KiB text file is
//     minified with overwhelming probability; hand-written source is never
//     packed like that)
//
// Two different consumers need two different flavors:
//
//   - DefaultExcluded (the walk filter) uses the full table INCLUDING
//     lockfiles, because no per-file code analyzer has business parsing a
//     multi-megabyte package-lock.json (its sha512 integrity strings even
//     trigger secret-detector false positives). Dependency analyzers such as
//     osv and trivy walk the workspace root themselves, so excluding lock
//     files from the candidate list never hides them from dependency
//     scanning.
//   - IsArtifactPath (the finding filter) deliberately does NOT treat a
//     lockfile as an artifact: osv and trivy report lockfile paths, and
//     those findings must survive.
//
// All patterns are intentionally conservative: a name that a human might
// plausibly give a hand-written file (bin/, packages/, service2.js,
// use-auth-hook.js) never matches, so the filter only fires where the file
// is generated beyond reasonable doubt.

// defaultDirectories lists directory basenames that only ever hold generated
// output, installed dependencies, caches, or logs. Matched case-
// insensitively against every directory level of a walked path, so any
// nested node_modules or target is pruned mid-walk. Deliberately absent:
// bin (hand-written shell scripts live there), packages (pnpm monorepos keep
// first-party source in it), and deps (ambiguous across ecosystems).
var defaultDirectories = map[string]struct{}{
	// Version control and editors (historical set).
	".git": {}, ".hg": {}, ".svn": {}, ".idea": {}, ".vscode": {},
	// JavaScript and friends: build output, framework caches, package stores.
	"node_modules": {}, "dist": {}, "build": {}, "out": {}, ".next": {}, ".nuxt": {},
	".output": {}, ".svelte-kit": {}, ".astro": {}, ".vite": {}, ".parcel-cache": {},
	".turbo": {}, ".angular": {}, "storybook-static": {}, ".docusaurus": {}, ".gatsby": {},
	"bower_components": {}, ".pnpm-store": {}, ".yarn": {}, ".npm": {},
	// Python: virtualenvs, caches, packaging.
	".venv": {}, "venv": {}, "env": {}, "__pycache__": {}, ".pytest_cache": {},
	".mypy_cache": {}, ".ruff_cache": {}, ".tox": {}, ".nox": {}, ".eggs": {},
	"site-packages": {}, ".hypothesis": {}, ".ipynb_checkpoints": {},
	// JVM, Rust, .NET, Flutter: build directories and compiler caches.
	"target": {}, "obj": {}, ".gradle": {}, ".dart_tool": {},
	// Vendored dependencies (composer, go mod vendor, bundler).
	"vendor": {},
	// Apple ecosystems.
	"pods": {}, "deriveddata": {},
	// Infrastructure tooling caches.
	".terraform": {}, ".terragrunt-cache": {},
	// Test and coverage reports (pure outputs).
	"coverage": {}, ".cache": {}, ".nyc_output": {}, "playwright-report": {}, "test-results": {},
	// Elixir/Erlang release builds.
	"_build": {},
	// Log directories: their *.txt members are otherwise scan candidates.
	"log": {}, "logs": {},
}

// artifactDirectorySuffixes matches directories whose basename ends with one
// of these (case-insensitively): Python egg-info directories carry a
// versioned name (foo-1.2.3.dist-info style) that no fixed set can cover.
var artifactDirectorySuffixes = []string{".egg-info"}

// lockFileNames are dependency lockfiles. They are excluded from the
// discovery candidate list (no analyzer should parse them line by line) but
// are NOT artifacts for finding purposes — dependency analyzers legitimately
// report vulnerabilities at these paths.
var lockFileNames = map[string]struct{}{
	"package-lock.json": {}, "npm-shrinkwrap.json": {}, "packages.lock.json": {},
	"pnpm-lock.yaml": {}, "yarn.lock": {}, "bun.lock": {}, "bun.lockb": {},
	"cargo.lock": {}, "composer.lock": {}, "gemfile.lock": {}, "poetry.lock": {},
	"pipfile.lock": {}, "pdm.lock": {}, "uv.lock": {}, "go.sum": {},
}

// generatedFileSuffixes are file-name suffixes (matched case-insensitively
// on the whole basename) that mark generated or minified output wherever the
// file sits — a bundle copied next to its sources is still a bundle.
var generatedFileSuffixes = []string{
	// Minified web output. .min.js/.map/.pyc/.pyo predate this table; the
	// rest close the gap the dogfood scan exposed.
	".min.js", ".min.mjs", ".min.cjs", ".min.jsx", ".min.css",
	".chunk.js", ".chunk.css", ".bundle.js", ".bundle.css",
	".map", ".pyc", ".pyo",
	// Build metadata: TypeScript's incremental state is a single-line
	// multi-megabyte JSON.
	".tsbuildinfo",
	// Protobuf and gRPC generated code.
	".pb.go", ".pb.cc", ".pb.h", "_pb2.py", "_pb2_grpc.py",
	// Generator output across ecosystems.
	".generated.ts", ".generated.tsx", ".generated.js", ".generated.jsx", ".generated.cs",
	".g.cs", ".designer.cs",
}

// bundleFilePrefixes and bundleFileNames catch webpack split-chunk output
// that carries no .min marker and no hash (vendors~main.js, vendor.js).
var (
	bundleFilePrefixes = []string{"vendors~", "vendor~"}
	bundleFileNames    = map[string]struct{}{"vendor.js": {}, "vendors.js": {}}
)

// chunkIDFile matches pure numeric chunk basenames (0.js, 12.chunk.js) —
// webpack's default chunk naming. No one hand-names a file like this.
var chunkIDFile = regexp.MustCompile(`^\d+(\.(chunk|chunks))*(\.(m?js|cjs|css))$`)

// hashedBundleFile matches bundler output named <stem>-<hash>.<ext> or
// <stem>.<hash>.<ext>: Vite and esbuild emit index-CKc0XBc7.js, webpack
// emits main.6f8f4f2a.js. Hashes come from base64url, so a hash can contain
// a hyphen itself (chunk-editor-C-LEM0mN.js) — when the final segment is
// not hash-like, the two trailing segments joined are tried too. The check
// runs on the ORIGINAL basename (case matters: see hashLikeSegment).
func hashedBundleFile(base string) bool {
	dot := strings.LastIndexByte(base, '.')
	if dot <= 0 {
		return false
	}
	switch strings.ToLower(base[dot:]) {
	case ".js", ".mjs", ".cjs", ".jsx", ".css":
	default:
		return false
	}
	stem := base[:dot]
	last := stem
	joined := ""
	hasSeparator := false
	for i := len(stem) - 1; i >= 0; i-- {
		if stem[i] == '-' || stem[i] == '_' || stem[i] == '.' {
			hasSeparator = true
			last = stem[i+1:]
			for j := i - 1; j >= 0; j-- {
				if stem[j] == '-' || stem[j] == '_' || stem[j] == '.' {
					joined = stem[j+1:]
					break
				}
			}
			break
		}
	}
	if !hasSeparator {
		// A bare basename that is nothing but a hash (6f8f4f2a.js, a
		// webpack chunk). The digit-less mixed-case shape never matches
		// here: DarkMode.js is a hand-written component.
		return hashLikeSegment(stem, false)
	}
	if hashLikeSegment(last, true) {
		return true
	}
	return joined != "" && hashLikeSegment(joined, true)
}

// hashLikeSegment decides whether a trailing name segment before the
// extension looks like a bundler hash (CKc0XBc7, 6f8f4f2a, BdSwwqVo,
// C-LEM0mN) rather than part of a hand-written name. Rules, tuned against
// real bundle names and plausible source names:
//
//   - at least 7 characters, drawn only from [A-Za-z0-9_$-] (base64url
//     hashes are 8 characters by default but vary with tool config);
//   - when the segment contains a digit, it matches if it also mixes case
//     or carries a second digit. Vite/esbuild hashes are mixed-case base64
//     and virtually always include an uppercase letter; webpack hex hashes
//     are lowercase but digit-dense. This keeps service2.js (one digit, no
//     upper case) a candidate;
//   - digit-less hashes exist (BdSwwqVo) but are indistinguishable from a
//     PascalCase word by content alone, so that shape only matches AFTER a
//     separator within a longer stem (chunk-charts-BdSwwqVo.js) — never as
//     the whole stem, where DarkMode.js is a hand-written component and
//     must stay a candidate.
//
// The afterSeparator argument carries that context: true when the segment
// is the tail of a longer stem, false when the whole stem is being tested.
func hashLikeSegment(segment string, afterSeparator bool) bool {
	if len(segment) < 7 {
		return false
	}
	digits, upper, lower := 0, 0, 0
	for i := 0; i < len(segment); i++ {
		c := segment[i]
		switch {
		case c >= '0' && c <= '9':
			digits++
		case c >= 'a' && c <= 'z':
			lower++
		case c >= 'A' && c <= 'Z':
			upper++
		case c == '$' || c == '_' || c == '-':
		default:
			return false
		}
	}
	if digits > 0 {
		return upper > 0 || digits >= 2
	}
	return afterSeparator && upper > 0 && lower > 0
}

// artifactDirectoryName reports whether a path component is an artifact
// directory basename.
func artifactDirectoryName(base string) bool {
	lower := strings.ToLower(base)
	if _, ok := defaultDirectories[lower]; ok {
		return true
	}
	for _, suffix := range artifactDirectorySuffixes {
		if strings.HasSuffix(lower, suffix) {
			return true
		}
	}
	return false
}

// artifactFileName reports whether a file basename marks generated output:
// lockfiles, generated-code suffixes, bundle names/prefixes, numeric chunk
// ids, and hash-suffixed bundler output.
func artifactFileName(base string) bool {
	lower := strings.ToLower(base)
	if _, ok := lockFileNames[lower]; ok {
		return true
	}
	for _, suffix := range generatedFileSuffixes {
		if strings.HasSuffix(lower, suffix) {
			return true
		}
	}
	for _, prefix := range bundleFilePrefixes {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	if _, ok := bundleFileNames[lower]; ok {
		return true
	}
	if chunkIDFile.MatchString(lower) {
		return true
	}
	return hashedBundleFile(base)
}

// IsArtifactPath reports whether a workspace-relative path points at
// generated output: any directory component is an artifact directory, or the
// basename is a generated file name. Lockfiles are explicitly NOT artifacts
// here — the scan pipeline uses this to drop noise findings from
// directory-walking analyzers, and dependency analyzers (osv, trivy) must
// keep their lockfile findings. Empty paths return false so findings without
// file attribution always survive.
func IsArtifactPath(rel string) bool {
	cleaned := filepath.ToSlash(filepath.Clean(rel))
	if cleaned == "" || cleaned == "." || cleaned == "/" {
		return false
	}
	for _, part := range strings.Split(cleaned, "/") {
		if artifactDirectoryName(part) {
			return true
		}
	}
	base := filepath.Base(cleaned)
	if _, ok := lockFileNames[strings.ToLower(base)]; ok {
		return false
	}
	return artifactFileName(base)
}

// Content and size heuristics. Name-based rules cannot catch an unminified-
// looking name (app.js) that contains a minified bundle, or a 200 MB data
// dump with a source-looking extension; these caps do.
const (
	// maxCandidateBytes skips any single candidate larger than this. A file
	// past 10 MiB is not hand-maintained source, and feeding it to per-file
	// analyzers risks both time and memory.
	maxCandidateBytes = 10 << 20
	// minifiedSniffThreshold is the size above which a text candidate is
	// worth a content sniff. Below 128 KiB even a bundle is cheap to scan,
	// and reading file heads during a walk should stay rare.
	minifiedSniffThreshold = 128 << 10
	// minifiedSniffWindow is how much of the file head the sniff reads.
	minifiedSniffWindow = 64 << 10
	// minifiedLineLength: any single line this long marks the file as
	// generated. Hand-written code essentially never packs 500 characters
	// onto one line; bundlers routinely emit lines with hundreds of
	// thousands.
	minifiedLineLength = 500
)

// minifiedSniffExtensions are the candidate extensions whose files get the
// content sniff. Markdown is deliberately absent: generated markdown
// reports exist, but long lines in hand-written docs are common enough that
// dropping them would cost real findings.
var minifiedSniffExtensions = map[string]struct{}{
	".js": {}, ".jsx": {}, ".mjs": {}, ".cjs": {}, ".ts": {}, ".tsx": {},
	".css": {}, ".scss": {}, ".less": {}, ".html": {}, ".json": {}, ".jsonc": {}, ".txt": {},
}

// LooksMinified reports whether any newline-delimited line in head is at
// least minifiedLineLength bytes long. It sees only the file's head; a
// minified file qualifies on its first line anyway.
func LooksMinified(head []byte) bool {
	for len(head) > 0 {
		var line []byte
		if index := bytes.IndexByte(head, '\n'); index >= 0 {
			line, head = head[:index], head[index+1:]
		} else {
			line, head = head, nil
		}
		if len(line) >= minifiedLineLength {
			return true
		}
	}
	return false
}

// skipGeneratedContent decides from size and content whether a regular file
// on disk is generated: oversized candidates are skipped outright, and large
// text files get a bounded head-sniff for minified lines. The path is the
// absolute file; read failures keep the file (inability to peek is not
// evidence of generation, and the analyzers will surface real read errors).
func skipGeneratedContent(path string, size int64) bool {
	if size > maxCandidateBytes {
		return true
	}
	if size < minifiedSniffThreshold {
		return false
	}
	if _, ok := minifiedSniffExtensions[strings.ToLower(filepath.Ext(path))]; !ok {
		return false
	}
	head, err := readPrefix(path, minifiedSniffWindow)
	if err != nil {
		return false
	}
	return LooksMinified(head)
}

// readPrefix reads at most limit bytes from the start of path. Short files
// return short data; the caller treats a read error as "no evidence".
func readPrefix(path string, limit int) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	buffer := make([]byte, limit)
	read := 0
	for read < len(buffer) {
		n, err := file.Read(buffer[read:])
		read += n
		if err != nil {
			break
		}
	}
	return buffer[:read], nil
}
