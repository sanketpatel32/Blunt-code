package scans

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"bluntcode/internal/events"
)

// TestShutdownWaitsForScanGoroutines pins that Shutdown does not return while
// a scan goroutine can still write to the database, and that it reports a
// timeout instead of pretending the scan stopped.
func TestShutdownWaitsForScanGoroutines(t *testing.T) {
	analyzer := &scriptedAnalyzer{id: "fake", runEntered: make(chan struct{}), runGate: make(chan struct{})}
	fixture := newScanFixture(t, analyzer)
	scan := fixture.startScan(t)
	<-analyzer.runEntered

	short, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := fixture.service.Shutdown(short); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Shutdown with a stuck scan = %v, want a deadline error", err)
	}
	if _, err := fixture.service.Start(context.Background(), fixture.work, "standard", nil); !errors.Is(err, ErrShuttingDown) {
		t.Fatalf("Start during shutdown = %v, want ErrShuttingDown", err)
	}

	close(analyzer.runGate)
	if err := fixture.service.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown after the scan finished: %v", err)
	}
	// Shutdown returned, so the goroutine is gone and its terminal state is
	// already persisted — no polling needed.
	current, err := fixture.db.Scan(context.Background(), scan.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.State != "cancelled" {
		t.Fatalf("scan state after shutdown = %q, want cancelled", current.State)
	}
}

// TestUnpersistableTerminalStateIsSurfaced closes the database under a running
// scan: every later write fails, and the terminal event must say so instead of
// reporting a clean outcome that never reached the store.
func TestUnpersistableTerminalStateIsSurfaced(t *testing.T) {
	analyzer := &scriptedAnalyzer{id: "fake", runEntered: make(chan struct{}), runGate: make(chan struct{})}
	fixture := newScanFixture(t, analyzer)
	scan := fixture.startScan(t)
	ch, unsubscribe := fixture.bus.Subscribe(scan.ID)
	defer unsubscribe()
	<-analyzer.runEntered
	if err := fixture.db.Close(); err != nil {
		t.Fatal(err)
	}
	close(analyzer.runGate)
	event := waitTerminal(t, ch)
	data, _ := event.Data.(map[string]any)
	if message, _ := data["error"].(string); message == "" {
		t.Fatalf("terminal event %s %#v does not surface the failed state write", event.Type, data)
	}
	if err := fixture.service.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
}

// TestRetentionDeletesPrunedReports pins that automatic retention removes the
// Markdown reports of pruned scans, not just their rows.
func TestRetentionDeletesPrunedReports(t *testing.T) {
	t.Setenv("BLUNTCODE_MAX_SCANS_PER_WORKSPACE", "1")
	fixture := newScanFixture(t, &scriptedAnalyzer{id: "fake"})
	first := fixture.startScan(t)
	fixture.waitForTerminal(t, first.ID)
	firstReport, err := fixture.db.ReportPath(context.Background(), first.ID)
	if err != nil || firstReport == "" {
		t.Fatalf("first scan has no report: %q %v", firstReport, err)
	}
	// Report names carry a one-second timestamp; make sure the second scan
	// writes its own file instead of overwriting the first one.
	time.Sleep(1100 * time.Millisecond)
	second := fixture.startScan(t)
	ch, unsubscribe := fixture.bus.Subscribe(second.ID)
	defer unsubscribe()
	waitTerminal(t, ch)
	if _, err := fixture.db.Scan(context.Background(), first.ID); err == nil {
		t.Fatal("retention kept the older scan")
	}
	if _, err := os.Stat(firstReport); !os.IsNotExist(err) {
		t.Fatalf("pruned scan's report %s is still on disk (%v)", filepath.Base(firstReport), err)
	}
	secondReport, err := fixture.db.ReportPath(context.Background(), second.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(secondReport); err != nil {
		t.Fatalf("kept scan's report is missing: %v", err)
	}
}

func waitTerminal(t *testing.T, ch <-chan events.Event) events.Event {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case event := <-ch:
			if event.Type == "scan.completed" || event.Type == "scan.cancelled" {
				return event
			}
		case <-deadline:
			t.Fatal("no terminal scan event was published")
			return events.Event{}
		}
	}
}
