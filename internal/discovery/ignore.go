package discovery

import (
	"bytes"
	"errors"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// IgnoreFileName is the per-workspace ignore file that teams can commit so
// every scan (CLI or web UI, any machine) honors the same excludes without
// re-entering rules in the UI.
const IgnoreFileName = ".bluntcodeignore"

// Caps that keep a hostile or accidentally huge ignore file from hurting scan
// startup: the file is read only up to ignoreFileMaxBytes and at most
// ignoreFileMaxPatterns patterns are kept.
const (
	ignoreFileMaxBytes    = 64 << 10
	ignoreFileMaxPatterns = 1000
)

// parseIgnoreFile parses ignore-file content. The format is a small
// gitignore-inspired subset:
//
//   - one pattern per line, UTF-8, LF or CRLF line endings (CR is whitespace
//     and trimmed along with other leading/trailing space),
//   - blank lines and lines whose first non-space character is '#' are
//     ignored (comment; there is no '\#' escape),
//   - patterns use the same matching semantics as user exclude rules
//     (basename, "dir/**" prefix, and "**/name" suffix forms,
//     case-insensitive; see excludedByUser),
//   - negation ('!'-prefixed lines) is NOT supported: such lines are counted
//     in the negated return so callers can surface them, then skipped.
//
// Patterns beyond ignoreFileMaxPatterns are silently dropped. Content that is
// not valid UTF-8 yields no patterns at all: a broken ignore file must never
// fail a scan.
func parseIgnoreFile(data []byte) (patterns []string, negated int) {
	if !utf8.Valid(data) {
		return nil, 0
	}
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "!") {
			negated++
			continue
		}
		if len(patterns) >= ignoreFileMaxPatterns {
			break
		}
		patterns = append(patterns, filepath.ToSlash(line))
	}
	return patterns, negated
}

// loadIgnorePatterns reads and parses root's ignore file. A missing file is
// the normal case and returns nothing; any other failure (unreadable file,
// invalid UTF-8) is logged and swallowed so discovery always proceeds.
func loadIgnorePatterns(path string) (patterns []string, negated int) {
	data, err := os.ReadFile(path)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			log.Printf("discovery: ignoring unreadable %s: %v", IgnoreFileName, err)
		}
		return nil, 0
	}
	if len(data) > ignoreFileMaxBytes {
		truncated := data[:ignoreFileMaxBytes]
		// Drop a trailing partial line so a pattern cut in half by the byte
		// cap cannot accidentally match. A single line longer than the whole
		// cap is dropped entirely.
		if index := bytes.LastIndexByte(truncated, '\n'); index >= 0 {
			truncated = truncated[:index]
		} else {
			truncated = nil
		}
		log.Printf("discovery: %s exceeds %d bytes; everything after byte %d was skipped", IgnoreFileName, ignoreFileMaxBytes, len(truncated))
		data = truncated
	}
	patterns, negated = parseIgnoreFile(data)
	if negated > 0 {
		log.Printf("discovery: %s contained %d '!' pattern(s); negation is not supported and they were skipped", IgnoreFileName, negated)
	}
	return patterns, negated
}

// WorkspaceExcludes merges the patterns from the workspace root's
// .bluntcodeignore into userExcludes and returns the combined list. The
// file's patterns are purely additive excludes, exactly like DB exclude
// rules; they never cancel include rules. The result is deduplicated
// (case-insensitively, mirroring how the matcher normalizes patterns), so
// applying WorkspaceExcludes to an already-merged list is a no-op and callers
// such as the scan service can merge for snapshot purposes even though
// Discover and Tree merge again internally.
func WorkspaceExcludes(root string, userExcludes []string) []string {
	filePatterns, _ := loadIgnorePatterns(filepath.Join(root, IgnoreFileName))
	if len(filePatterns) == 0 {
		return append([]string(nil), userExcludes...)
	}
	merged := make([]string, 0, len(userExcludes)+len(filePatterns))
	seen := make(map[string]struct{}, len(userExcludes)+len(filePatterns))
	for _, pattern := range append(append([]string(nil), userExcludes...), filePatterns...) {
		key := strings.ToLower(filepath.ToSlash(strings.TrimSpace(pattern)))
		if key == "" {
			continue
		}
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		merged = append(merged, pattern)
	}
	return merged
}
