package main

// Shared service bootstrap for the server UI path and the headless scan CLI.
// Both entry points must attach to the same data directory, honor the same
// single-instance lock, and build the identical analyzer stack so a CLI scan
// behaves exactly like one started from the browser.

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"bluntcode/internal/analyzers"
	"bluntcode/internal/analyzers/biome"
	analyzerscheckov "bluntcode/internal/analyzers/checkov"
	"bluntcode/internal/analyzers/dastprobe"
	"bluntcode/internal/analyzers/gitleaks"
	"bluntcode/internal/analyzers/license"
	analyzersosv "bluntcode/internal/analyzers/osv"
	"bluntcode/internal/analyzers/pentest"
	"bluntcode/internal/analyzers/ruff"
	"bluntcode/internal/analyzers/secrets"
	"bluntcode/internal/analyzers/semgrep"
	"bluntcode/internal/analyzers/sonarqube"
	"bluntcode/internal/analyzers/todo"
	analyzerstrivy "bluntcode/internal/analyzers/trivy"
	"bluntcode/internal/config"
	"bluntcode/internal/core"
	"bluntcode/internal/database"
	"bluntcode/internal/events"
	"bluntcode/internal/instance"
	"bluntcode/internal/scans"
	"bluntcode/internal/tools"
	"bluntcode/internal/workspace"
)

// appCore bundles the long-lived services shared by `bluntcode` (server) and
// `bluntcode scan`. It is constructed by openCore and released with the
// returned release function.
type appCore struct {
	paths       config.Paths
	logger      *slog.Logger
	db          *database.DB
	bus         *events.Bus
	toolService *tools.Service
	registry    *analyzers.Registry
	scans       *scans.Service
	settings    database.AppSettings
}

