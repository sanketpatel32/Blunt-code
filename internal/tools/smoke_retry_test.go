package tools

import (
	"context"
	"errors"
	"testing"
)

// A fresh executable can fail its first probe for environmental reasons —
// a virus scanner holding the new file, or rename visibility lagging. The
// probe gets one retry before the failure counts against the install.
func TestSmokeProbeRetriesOnceBeforeFailing(t *testing.T) {
	calls := 0
	manager := Manager{SmokeRunner: func(context.Context, string, []string) (string, error) {
		calls++
		if calls == 1 {
			return "", errors.New("transient access-denied")
		}
		return "1.2.3", nil
	}}
	err := manager.smokeProbe(context.Background(), "tool.exe", &SmokeTest{Args: []string{"--version"}, Expect: "1.2.3"})
	if err != nil {
		t.Fatalf("retryable failure must recover, got %v", err)
	}
	if calls != 2 {
		t.Fatalf("expected 2 probe attempts, got %d", calls)
	}
}

func TestSmokeProbeGivesUpAfterOneRetry(t *testing.T) {
	calls := 0
	manager := Manager{SmokeRunner: func(context.Context, string, []string) (string, error) {
		calls++
		return "", errors.New("broken binary")
	}}
	err := manager.smokeProbe(context.Background(), "tool.exe", &SmokeTest{Args: []string{"--version"}})
	if err == nil {
		t.Fatal("persistent failure must still fail")
	}
	if calls != 2 {
		t.Fatalf("expected exactly 2 probe attempts (1 + 1 retry), got %d", calls)
	}
}

func TestSmokeProbeSuccessSkipsRetry(t *testing.T) {
	calls := 0
	manager := Manager{SmokeRunner: func(context.Context, string, []string) (string, error) {
		calls++
		return "ok", nil
	}}
	if err := manager.smokeProbe(context.Background(), "tool.exe", &SmokeTest{Args: []string{"--version"}}); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("passing probe must not retry, got %d attempts", calls)
	}
}

func TestSmokeProbeHonoursCancellationDuringBackoff(t *testing.T) {
	manager := Manager{SmokeRunner: func(context.Context, string, []string) (string, error) {
		return "", errors.New("transient")
	}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already dead: the retry wait must not sit out the delay
	err := manager.smokeProbe(ctx, "tool.exe", &SmokeTest{Args: []string{"--version"}})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled context must abort the retry wait, got %v", err)
	}
}
