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

	"bluntcode/internal/api"
	"bluntcode/internal/config"
	"bluntcode/internal/doctor"
	"bluntcode/internal/tools"
)

const version = "0.2.1"

//go:embed static/*
var staticFiles embed.FS

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "doctor":
			runDoctor(os.Args[2:])
			return
		case "scan":
			runScan(os.Args[2:])
			return
		}
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
		fmt.Fprintln(os.Stderr, "       bluntcode doctor [--json]")
		fmt.Fprintln(os.Stderr, "       bluntcode scan <path> [--profile quick|standard|deep] [--json] [--timeout 30m] [--quiet]")
		os.Exit(2)
	}
	// Shared bootstrap (single-instance lock, database, tool service, analyzer
	// registry, scan service) lives in bootstrap.go and is reused by `scan`.
	app, release, err := openCore()
	if err != nil {
		fatal(err)
	}
	defer release()
	if flags.NArg() == 1 {
		if _, err := ensureWorkspace(context.Background(), app.db, flags.Arg(0)); err != nil {
			fatal(err)
		}
	}
	listener, err := net.Listen("tcp4", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		fatal(fmt.Errorf("listen on loopback: %w", err))
	}
	defer listener.Close()
	server := api.New(app.db, app.bus, app.scans, app.toolService, app.paths, version, app.logger)
	root := http.NewServeMux()
	root.Handle("/api/", server.Handler())
	root.Handle("/", staticHandler())
	url := "http://" + listener.Addr().String() + "/"
	fmt.Println("Blunt Code listening on " + url)
	app.logger.Info("server started", "url", url)
	if !noBrowser && app.settings.OpenBrowser {
		openBrowser(url, app.logger)
	}
	httpServer := &http.Server{Handler: root, ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second}
	var shutdownOnce sync.Once
	shutdown := func() {
		shutdownOnce.Do(func() {
			app.logger.Info("shutdown requested")
			app.scans.Shutdown()
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
			defer cancel()
			if err := httpServer.Shutdown(shutdownCtx); err != nil {
				app.logger.Warn("HTTP shutdown failed", "error", err)
			}
		})
	}
	server.SetShutdown(shutdown)
	interrupt := make(chan os.Signal, 2)
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
