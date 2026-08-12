//go:build windows

// Package instance keeps one Blunt Code backend attached to a data directory.
package instance

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

var ErrAlreadyRunning = errors.New("Blunt Code is already running for this data directory")

// Guard owns a per-user-session named mutex until Close is called. The mutex
// name is derived from the data directory, so independent portable installs do
// not block one another and the directory itself is not exposed system-wide.
type Guard struct{ handle windows.Handle }

func Acquire(dataDir string) (*Guard, error) {
	name := mutexName(dataDir)
	nameUTF16, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return nil, fmt.Errorf("encode single-instance mutex name: %w", err)
	}
	handle, err := windows.CreateMutex(nil, false, nameUTF16)
	if err != nil {
		if errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
			_ = windows.CloseHandle(handle)
			return nil, ErrAlreadyRunning
		}
		return nil, fmt.Errorf("create single-instance mutex: %w", err)
	}
	return &Guard{handle: handle}, nil
}

func (g *Guard) Close() error {
	if g == nil || g.handle == 0 {
		return nil
	}
	err := windows.CloseHandle(g.handle)
	g.handle = 0
	return err
}

func mutexName(dataDir string) string {
	canonical, err := filepath.Abs(dataDir)
	if err != nil {
		canonical = filepath.Clean(dataDir)
	}
	digest := sha256.Sum256([]byte(strings.ToLower(filepath.Clean(canonical))))
	return "Local\\BluntCode-" + hex.EncodeToString(digest[:16])
}
