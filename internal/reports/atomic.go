package reports

import (
	"io"
	"os"
	"path/filepath"
)

// WriteFileAtomic writes a report artifact so a crash mid-write can never
// leave a truncated file that reads as a successful export (IMP-12): content
// lands in a temporary file beside the destination, is flushed to disk, and
// replaces the destination with a single rename. A failed or interrupted
// write leaves the previous file (or nothing) — never a partial artifact.
func WriteFileAtomic(path string, perm os.FileMode, write func(w io.Writer) error) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	commit := false
	defer func() {
		if !commit {
			_ = tmp.Close()
			_ = os.Remove(tmpName)
		}
	}()
	if err := write(tmp); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, perm); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	commit = true
	return nil
}

// WriteBytesAtomic is WriteFileAtomic for an in-memory document.
func WriteBytesAtomic(path string, data []byte, perm os.FileMode) error {
	return WriteFileAtomic(path, perm, func(w io.Writer) error {
		_, err := w.Write(data)
		return err
	})
}
