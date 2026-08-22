package config

import (
	"fmt"
	"os"
	"path/filepath"
)

const AppName = "BluntCode"

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
