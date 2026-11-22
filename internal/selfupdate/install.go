package selfupdate

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"
)

// ApplyOptions drives the swap step, which runs in a copy of the installed
// binary after the app that staged the update has exited.
type ApplyOptions struct {
	PayloadDir  string
	InstallDir  string
	WaitPID     int
	WaitTimeout time.Duration
	Relaunch    bool
	Log         io.Writer
}

// swapAttempts covers file handles that linger briefly after the old process
// exits (antivirus scans, Explorer thumbnails).
var swapAttempts = 10

var swapRetryDelay = time.Second

// Apply waits for the old process, swaps the verified payload into the
// install directory (restoring the previous copy on failure), and relaunches
// whichever version is installed afterwards.
func Apply(options ApplyOptions) error {
	logf := func(format string, args ...any) {
		if options.Log != nil {
			fmt.Fprintf(options.Log, time.Now().Format(time.RFC3339)+" "+format+"\n", args...)
		}
	}
	if options.WaitPID > 0 {
		logf("waiting for process %d to exit", options.WaitPID)
		if err := waitForExit(options.WaitPID, options.WaitTimeout); err != nil {
			logf("wait failed: %v", err)
			return err
		}
	}
	var err error
	for attempt := 1; attempt <= swapAttempts; attempt++ {
		if err = Install(options.PayloadDir, options.InstallDir); err == nil {
			break
		}
		logf("install attempt %d failed: %v", attempt, err)
		if attempt < swapAttempts {
			// Back off instead of a fixed delay — a held file lock needs a
			// growing window, not the same wrong guess three times.
			time.Sleep(swapRetryDelay * time.Duration(attempt))
		}
	}
	if err == nil {
		logf("installed %s", options.InstallDir)
	}
	if options.Relaunch {
		executable := filepath.Join(options.InstallDir, executableName)
		if _, statErr := os.Stat(executable); statErr == nil {
			command := exec.Command(executable)
			command.Dir = options.InstallDir
			if startErr := StartDetached(command); startErr != nil {
				logf("relaunch failed: %v", startErr)
			}
		}
	}
	return err
}

// Install replaces installDir with payloadDir. The previous installation is
// moved aside first and restored if the new one cannot be put in place, so an
// interrupted update never leaves the user without a working copy.
func Install(payloadDir, installDir string) error {
	if _, err := os.Stat(filepath.Join(payloadDir, executableName)); err != nil {
		return fmt.Errorf("staged payload is missing %s: %w", executableName, err)
	}
	parent := filepath.Dir(installDir)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return err
	}
	backup := filepath.Join(parent, ".bluntcode-backup-"+strconv.FormatInt(time.Now().UnixNano(), 36))
	hadPrevious := false
	if _, err := os.Stat(installDir); err == nil {
		if err := os.Rename(installDir, backup); err != nil {
			return fmt.Errorf("move current installation aside: %w", err)
		}
		hadPrevious = true
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := movePayload(payloadDir, installDir); err != nil {
		if hadPrevious {
			_ = os.RemoveAll(installDir)
			if restoreErr := os.Rename(backup, installDir); restoreErr != nil {
				return fmt.Errorf("install failed (%v) and the previous copy could not be restored from %s: %w", err, backup, restoreErr)
			}
		}
		return fmt.Errorf("install new version: %w", err)
	}
	if hadPrevious {
		_ = os.RemoveAll(backup)
	}
	return nil
}

// movePayload renames when possible and copies when the staging folder lives
// on a different volume than the install directory.
func movePayload(source, target string) error {
	if err := os.Rename(source, target); err == nil {
		return nil
	}
	if err := copyTree(source, target); err != nil {
		return err
	}
	_ = os.RemoveAll(source)
	return nil
}

func copyTree(source, target string) error {
	return filepath.WalkDir(source, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		destination := filepath.Join(target, relative)
		if entry.IsDir() {
			return os.MkdirAll(destination, 0o755)
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("staged payload entry %s is not a regular file", relative)
		}
		return copyFile(path, destination)
	})
}

// CopyFile copies one regular file, creating or truncating the target.
func CopyFile(source, target string) error { return copyFile(source, target) }

func copyFile(source, target string) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
