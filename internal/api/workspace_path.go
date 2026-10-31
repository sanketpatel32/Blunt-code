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
	return root, nil
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
