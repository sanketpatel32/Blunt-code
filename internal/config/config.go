package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const AppName = "BluntCode"

// DefaultMaxScansPerWorkspace is the default retention policy limit: keep at most
// 20 terminal scans per workspace to prevent indefinite SQLite database growth.
const DefaultMaxScansPerWorkspace = 20

// MaxScansPerWorkspace returns the configured retention limit, or 0 if auto-pruning is disabled.
func MaxScansPerWorkspace() int {
	if val := strings.TrimSpace(os.Getenv("BLUNTCODE_MAX_SCANS_PER_WORKSPACE")); val != "" {
		if val == "0" || strings.EqualFold(val, "off") || strings.EqualFold(val, "none") || strings.EqualFold(val, "disable") || strings.EqualFold(val, "disabled") {
			return 0
		}
		if n, err := strconv.Atoi(val); err == nil && n >= 1 {
			return n
		}
	}
	return DefaultMaxScansPerWorkspace
}

type Paths struct {
	DataDir    string
	DBPath     string
	LogsDir    string
	TempDir    string
	ToolsDir   string
	ReportsDir string
}

func DefaultPaths() (Paths, error) {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		var err error
		base, err = os.UserCacheDir()
		if err != nil {
			return Paths{}, fmt.Errorf("resolve local application data directory: %w", err)
		}
	}
	return NewPaths(filepath.Join(base, AppName))
}

func NewPaths(dataDir string) (Paths, error) {
	abs, err := filepath.Abs(dataDir)
	if err != nil {
		return Paths{}, fmt.Errorf("normalize data directory: %w", err)
	}
	p := Paths{DataDir: filepath.Clean(abs)}
	p.DBPath = filepath.Join(p.DataDir, "bluntcode.db")
	p.LogsDir = filepath.Join(p.DataDir, "logs")
	p.TempDir = filepath.Join(p.DataDir, "temp")
	p.ToolsDir = filepath.Join(p.DataDir, "tools")
	// ReportsDir is deliberately not created up front: the scan service
	// creates it when the first markdown report is written, so a fresh
	// install never carries an empty reports folder.
	p.ReportsDir = filepath.Join(p.DataDir, "reports")
	for _, dir := range []string{p.DataDir, p.LogsDir, p.TempDir, p.ToolsDir} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return Paths{}, fmt.Errorf("create %s: %w", dir, err)
		}
	}
	return p, nil
}
