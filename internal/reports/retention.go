package reports

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// RemoveFiles deletes the Markdown reports of scans that were deleted from
// the database. Only files directly inside reportsDir are removed: report
// paths come from database rows, and retention must never become a way to
// delete arbitrary files. A report that is already gone is not an error.
// Every path is attempted; the failures are joined into the returned error.
func RemoveFiles(reportsDir string, paths []string) error {
	var errs []error
	for _, path := range paths {
		if err := removeFile(reportsDir, path); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func removeFile(reportsDir, path string) error {
	if path == "" {
		return nil
	}
	dir, err := filepath.Abs(reportsDir)
	if err != nil {
		return fmt.Errorf("resolve reports directory: %w", err)
	}
	file, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolve report %s: %w", path, err)
	}
	if !samePath(filepath.Dir(file), dir) {
		return fmt.Errorf("report %s is outside the reports directory %s; not removing it", path, reportsDir)
	}
	if err := os.Remove(file); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove report: %w", err)
	}
	return nil
}

func samePath(a, b string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}
