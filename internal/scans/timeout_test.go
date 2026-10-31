package scans

import (
	"context"
	"strings"
	"testing"
	"time"

	"bluntcode/internal/analyzers"
)

// hangingAnalyzer blocks in Run until its context ends and then returns a
// nil error, like a tool whose kill shows up as an ordinary exit. Normalize
// would happily turn that fragment into a clean, finding-free run.
type hangingAnalyzer struct{ scriptedAnalyzer }

func (a *hangingAnalyzer) Run(ctx context.Context, _ analyzers.AnalyzerPlan, _ analyzers.EventEmitter) (analyzers.AnalyzerResult, error) {
	<-ctx.Done()
	return analyzers.AnalyzerResult{}, nil
}

func TestAnalyzerDeadlineFailsTheRunInsteadOfNormalizingPartialOutput(t *testing.T) {
	previous := fastAnalyzerTimeout
	fastAnalyzerTimeout = 150 * time.Millisecond
	t.Cleanup(func() { fastAnalyzerTimeout = previous })

	normalized := make(chan struct{})
	adapter := &hangingAnalyzer{scriptedAnalyzer{id: "hang", normalizeEntered: normalized}}
	fixture := newScanFixture(t, adapter)
	scan := fixture.waitForTerminal(t, fixture.startScan(t).ID)

	select {
	case <-normalized:
		t.Fatal("Normalize ran on the output of a run that hit its deadline")
	default:
	}
	runs, err := fixture.db.AnalyzerRuns(context.Background(), scan.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].State != "failed" || !strings.Contains(runs[0].ErrorSummary, "timed out after 150ms") {
		t.Fatalf("runs = %+v, want one failed run that says it timed out", runs)
	}
}
