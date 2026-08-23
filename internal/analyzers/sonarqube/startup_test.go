package sonarqube

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// neverReadyServer starts fine but never reports healthy.
type neverReadyServer struct{ polls atomic.Int64 }

func (s *neverReadyServer) Start(context.Context, map[string]string) error { return nil }
func (s *neverReadyServer) Healthy(context.Context) (bool, error) {
	s.polls.Add(1)
	return false, nil
}
func (s *neverReadyServer) Shutdown(context.Context) error { return nil }

func statusServer(t *testing.T, polls *atomic.Int64, failures int64, final string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if polls.Add(1) <= failures {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"` + final + `"}`))
	}))
}

func TestStartupTimeoutFromEnv(t *testing.T) {
	cases := []struct {
		name, value string
		want        time.Duration
	}{
		{"empty means no override", "", 0},
		{"seconds", "45s", 45 * time.Second},
		{"minutes", "5m", 5 * time.Minute},
		{"compound", "1m30s", 90 * time.Second},
		{"unparseable ignored", "fast", 0},
		{"missing unit ignored", "45", 0},
		{"zero ignored", "0s", 0},
		{"negative ignored", "-45s", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var warned bytes.Buffer
			logger := slog.New(slog.NewTextHandler(&warned, nil))
			if got := startupTimeoutFromEnv(func(string) string { return tc.value }, logger); got != tc.want {
				t.Fatalf("startupTimeoutFromEnv(%q) = %s, want %s", tc.value, got, tc.want)
			}
			if tc.want > 0 && warned.Len() > 0 {
				t.Fatalf("valid value %q must not warn: %q", tc.value, warned.String())
			}
			if tc.want == 0 && tc.value != "" && !strings.Contains(warned.String(), startupTimeoutEnvVar) {
				t.Fatalf("expected a warning for %q, got %q", tc.value, warned.String())
			}
		})
	}
}

func TestStartupBudgetResolution(t *testing.T) {
	t.Setenv(startupTimeoutEnvVar, "")
	if got := (&Adapter{}).startupBudget(); got != fallbackStartupTimeout {
		t.Fatalf("bare adapter budget = %s, want the %s fallback", got, fallbackStartupTimeout)
	}
	adapter := New(nil, nil, nil, nil)
	if got := adapter.startupBudget(); got != defaultStartupTimeout {
		t.Fatalf("New adapter budget = %s, want the %s default", got, defaultStartupTimeout)
	}
	t.Setenv(startupTimeoutEnvVar, "45s")
	if got := adapter.startupBudget(); got != 45*time.Second {
		t.Fatalf("env override budget = %s, want 45s", got)
	}
}

func TestWaitForHealthyReturnsImmediatelyOnFirstPoll(t *testing.T) {
	var polls atomic.Int64
	server := statusServer(t, &polls, 0, "UP")
	defer server.Close()
	managed := &ManagedServer{URL: server.URL}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	start := time.Now()
	if err := waitForHealthy(ctx, managed, time.Minute, 10*time.Millisecond, discardLogger()); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("healthy-on-first-poll wait took %s; it must return without sleeping", elapsed)
	}
	if n := polls.Load(); n != 1 {
		t.Fatalf("expected exactly one health poll, got %d", n)
	}
}

func TestWaitForHealthyUsesShortIntervalAfterFailures(t *testing.T) {
	const failures = 3
	var polls atomic.Int64
	server := statusServer(t, &polls, failures, "UP")
	defer server.Close()
	managed := &ManagedServer{URL: server.URL}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	start := time.Now()
	if err := waitForHealthy(ctx, managed, 10*time.Second, 25*time.Millisecond, discardLogger()); err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(start)
	if n := polls.Load(); n != failures+1 {
		t.Fatalf("expected %d polls, got %d", failures+1, n)
	}
	// Three 25ms sleeps must finish far below any coarse one-second cadence.
	if elapsed > time.Second {
		t.Fatalf("short poll interval was not honored: %s", elapsed)
	}
}

func TestWaitForHealthyTimesOutPromptlyWithSmallBudget(t *testing.T) {
	var polls atomic.Int64
	server := statusServer(t, &polls, 1<<30, "UP")
	defer server.Close()
	managed := &ManagedServer{URL: server.URL}
	start := time.Now()
	err := waitForHealthy(context.Background(), managed, 400*time.Millisecond, 40*time.Millisecond, discardLogger())
	if err == nil || !strings.Contains(err.Error(), "did not become healthy within 400ms") {
		t.Fatalf("unexpected timeout error: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("expired budget returned after %s; it must fail promptly", elapsed)
	}
}

func TestWaitForHealthyAbortsPromptlyOnContextCancellation(t *testing.T) {
	var polls atomic.Int64
	server := statusServer(t, &polls, 1<<30, "UP")
	defer server.Close()
	managed := &ManagedServer{URL: server.URL}
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(120*time.Millisecond, cancel)
	start := time.Now()
	err := waitForHealthy(ctx, managed, time.Minute, 30*time.Millisecond, discardLogger())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("cancellation returned after %s; it must abort promptly", elapsed)
	}
}

func TestWaitForHealthyLogsProgressAndRecovery(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	var polls atomic.Int64
	server := statusServer(t, &polls, 2, "UP")
	defer server.Close()
	managed := &ManagedServer{URL: server.URL}
	if err := waitForHealthy(context.Background(), managed, 5*time.Second, 25*time.Millisecond, logger); err != nil {
		t.Fatal(err)
	}
	out := logs.String()
	if !strings.Contains(out, "waiting for managed SonarQube") || !strings.Contains(out, "starting") {
		t.Fatalf("expected a waiting progress log with the observed status: %q", out)
	}
	if !strings.Contains(out, "remaining") || !strings.Contains(out, "elapsed") {
		t.Fatalf("progress log must include elapsed and remaining budget: %q", out)
	}
	if !strings.Contains(out, "managed SonarQube is healthy") {
		t.Fatalf("expected a healthy log after waiting: %q", out)
	}
}

func TestEnsureRunningEnvTimeoutOverridesBudget(t *testing.T) {
	t.Setenv(startupTimeoutEnvVar, "400ms")
	server := &neverReadyServer{}
	adapter := &Adapter{Server: server, Client: startupClient{}, Runtime: ManagedRuntime{JavaHome: "configured"}, StartupTimeout: time.Hour}
	start := time.Now()
	err := adapter.EnsureRunning(context.Background())
	if err == nil || !strings.Contains(err.Error(), "did not become healthy within 400ms") {
		t.Fatalf("expected the env-configured 400ms budget to expire, got %v", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("startup wait took %s; the env override must bound the budget", elapsed)
	}
	if n := server.polls.Load(); n < 2 {
		t.Fatalf("expected repeated polling inside the budget, got %d polls", n)
	}
}
