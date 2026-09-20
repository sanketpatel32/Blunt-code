package cleaner

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"bluntcode/internal/config"
	"bluntcode/internal/database"
)

// Options controls which storage targets to clean.
type Options struct {
	Logs   bool `json:"logs"`
	Cache  bool `json:"cache"`
	Vacuum bool `json:"vacuum"`
	All    bool `json:"all"`
}

// Result summarizes the disk space and items cleaned.
type Result struct {
	ReclaimedBytes int64    `json:"reclaimed_bytes"`
	LogsRemoved    int      `json:"logs_removed"`
	CacheCleared   bool     `json:"cache_cleared"`
	DBVacuumed     bool     `json:"db_vacuumed"`
	Details        []string `json:"details,omitempty"`
	Errors         []string `json:"errors,omitempty"`
}

// FormatBytes formats a byte size into human-readable notation (e.g. "1.3 GB", "42.5 MB").
func FormatBytes(b int64) string {
	if b < 1024 {
		return fmt.Sprintf("%d B", b)
	}
	const unit = 1024
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

// CleanTrivyCache removes Trivy's vulnerability database cache directory
// located under paths.DataDir/trivy-cache.
func CleanTrivyCache(paths config.Paths) (int64, error) {
	if paths.DataDir == "" {
		return 0, nil
	}
	cacheDir := filepath.Join(paths.DataDir, "trivy-cache")
	info, err := os.Stat(cacheDir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	if !info.IsDir() {
		return 0, nil
	}

	var size int64
	_ = filepath.WalkDir(cacheDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr == nil && d != nil && !d.IsDir() {
			if fi, fiErr := d.Info(); fiErr == nil {
				size += fi.Size()
			}
		}
		return nil
	})

	if err := os.RemoveAll(cacheDir); err != nil {
		// Attempt to clear read-only attributes on Windows and retry
		_ = filepath.WalkDir(cacheDir, func(p string, d fs.DirEntry, walkErr error) error {
			if walkErr == nil {
				_ = os.Chmod(p, 0o666)
			}
			return nil
		})
		if err = os.RemoveAll(cacheDir); err != nil {
			return 0, fmt.Errorf("remove trivy-cache: %w", err)
		}
	}
	return size, nil
}

// PruneLogs deletes log files under paths.LogsDir that have not been modified
// within maxAge (default: 7 days). Active process logs (bluntcode.log) are preserved.
// If an old log file is marked read-only on Windows, its attribute is cleared before removal.
func PruneLogs(paths config.Paths, maxAge time.Duration) (int64, int, error) {
	if paths.LogsDir == "" {
		return 0, 0, nil
	}
	entries, err := os.ReadDir(paths.LogsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, 0, nil
		}
		return 0, 0, err
	}

	cutoff := time.Now().Add(-maxAge)
	var reclaimed int64
	var count int

	for _, entry := range entries {
		if entry.IsDir() || entry.Name() == "bluntcode.log" {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			filePath := filepath.Join(paths.LogsDir, entry.Name())
			removeErr := os.Remove(filePath)
			if removeErr != nil {
				_ = os.Chmod(filePath, 0o666)
				removeErr = os.Remove(filePath)
			}
			if removeErr == nil {
				reclaimed += info.Size()
				count++
			}
		}
	}

	return reclaimed, count, nil
}

// VacuumDB defragments the SQLite database file and reclaims uncompacted pages.
func VacuumDB(ctx context.Context, paths config.Paths, db *database.DB) (int64, error) {
	if db == nil || paths.DBPath == "" {
		return 0, nil
	}
	var beforeSize int64
	if fi, err := os.Stat(paths.DBPath); err == nil {
		beforeSize += fi.Size()
	}
	walPath := paths.DBPath + "-wal"
	if fi, err := os.Stat(walPath); err == nil {
		beforeSize += fi.Size()
	}

	if err := db.Vacuum(ctx); err != nil {
		return 0, fmt.Errorf("sqlite vacuum: %w", err)
	}

	var afterSize int64
	if fi, err := os.Stat(paths.DBPath); err == nil {
		afterSize += fi.Size()
	}
	if fi, err := os.Stat(walPath); err == nil {
		afterSize += fi.Size()
	}

	var reclaimed int64
	if beforeSize > afterSize {
		reclaimed = beforeSize - afterSize
	}
	return reclaimed, nil
}

// Clean executes the selected storage cleaning tasks.
func Clean(ctx context.Context, paths config.Paths, db *database.DB, opts Options) (Result, error) {
	runAll := opts.All || (!opts.Logs && !opts.Cache && !opts.Vacuum)
	doLogs := runAll || opts.Logs
	doCache := runAll || opts.Cache
	doVacuum := runAll || opts.Vacuum

	var res Result

	if doCache {
		cacheBytes, err := CleanTrivyCache(paths)
		if err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("trivy-cache: %v", err))
		} else {
			res.CacheCleared = true
			res.ReclaimedBytes += cacheBytes
			if cacheBytes > 0 {
				res.Details = append(res.Details, fmt.Sprintf("Cleared Trivy cache (%s reclaimed)", FormatBytes(cacheBytes)))
			} else {
				res.Details = append(res.Details, "Trivy cache already clean")
			}
		}
	}

	if doLogs {
		logBytes, logCount, err := PruneLogs(paths, 7*24*time.Hour)
		if err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("logs: %v", err))
		} else {
			res.LogsRemoved = logCount
			res.ReclaimedBytes += logBytes
			if logCount > 0 {
				res.Details = append(res.Details, fmt.Sprintf("Pruned %d log file(s) older than 7 days (%s reclaimed)", logCount, FormatBytes(logBytes)))
			} else {
				res.Details = append(res.Details, "No log files older than 7 days to prune")
			}
		}
	}

	if doVacuum && db != nil {
		vacuumBytes, err := VacuumDB(ctx, paths, db)
		if err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("db vacuum: %v", err))
		} else {
			res.DBVacuumed = true
			res.ReclaimedBytes += vacuumBytes
			if vacuumBytes > 0 {
				res.Details = append(res.Details, fmt.Sprintf("Compacted database (%s reclaimed)", FormatBytes(vacuumBytes)))
			} else {
				res.Details = append(res.Details, "Compacted database (pages defragmented)")
			}
		}
	}

	if len(res.Errors) > 0 && res.ReclaimedBytes == 0 && !res.CacheCleared && !res.DBVacuumed && res.LogsRemoved == 0 {
		return res, fmt.Errorf("cleanup failed: %s", strings.Join(res.Errors, "; "))
	}

	return res, nil
}
