//go:build !windows

package sonarqube

import (
	"context"
	"log/slog"
	"os"
)

// Managed SonarQube exists only on Windows (NewManaged rejects every other
// platform), so the stray sweep and kill-on-close tracking degrade to no-ops
// here purely to keep the package compiling elsewhere.
func sweepStrayServerProcesses(context.Context, string, *slog.Logger) error { return nil }
func trackProcessInKillOnCloseJob(*os.Process) error                        { return nil }
