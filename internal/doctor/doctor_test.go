package doctor

import (
	"context"
	"strings"
	"testing"

	"bluntcode/internal/config"
	"bluntcode/internal/tools"
)

func TestRunChecksLocalFoundationsAndReportsMissingToolsAsWarnings(t *testing.T) {
	paths, err := config.NewPaths(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	service := tools.NewService(paths.ToolsDir, tools.Manifest{}, true)
	result := Run(context.Background(), Options{Version: "test", Paths: paths, Tools: service})
	if !result.OK || result.ExitCode() != 0 {
		t.Fatalf("result = %#v", result)
	}
	if got := find(result, "Database and migrations"); got.Status != StatusOK {
		t.Fatalf("database = %#v", got)
	}
	if got := find(result, "Ruff"); got.Status != StatusWarn {
		t.Fatalf("ruff = %#v", got)
	}
	if got := find(result, "Disk space"); got.Status != StatusOK && got.Status != StatusWarn {
		t.Fatalf("disk space = %#v", got)
	}
	var output strings.Builder
	result.WriteHuman(&output)
	if !strings.Contains(output.String(), "Blunt Code: OK") || !strings.Contains(output.String(), "SonarQube startup: SKIP") {
		t.Fatalf("human output = %q", output.String())
	}
}

func TestRunFailsWhenDataDirectoryIsNotConfigured(t *testing.T) {
	result := Run(context.Background(), Options{Version: "test"})
	if result.OK || result.ExitCode() != 1 {
		t.Fatalf("result = %#v", result)
	}
	if got := find(result, "Data directory"); got.Status != StatusFail || !got.Hard {
		t.Fatalf("data directory = %#v", got)
	}
}

func find(result Result, name string) Check {
	for _, check := range result.Checks {
		if check.Name == name {
			return check
		}
	}
	return Check{}
}
