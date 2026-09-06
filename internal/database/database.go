package database

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

type DB struct{ SQL *sql.DB }

// Open owns the database file at path. Coordination and durability contract
// (IMP-11): process-level ownership is the instance package's machine-wide
// data-directory guard, so under normal operation exactly one process writes
// here; the connection still sets a 5-second busy timeout and enforces
// foreign keys, so an overlapping reader (read-only doctor against a live
// server) waits briefly instead of failing, and cascading deletes can never
// orphan findings, metrics, or hash rows. Migrations run inside one
// transaction each; a crash mid-migration rolls back cleanly, and result
// persistence (SaveAnalyzerResult) is transactional — a killed process leaves
// either fully committed results or a scan row the next start relabels
// "interrupted" (MarkInterruptedScans). Disk-full surfaces as ordinary SQLite
// write errors: scans fail honestly, never partially.
func Open(ctx context.Context, path string) (*DB, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	if err = db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}
	result := &DB{SQL: db}
	if err = result.Migrate(ctx); err != nil {
		db.Close()
		return nil, err
	}
	// One-time data heal for findings stored by releases with the SonarQube
	// project-key path bug; a no-op for healthy rows, so it rides on open.
	if _, err = result.RepairSonarqubeFindingPaths(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return result, nil
}

func (d *DB) Close() error { return d.SQL.Close() }

func (d *DB) Migrate(ctx context.Context) error {
	// This is migration bookkeeping bootstrap only; schema changes stay in numbered files.
	if _, err := d.SQL.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL)`); err != nil {
		return fmt.Errorf("initialize migration bookkeeping: %w", err)
	}
	entries, err := fs.ReadDir(migrationFiles, "migrations")
	if err != nil {
		return err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		var version int
		if _, err := fmt.Sscanf(entry.Name(), "%d_", &version); err != nil {
			return fmt.Errorf("invalid migration %s", entry.Name())
		}
		var applied int
		err := d.SQL.QueryRowContext(ctx, `SELECT 1 FROM schema_migrations WHERE version = ?`, version).Scan(&applied)
		if err == nil {
			continue
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		contents, err := migrationFiles.ReadFile("migrations/" + entry.Name())
		if err != nil {
			return err
		}
		tx, err := d.SQL.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, string(contents)); err == nil {
			_, err = tx.ExecContext(ctx, `INSERT INTO schema_migrations(version, applied_at) VALUES(?, ?)`, version, time.Now().UTC().Format(time.RFC3339Nano))
		}
		if err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("apply migration %s: %w", entry.Name(), err)
		}
		if err = tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}