// openCore acquires the single-instance lock, opens the database, and builds
// the tool service, managed SonarQube, analyzer registry, and scan service in
// exactly the order the server path has always used. release tears everything
// down in reverse dependency order: scans first (so running analyzers stop
// before managed processes), then SonarQube, database, lock, and log file.
func openCore() (core *appCore, release func(), err error) {
	paths, err := config.DefaultPaths()
	if err != nil {
		return nil, nil, err
	}
	logSink := io.WriteCloser(nopWriteCloser{})
	if file, openErr := os.OpenFile(filepath.Join(paths.LogsDir, "bluntcode.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600); openErr == nil {
		logSink = file
	}
	logger := slog.New(slog.NewTextHandler(logSink, &slog.HandlerOptions{Level: slog.LevelInfo}))
	// Every early return below must release what it owns so far — including
	// the log file, whose open handle otherwise outlives the failed start and
	// (on Windows) keeps the data directory locked.
	abort := func(err error) (*appCore, func(), error) {
		_ = logSink.Close()
		return nil, nil, err
	}
	guard, err := instance.Acquire(paths.DataDir)
	if err != nil {
		return abort(err)
	}
	db, err := database.Open(context.Background(), paths.DBPath)
	if err != nil {
		_ = guard.Close()
		return abort(err)
	}
	if err := db.MarkInterruptedScans(context.Background()); err != nil {
		_ = db.Close()
		_ = guard.Close()
		return abort(err)
	}
	// The git_repository column postdates most workspace rows: refresh it at
	// startup rather than leaving pre-migration rows stuck on the zero value
	// until something happens to rediscover them. The probe is a few stat
	// calls per workspace — well under a millisecond even for a large fleet.
	if all, err := db.Workspaces(context.Background()); err == nil {
		for _, work := range all {
			detected := workspace.IsGitRepository(work.RootPath)
			if detected != work.GitRepository {
				if err := db.SetWorkspaceGitRepository(context.Background(), work.ID, detected); err != nil {
					logger.Warn("startup git_repository refresh failed", "workspace", work.ID, "error", err)
				}
			}
		}
	}
	bus := events.New()
	manifest, err := tools.DefaultManifest()
	if err != nil {
		_ = db.Close()
		_ = guard.Close()
		return abort(fmt.Errorf("load embedded tool manifest: %w", err))
	}
	toolService := tools.NewService(paths.ToolsDir, manifest, false)
	toolService.SetDataDir(paths.DataDir)
	// Recover from interrupted updates before any readiness decision: drop
	// stale download temps and staging directories, restore a version
	// directory whose activation died between the two renames.
	toolService.SweepStaging()
	appSettings, err := db.AppSettings(context.Background())
	if err != nil {
		_ = db.Close()
		_ = guard.Close()
		return abort(fmt.Errorf("load application settings: %w", err))
	}
	toolService.SetOffline(appSettings.Offline)
	semgrepExecutable, semgrepRules, semgrepVersion := "", "", ""
	if semgrepPaths, ok := toolService.SemgrepPaths(); ok {
		semgrepExecutable, semgrepRules = semgrepPaths.Executable, semgrepPaths.RulesDir
	}
	semgrepVersion = toolService.Status("semgrep").Version
	managedSonar, err := sonarqube.NewManaged(paths.DataDir, toolService.Manager, manifest)
	if err != nil {
		// One optional analyzer's platform restriction must not take the whole
		// app down: warn and keep booting — the registry/UI already model
		// not-ready tools.
		logger.Warn("managed SonarQube unavailable; scans run without it", "error", err)
		managedSonar = nil
	}
	registry := analyzers.NewRegistry()
	// Analyzer adapters only execute Blunt Code-managed paths; no PATH lookup is used.
	_ = registry.Register(ruff.New(filepath.Join(paths.ToolsDir, "ruff", "0.16.0", "ruff.exe"), "0.16.0"))
	_ = registry.Register(biome.New(filepath.Join(paths.ToolsDir, "biome", "2.5.6", "biome.exe"), "2.5.6"))
	_ = registry.Register(gitleaks.New(filepath.Join(paths.ToolsDir, "gitleaks-secrets", "8.30.1", "gitleaks.exe"), "8.30.1"))
	// Both dependency scanners always match locally (osv against its managed
	// DB cache, trivy with telemetry and update checks off). Offline mode
	// additionally stops their database downloads: osv refuses to run when
	// the databases were never fetched, and trivy refuses cold-cache scans.
	osvAdapter := analyzersosv.New(filepath.Join(paths.ToolsDir, "osv-dependencies", "2.5.1", "osv-scanner.exe"), "2.5.1")
	osvAdapter.Offline = appSettings.Offline
	osvAdapter.DBCacheDir = filepath.Join(paths.DataDir, "osv-db-cache")
	_ = registry.Register(osvAdapter)
	// trivy's vulnerability DB decompresses to ~1.3 GB, so its cache is pinned
	// inside the app data dir instead of trivy's user-wide %LOCALAPPDATA% default.
	trivyAdapter := analyzerstrivy.New(filepath.Join(paths.ToolsDir, "container-trivy", "0.74.0", "trivy.exe"), "0.74.0")
	trivyAdapter.CacheDir = filepath.Join(paths.DataDir, "trivy-cache")
	trivyAdapter.Offline = appSettings.Offline
	_ = registry.Register(trivyAdapter)
	// checkov runs through its uv tool venv's interpreter (uv's Windows shim
	// resolves python from PATH, so python -m checkov.main is the hermetic
	// entry); the path mirrors the manifest's executable field.
	_ = registry.Register(analyzerscheckov.New(filepath.Join(paths.ToolsDir, "iac-checkov", "3.3.16", "env", "checkov", "Scripts", "python.exe"), "3.3.16"))
	_ = registry.Register(semgrep.New(semgrepExecutable, semgrepVersion, semgrepRules))
	if managedSonar != nil {
		_ = registry.Register(managedSonar)
	}
	registerBuiltinAnalyzers(registry, toolService.Offline)
	scanService := scans.New(db, registry, bus, paths.ReportsDir, paths.ToolsDir, toolService)
	app := &appCore{
		paths:       paths,
		logger:      logger,
		db:          db,
		bus:         bus,
		toolService: toolService,
		registry:    registry,
		scans:       scanService,
		settings:    appSettings,
	}
	release = func() {
		// Scans must reach a terminal state before managed SonarQube stops
		// and the database closes; otherwise late analyzer writes hit a
		// closed store.
		stopScans(scanService, logger)
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if managedSonar != nil {
			if err := managedSonar.Shutdown(shutdownCtx); err != nil {
				logger.Warn("managed SonarQube shutdown failed", "error", err)
			}
		}
		_ = db.Close()
		_ = guard.Close()
		_ = logSink.Close()
	}
	return app, release, nil
}

// scanShutdownTimeout bounds how long shutdown waits for cancelled scans to
// record their terminal state. Scans still running after it are reconciled to
// "interrupted" on the next start.
const scanShutdownTimeout = 15 * time.Second

// stopScans cancels every running scan and waits (bounded) for the scan
// goroutines to finish. Safe to call more than once.
func stopScans(service *scans.Service, logger *slog.Logger) {
	ctx, cancel := context.WithTimeout(context.Background(), scanShutdownTimeout)
	defer cancel()
	if err := service.Shutdown(ctx); err != nil {
		logger.Warn("scans did not stop before the shutdown timeout", "error", err)
	}
}

// nopWriteCloser keeps openCore logging-safe when the log file cannot be
// opened: slog must never panic on a nil writer.
type nopWriteCloser struct{}

func (nopWriteCloser) Write(p []byte) (int, error) { return len(p), nil }
func (nopWriteCloser) Close() error                { return nil }

// ensureWorkspace registers pathArg as a workspace when it is new and returns
// the workspace record. NormalizeRoot resolves to an absolute, symlink-free
// directory including NTFS junctions. The lookup also tolerates case
// differences (and 8.3 short-name spellings) so re-scanning a path never
// duplicates a workspace on Windows.
func ensureWorkspace(ctx context.Context, db *database.DB, pathArg string) (core.Workspace, error) {
	root, err := workspace.NormalizeRoot(pathArg)
	if err != nil {
		return core.Workspace{}, err
	}
	// Existing workspaces refresh their git_repository flag on return — the
	// folder may have gained or lost .git since it was added, and the CLI
	// path never goes through the discover endpoint that also refreshes it.
	refreshGit := func(existing core.Workspace) core.Workspace {
		if detected := workspace.IsGitRepository(root); detected != existing.GitRepository {
			if err := db.SetWorkspaceGitRepository(ctx, existing.ID, detected); err == nil {
				existing.GitRepository = detected
			}
		}
		return existing
	}
	if existing, lookupErr := db.WorkspaceByRoot(ctx, root); lookupErr == nil {
		return refreshGit(existing), nil
	}
	key := workspace.CanonicalKey(root)
	if all, listErr := db.Workspaces(ctx); listErr == nil {
		for _, existing := range all {
			if workspace.CanonicalKey(existing.RootPath) == key {
				return refreshGit(existing), nil
			}
		}
	}
	return db.CreateWorkspace(ctx, core.Workspace{Name: filepath.Base(root), RootPath: root, GitRepository: workspace.IsGitRepository(root)})
}

// registerBuiltinAnalyzers adds the in-process analyzers. The four static
// built-ins (pentest patterns, secrets, TODO markers, licenses) never touch
// the network, so they register in offline mode too: offline must keep the
// checks that are guaranteed local and withhold only network-dependent work.
// The DAST probe is the one outbound built-in; offline mode keeps it but
// narrows it to loopback targets, so it fails honestly instead of dialing
// out. offline is read live (the tool service's setting, which Settings
// updates), so toggling offline mode needs no restart. Managed-tool analyzers keep failing honestly when their tool was never
// downloaded (offline mode refuses downloads).
func registerBuiltinAnalyzers(registry *analyzers.Registry, offline func() bool) {
	_ = registry.Register(pentest.New())
	_ = registry.Register(secrets.New())
	_ = registry.Register(todo.New())
	_ = registry.Register(license.New())
	probe := dastprobe.New()
	probe.Offline = offline
	_ = registry.Register(probe)
}
