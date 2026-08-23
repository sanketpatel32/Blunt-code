package sonarqube

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"
)

// The startup wait is configurable and adaptive: a warm server answers the
// first poll and returns immediately, while a cold community-server boot gets
// a generous default budget, short polls, and periodic progress logs so the
// wait never looks hung.
const (
	// startupTimeoutEnvVar overrides the managed-server startup budget with a
	// Go duration string, for example "45s", "5m", or "90s".
	startupTimeoutEnvVar = "BLUNTCODE_SONAR_STARTUP_TIMEOUT"
	// defaultStartupTimeout is what New configures: a cold community-server
	// boot (JVM plus embedded search index) routinely takes more than three
	// minutes on consumer Windows hardware.
	defaultStartupTimeout = 10 * time.Minute
	// fallbackStartupTimeout keeps the pre-existing safety net for adapters
	// built as bare struct literals without New.
	fallbackStartupTimeout = 3 * time.Minute
	// startupPollInterval keeps the health cadence short so a server that
	// finishes booting is noticed almost immediately.
	startupPollInterval = time.Second
	// startupProgressInterval spaces the "still waiting" logs so a long cold
	// boot stays transparent without flooding the log.
	startupProgressInterval = 20 * time.Second
)

// startupTimeoutFromEnv parses BLUNTCODE_SONAR_STARTUP_TIMEOUT. It returns 0
// when the variable is unset or invalid (unparseable or <= 0) after logging a
// warning, so callers keep their built-in default instead of failing.
func startupTimeoutFromEnv(getenv func(string) string, logger *slog.Logger) time.Duration {
	raw := strings.TrimSpace(getenv(startupTimeoutEnvVar))
	if raw == "" {
		return 0
	}
	value, err := time.ParseDuration(raw)
	if err != nil || value <= 0 {
		if logger == nil {
			logger = slog.Default()
		}
		logger.Warn("ignoring invalid SonarQube startup timeout", "env", startupTimeoutEnvVar, "value", raw, "hint", "use a Go duration such as 45s, 5m, or 90s")
		return 0
	}
	return value
}

// startupBudget resolves the effective startup wait: the environment override
// wins, then the adapter's configured StartupTimeout, then the fallback.
func (a *Adapter) startupBudget() time.Duration {
	if override := startupTimeoutFromEnv(os.Getenv, nil); override > 0 {
		return override
	}
	if a.StartupTimeout > 0 {
		return a.StartupTimeout
	}
	return fallbackStartupTimeout
}

// waitForHealthy polls the managed server until its health endpoint reports
// UP, the budget expires, the scan context is cancelled, or the server
// process exits. The first poll runs immediately, so a server that is already
// healthy returns without any fixed sleep. Progress is logged on status
// transitions and every startupProgressInterval so a long cold boot is
// visibly alive rather than silently hung.
func waitForHealthy(ctx context.Context, server Server, budget, interval time.Duration, logger *slog.Logger) error {
	if logger == nil {
		logger = slog.Default()
	}
	if interval <= 0 {
		interval = startupPollInterval
	}
	start := time.Now()
	deadline := start.Add(budget)
	lastState := ""
	nextProgress := start
	polls := 0
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		// A dead process must fail the scan immediately instead of burning the
		// whole budget against a refused connection.
		if exitErr, exited := serverExitStatus(server); exited {
			if exitErr == nil {
				return fmt.Errorf("managed SonarQube stopped during startup")
			}
			return fmt.Errorf("managed SonarQube stopped during startup: %w", exitErr)
		}
		healthy, err := server.Healthy(ctx)
		polls++
		if err == nil && healthy {
			if polls > 1 {
				logger.Info("managed SonarQube is healthy", "elapsed", time.Since(start).Round(time.Millisecond), "polls", polls)
			}
			return nil
		}
		state := healthState(err)
		now := time.Now()
		if state != lastState || !now.Before(nextProgress) {
			remaining := deadline.Sub(now)
			if remaining < 0 {
				remaining = 0
			}
			logger.Info("waiting for managed SonarQube", "elapsed", now.Sub(start).Round(time.Millisecond), "remaining", remaining.Round(time.Millisecond), "status", state)
			lastState = state
			nextProgress = now.Add(startupProgressInterval)
		}
		if !now.Before(deadline) {
			return fmt.Errorf("managed SonarQube did not become healthy within %s (last status: %s)", budget, state)
		}
		delay := interval
		if remain := time.Until(deadline); remain < delay {
			delay = remain
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

// serverExitStatus reports an unexpected child exit without consuming the
// process wait result. It lets startup fail immediately instead of waiting
// for the whole budget when SonarQube cannot bind or clean its runtime
// directory.
func serverExitStatus(server Server) (error, bool) {
	if monitor, ok := server.(interface{ ExitStatus() (error, bool) }); ok {
		return monitor.ExitStatus()
	}
	return nil, false
}

// healthState classifies the latest poll for progress logging: a transport
// error means the listener is not accepting connections yet, anything else is
// SonarQube answering but still initializing.
func healthState(err error) string {
	if err != nil {
		return "unreachable"
	}
	return "starting"
}
