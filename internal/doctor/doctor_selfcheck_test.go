package doctor

import (
	"context"
	"strings"
	"testing"

	"bluntcode/internal/config"
	"bluntcode/internal/database"
)

// TestDatabaseSelfCheckSkipsWhenFileIsAbsent pins the fresh-install path: a
// missing database file is informational (never a failure and never hard),
// so doctor stays green before Blunt Code has ever been launched.
func TestDatabaseSelfCheckSkipsWhenFileIsAbsent(t *testing.T) {
	paths, err := config.NewPaths(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	check := databaseSelfCheck(context.Background(), paths)
	if check.Status != StatusSkip || check.Hard {
		t.Fatalf("absent file check = %#v, want skip/non-hard", check)
	}
	if !strings.Contains(strings.ToLower(check.Detail), "does not exist") {
		t.Fatalf("detail must explain the missing file: %q", check.Detail)
	}
}

// TestDatabaseSelfCheckPassesOnHealthyDatabase creates a real database
// through database.Open (migrations applied) and expects quick_check's ok.
func TestDatabaseSelfCheckPassesOnHealthyDatabase(t *testing.T) {
	ctx := context.Background()
	paths, err := config.NewPaths(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	db, err := database.Open(ctx, paths.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	check := databaseSelfCheck(ctx, paths)
	if check.Status != StatusOK || check.Hard {
		t.Fatalf("healthy database check = %#v, want ok/non-hard", check)
	}
	if strings.TrimSpace(check.Detail) != "ok" {
		t.Fatalf("quick_check must report ok, got %q", check.Detail)
	}

	// The check must also surface through the full Run flow without flipping
	// the overall result.
	result := Run(ctx, Options{Version: "test", Paths: paths})
	if selfCheck := find(result, "Database self-check"); selfCheck.Status != StatusOK {
		t.Fatalf("Run result check = %#v", selfCheck)
	}
	if !result.OK || result.ExitCode() != 0 {
		t.Fatalf("a healthy database must keep the run green: %#v", result)
	}
}

// TestTruncateCheckDetail caps long pragma messages while keeping short ones
// verbatim.
func TestTruncateCheckDetail(t *testing.T) {
	short := "ok"
	if got := truncateCheckDetail(short, databaseSelfCheckDetailLimit); got != short {
		t.Fatalf("short detail changed: %q", got)
	}
	long := strings.Repeat("x", databaseSelfCheckDetailLimit*3)
	got := truncateCheckDetail(long, databaseSelfCheckDetailLimit)
	if want := strings.Repeat("x", databaseSelfCheckDetailLimit) + "..."; got != want {
		t.Fatalf("truncated length = %d runes", len([]rune(got)))
	}
}
