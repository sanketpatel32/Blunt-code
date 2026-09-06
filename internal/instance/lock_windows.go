//go:build windows

// Package instance keeps one Blunt Code backend attached to a data directory.
//
// Ownership contract: one backend per data directory per machine — the mutex
// lives in the Global namespace, so simultaneous starts from different Windows
// sessions (fast user switching), different users, or a server session cannot
// each claim the same user-local database; the loser gets ErrAlreadyRunning
// instead of becoming an ambiguous second owner. The name is derived from the
// canonical absolute data-directory path (case-folded), so independent
// portable installs with distinct directories never block one another. What
// the lock coordinates is runtime ownership only: it holds no state on disk,
// and leftover application metadata (stale staging, backups) is the installer
// sweep's business, not the lock's.
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

// Guard owns the per-data-directory named mutex until Close is called.
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

// mutexName derives a machine-wide mutex name from the canonical data
// directory. Global\ (not Local\) is what makes the guard visible across
// Windows sessions and users; the hash keeps the directory path itself out of
// the machine-wide namespace.
func mutexName(dataDir string) string {
	canonical, err := filepath.Abs(dataDir)
	if err != nil {
		canonical = filepath.Clean(dataDir)
	}
	digest := sha256.Sum256([]byte(strings.ToLower(filepath.Clean(canonical))))
	return "Global\\BluntCode-" + hex.EncodeToString(digest[:16])
}
