package api

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"bluntcode/internal/workspace"
)

// apiWorkspaceRoot validates a folder path typed into the app and returns its
// normalized form, or an error whose text is safe to show the user as-is.
//
// The server's working directory means nothing to someone typing into a
// browser, so relative paths are rejected instead of being resolved against
// it ("getwd: no such file or directory" used to leak out of here). A leading
// "~" expands to the user's home folder, which is what people mean by it.
func apiWorkspaceRoot(raw string) (string, error) {
	path := strings.TrimSpace(raw)
	if len(path) >= 2 && (path[0] == '"' && path[len(path)-1] == '"' || path[0] == '\'' && path[len(path)-1] == '\'') {
		// Pasted from Explorer's "Copy as path", which wraps the path in quotes.
		path = strings.TrimSpace(path[1 : len(path)-1])
	}
	if path == "" {
		return "", userMessage("Enter the folder you want to scan.")
	}
	if path == "~" || strings.HasPrefix(path, "~/") || strings.HasPrefix(path, `~\`) {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", userMessage("Could not find your home folder; enter the full folder path instead.")
		}
		path = filepath.Join(home, path[1:])
	}
	if !filepath.IsAbs(path) {
		return "", userMessage("Use the full folder path, for example " + examplePath() + ".")
	}
	root, err := workspace.NormalizeRoot(path)
	if err != nil {
		return "", friendlyPathError(path, err)
	}
	if reason := sensitiveRoot(root); reason != "" {
		return "", userMessage(reason)
	}
	return root, nil
}

// sensitiveRoot refuses folders that are never a project but would turn the
// file tree and source previews into a read of the whole machine: a drive
// or filesystem root, the Windows directory, and the home folder itself
// (projects inside it are fine).
func sensitiveRoot(root string) string {
	clean := filepath.Clean(root)
	if filepath.Dir(clean) == clean {
		return "That is the root of a drive. Choose your project folder instead."
	}
	for _, name := range []string{"WINDIR", "SystemRoot"} {
		if dir := os.Getenv(name); dir != "" && within(clean, dir) {
			return "The Windows folder cannot be a workspace. Choose your project folder instead."
		}
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" && samePath(clean, home) {
		return "Your whole home folder cannot be a workspace. Choose a project folder inside it."
	}
	return ""
}

func samePath(a, b string) bool {
	a, b = filepath.Clean(a), filepath.Clean(b)
	if filepath.Separator == '\\' {
		return strings.EqualFold(a, b)
	}
	return a == b
}

func within(path, dir string) bool {
	if samePath(path, dir) {
		return true
	}
	rel, err := filepath.Rel(filepath.Clean(dir), filepath.Clean(path))
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// userMessage is an error whose text is a complete sentence written for the
// person using the app (capitalized and punctuated on purpose).
type userMessage string

func (m userMessage) Error() string { return string(m) }

func examplePath() string {
	if filepath.Separator == '\\' {
		return `C:\code\my-app`
	}
	return "/home/you/code/my-app"
}

// friendlyPathError maps the os/filepath errors NormalizeRoot can return to
// sentences; the raw "stat …: no such file or directory" text never reaches
// the client.
func friendlyPathError(path string, err error) error {
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return userMessage("That folder does not exist. Check the spelling, or use Browse to pick it.")
	case errors.Is(err, fs.ErrPermission):
		return userMessage("Blunt Code is not allowed to read that folder. Pick a folder your account can open.")
	case strings.Contains(err.Error(), "must be a directory"):
		return userMessage("That path is a file. Choose the folder that contains your project.")
	default:
		return userMessage("Could not open that folder. Check the path and try again.")
	}
}
