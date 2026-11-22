// Package testpath lets tests keep Windows-style fixture paths while running
// on any OS.
package testpath

import (
	"runtime"
	"strings"
)

// Native returns a Windows-style absolute fixture path such as `C:\ws\main.py`
// unchanged on Windows and as the equivalent rooted slash path (`/ws/main.py`)
// elsewhere, so path/filepath treats it as absolute on the host OS.
func Native(windowsPath string) string {
	if runtime.GOOS == "windows" {
		return windowsPath
	}
	p := windowsPath
	if len(p) >= 2 && p[1] == ':' {
		p = p[2:]
	}
	parts := strings.FieldsFunc(p, func(r rune) bool { return r == '\\' || r == '/' })
	return "/" + strings.Join(parts, "/")
}
