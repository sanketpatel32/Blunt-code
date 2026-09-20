package cleaner

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"bluntcode/internal/config"
	"bluntcode/internal/database"
)

func TestCleanTrivyCache(t *testing.T) {
	dataDir := t.TempDir()
	paths := config.Paths{DataDir: dataDir}

	// When cache does not exist, CleanTrivyCache returns 0, nil
	reclaimed, err := CleanTrivyCache(paths)
	if err != nil || reclaimed != 0 {
		t.Fatalf("expected 0, nil for nonexistent cache, got %d, %v", reclaimed, err)
	}

	// Create fake trivy-cache
	cacheDir := filepath.Join(dataDir, "trivy-cache", "db")
	if err := os.MkdirAll(cacheDir, 0o700); err != nil {
		t.Fatal(err)
	}
	samplePayload := []byte("vulnerability-database-contents-123456789")
	if err := os.WriteFile(filepath.Join(cacheDir, "trivy.db"), samplePayload, 0o600); err != nil {
		t.Fatal(err)
	}

	// Create a read-only file inside trivy-cache
	roFile := filepath.Join(cacheDir, "readonly.db")
	if err := os.WriteFile(roFile, []byte("ro-content"), 0o400); err != nil {
		t.Fatal(err)
	}
	_ = os.Chmod(roFile, 0o400)

	reclaimed, err = CleanTrivyCache(paths)
	if err != nil {
		t.Fatalf("CleanTrivyCache failed: %v", err)
	}
	if reclaimed <= 0 {
		t.Fatalf("expected positive reclaimed bytes, got %d", reclaimed)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "trivy-cache")); !os.IsNotExist(err) {
		t.Fatal("expected trivy-cache directory to be completely removed")
	}
}

func TestPruneLogs(t *testing.T) {
	logsDir := t.TempDir()
	paths := config.Paths{LogsDir: logsDir}

	// Create 1 old log file (10 days old), 1 old read-only log, 1 active server log, and 1 new log file (current)
	oldLog := filepath.Join(logsDir, "scan-old.log")
	oldReadOnlyLog := filepath.Join(logsDir, "scan-old-ro.log")
	activeServerLog := filepath.Join(logsDir, "bluntcode.log")
	newLog := filepath.Join(logsDir, "scan-new.log")

	payload := []byte("log data 12345")
	for _, f := range []string{oldLog, oldReadOnlyLog, activeServerLog, newLog} {
		if err := os.WriteFile(f, payload, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	_ = os.Chmod(oldReadOnlyLog, 0o400)

	// Set modtimes to 10 days ago for old files and active server log
	oldTime := time.Now().Add(-10 * 24 * time.Hour)
	for _, f := range []string{oldLog, oldReadOnlyLog, activeServerLog} {
		if err := os.Chtimes(f, oldTime, oldTime); err != nil {
			t.Fatal(err)
		}
	}

	reclaimed, count, err := PruneLogs(paths, 7*24*time.Hour)
	if err != nil {
		t.Fatalf("PruneLogs failed: %v", err)
	}
	// Expected to prune oldLog and oldReadOnlyLog (count = 2); activeServerLog and newLog are kept.
	if count != 2 {
		t.Fatalf("expected 2 logs pruned, got %d", count)
	}
	if reclaimed != int64(len(payload)*2) {
		t.Fatalf("expected %d bytes reclaimed, got %d", len(payload)*2, reclaimed)
	}

	if _, err := os.Stat(oldLog); !os.IsNotExist(err) {
		t.Fatal("expected oldLog to be deleted")
	}
	if _, err := os.Stat(oldReadOnlyLog); !os.IsNotExist(err) {
		t.Fatal("expected oldReadOnlyLog to be deleted")
	}
	if _, err := os.Stat(activeServerLog); err != nil {
		t.Fatal("expected activeServerLog (bluntcode.log) to still exist")
	}
	if _, err := os.Stat(newLog); err != nil {
		t.Fatal("expected newLog to still exist")
	}
}

func TestVacuumDB(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")
	paths := config.Paths{DBPath: dbPath}

	db, err := database.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	reclaimed, err := VacuumDB(context.Background(), paths, db)
	if err != nil {
		t.Fatalf("VacuumDB failed: %v", err)
	}
	if reclaimed < 0 {
		t.Fatalf("unexpected negative reclaimed: %d", reclaimed)
	}
}

func TestCleanAll(t *testing.T) {
	dataDir := t.TempDir()
	logsDir := filepath.Join(dataDir, "logs")
	_ = os.MkdirAll(logsDir, 0o700)
	dbPath := filepath.Join(dataDir, "bluntcode.db")

	db, err := database.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	paths := config.Paths{
		DataDir: dataDir,
		LogsDir: logsDir,
		DBPath:  dbPath,
	}

	// Run clean with all
	res, err := Clean(context.Background(), paths, db, Options{All: true})
	if err != nil {
		t.Fatalf("Clean failed: %v", err)
	}
	if !res.CacheCleared || !res.DBVacuumed {
		t.Fatalf("expected CacheCleared and DBVacuumed to be true, got %#v", res)
	}
}

func TestCleanEmptyPaths(t *testing.T) {
	emptyPaths := config.Paths{}
	reclaimed, err := CleanTrivyCache(emptyPaths)
	if err != nil || reclaimed != 0 {
		t.Fatalf("expected (0, nil) for empty DataDir, got (%d, %v)", reclaimed, err)
	}

	reclaimed, err = VacuumDB(context.Background(), emptyPaths, nil)
	if err != nil || reclaimed != 0 {
		t.Fatalf("expected (0, nil) for nil DB, got (%d, %v)", reclaimed, err)
	}
}
