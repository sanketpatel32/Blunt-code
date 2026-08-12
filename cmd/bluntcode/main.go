package main

import (
	"context"
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"bluntcode/internal/analyzers"
	"bluntcode/internal/analyzers/biome"
	"bluntcode/internal/analyzers/ruff"
	"bluntcode/internal/analyzers/semgrep"
	"bluntcode/internal/analyzers/sonarqube"
	"bluntcode/internal/api"
	"bluntcode/internal/config"
	"bluntcode/internal/core"
	"bluntcode/internal/database"
	"bluntcode/internal/doctor"
	"bluntcode/internal/events"
	"bluntcode/internal/instance"
	"bluntcode/internal/scans"
	"bluntcode/internal/tools"
	"bluntcode/internal/workspace"
)

const version = "0.1.0-dev"

//go:embed static/*
var staticFiles embed.FS

func main() {
	if len(os.Args) > 1 && os.Args[1] == "doctor" {
		runDoctor(os.Args[2:])
		return
	}
	runServer(os.Args[1:])
}

func runServer(args []string) {
	var noBrowser, showVersion bool
	var port int
	flags := flag.NewFlagSet("bluntcode", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	flags.BoolVar(&noBrowser, "no-browser", false, "do not open a browser")
	flags.BoolVar(&showVersion, "version", false, "print version")
	flags.IntVar(&port, "port", 0, "loopback port (0 selects a free port)")
	if err := flags.Parse(args); err != nil {
		os.Exit(2)
	}
	if showVersion {
		fmt.Println(version)
		return
	}
	if flags.NArg() > 1 {
		fmt.Fprintln(os.Stderr, "usage: bluntcode [path] [--no-browser] [--port N]")
		os.Exit(2)
	}
	paths, err := config.DefaultPaths()
	if err != nil {
		fatal(err)
	}
	logFile, err := os.OpenFile(filepath.Join(paths.LogsDir, "bluntcode.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err == nil {
		defer logFile.Close()
	}
	logger := slog.New(slog.NewTextHandler(logFile, &slog.HandlerOptions{Level: slog.LevelInfo}))
	guard, err := instance.Acquire(paths.DataDir)
	if err != nil {
		fatal(err)
	}
	defer guard.Close()
	db, err := database.Open(context.Background(), paths.DBPath)
	if err != nil {
		fatal(err)
	}
	defer db.Close()
	if err := db.MarkInterruptedScans(context.Background()); err != nil {
		fatal(err)
	}
	if flags.NArg() == 1 {
		root, normalizeErr := workspace.NormalizeRoot(flags.Arg(0))
		if normalizeErr != nil {
			fatal(normalizeErr)
		}
		if _, lookupErr := db.WorkspaceByRoot(context.Background(), root); lookupErr != nil {
			if _, createErr := db.CreateWorkspace(context.Background(), core.Workspace{Name: filepath.Base(root), RootPath: root}); createErr != nil {
				fatal(createErr)
			}
		}
	}
	listener, err := net.Listen("tcp4", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		fatal(fmt.Errorf("listen on loopback: %w", err))
	}
	defer listener.Close()
	bus := events.New()
	manifest, err := tools.DefaultManifest()
	if err != nil {
		fatal(fmt.Errorf("load embedded tool manifest: %w", err))
	}
	toolService := tools.NewService(paths.ToolsDir, manifest, false)
	appSettings, err := db.AppSettings(context.Background())
	if err != nil {
		fatal(fmt.Errorf("load application settings: %w", err))
	}
	toolService.SetOffline(appSettings.Offline)
	semgrepExecutable, semgrepRules, semgrepVersion := "", "", ""
	if semgrepPaths, ok := toolService.SemgrepPaths(); ok {
		semgrepExecutable, semgrepRules = semgrepPaths.Executable, semgrepPaths.RulesDir
	}
	semgrepVersion = toolService.Status("semgrep").Version
	managedSonar, err := sonarqube.NewManaged(paths.DataDir, toolService.Manager, manifest)
	if err != nil {
		fatal(fmt.Errorf("configure managed SonarQube: %w", err))
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if err := managedSonar.Shutdown(shutdownCtx); err != nil {
			logger.Warn("managed SonarQube shutdown failed", "error", err)
		}
	}()
	registry := analyzers.NewRegistry()
	// Analyzer adapters only execute Blunt Code-managed paths; no PATH lookup is used.
	_ = registry.Register(ruff.New(filepath.Join(paths.ToolsDir, "ruff", "0.16.0", "ruff.exe"), "0.16.0"))
	_ = registry.Register(biome.New(filepath.Join(paths.ToolsDir, "biome", "2.5.6", "biome.exe"), "2.5.6"))
	_ = registry.Register(semgrep.New(semgrepExecutable, semgrepVersion, semgrepRules))
	_ = registry.Register(managedSonar)
	scanService := scans.New(db, registry, bus, filepath.Join(paths.DataDir, "reports"), paths.ToolsDir, toolService)
	defer scanService.Shutdown()
	server := api.New(db, bus, scanService, toolService, paths, version, logger)
	root := http.NewServeMux()
	root.Handle("/api/", server.Handler())
	root.Handle("/", staticHandler())
	url := "http://" + listener.Addr().String() + "/"
	fmt.Println("Blunt Code listening on " + url)
	logger.Info("server started", "url", url)
	if !noBrowser && appSettings.OpenBrowser {
		openBrowser(url, logger)
	}
	httpServer := &http.Server{Handler: root, ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second}
	var shutdownOnce sync.Once
	shutdown := func() {
		shutdownOnce.Do(func() {
			logger.Info("shutdown requested")
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
			defer cancel()
			if err := httpServer.Shutdown(shutdownCtx); err != nil {
				logger.Warn("HTTP shutdown failed", "error", err)
			}
		})
	}
	server.SetShutdown(shutdown)
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)
	defer signal.Stop(interrupt)
	go func() {
		<-interrupt
		shutdown()
	}()
	if err := httpServer.Serve(listener); err != nil && err != http.ErrServerClosed {
		fatal(err)
	}
}

func runDoctor(args []string) {
	flags := flag.NewFlagSet("doctor", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	jsonOutput := flags.Bool("json", false, "write JSON diagnostics")
	if err := flags.Parse(args); err != nil {
		os.Exit(2)
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "usage: bluntcode doctor [--json]")
		os.Exit(2)
	}
	paths, err := config.DefaultPaths()
	if err != nil {
		fatal(err)
	}
	manifest, err := tools.DefaultManifest()
	if err != nil {
		fatal(fmt.Errorf("load embedded tool manifest: %w", err))
	}
	result := doctor.Run(context.Background(), doctor.Options{
		Version: version,
		Paths:   paths,
		Tools:   tools.NewService(paths.ToolsDir, manifest, true),
	})
	if *jsonOutput {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(result); err != nil {
			fatal(fmt.Errorf("write diagnostics: %w", err))
		}
	} else {
		result.WriteHuman(os.Stdout)
	}
	if code := result.ExitCode(); code != 0 {
		os.Exit(code)
	}
}
func staticHandler() http.Handler {
	content, err := fs.Sub(staticFiles, "static")
	if err != nil {
		panic(err)
	}
	files := http.FileServer(http.FS(content))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			name := strings.TrimPrefix(filepath.ToSlash(filepath.Clean(r.URL.Path)), "/")
			if _, err := fs.Stat(content, name); err != nil {
				clone := r.Clone(r.Context())
				clone.URL.Path = "/"
				files.ServeHTTP(w, clone)
				return
			}
		}
		files.ServeHTTP(w, r)
	})
}
func openBrowser(url string, logger *slog.Logger) {
	if runtime.GOOS != "windows" {
		return
	}
	if err := exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", url).Start(); err != nil {
		logger.Warn("could not open browser", "error", err)
	}
}
func fatal(err error) { fmt.Fprintln(os.Stderr, "bluntcode:", err); os.Exit(1) }
