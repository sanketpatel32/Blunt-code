package main

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
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
	"bluntcode/internal/build"
	"bluntcode/internal/config"
	"bluntcode/internal/doctor"
	"bluntcode/internal/instance"
	"bluntcode/internal/tools"
)

const version = build.Version // single source: internal/build/version.go

// defaultPort is the stable loopback port the app binds when --port is not
// given, so bookmarks, history entries, and pinned tabs survive restarts.
// --port 0 keeps the old free-port behavior; an explicit --port N is honored,
// and when that port already serves Blunt Code the new process hands off to
// the running instance instead of starting a second server.
const defaultPort = 8787

//go:embed static/*
var staticFiles embed.FS

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "doctor":
			runDoctor(os.Args[2:])
			return
		case "config":
			runConfig(os.Args[2:])
			return
		case "scan":
			runScan(os.Args[2:])
			return
		case "prune":
			runPrune(os.Args[2:])
			return
		case "agent":
			os.Exit(runAgent(os.Args[2:], os.Stdout, os.Stderr))
			return
		case "llm", "llms":
			os.Exit(runLLM(os.Args[2:], os.Stdout, os.Stderr))
			return
		case "workspace", "workspaces":
			os.Exit(runWorkspace(os.Args[2:], os.Stdout, os.Stderr))
			return
		case "findings", "finding":
			os.Exit(runFindings(os.Args[2:], os.Stdout, os.Stderr))
			return
		case "history", "scans":
			os.Exit(runHistory(os.Args[2:], os.Stdout, os.Stderr))
			return
		case "report", "reports":
			os.Exit(runReport(os.Args[2:], os.Stdout, os.Stderr))
			return
		case "suppress", "suppressions":
			os.Exit(runSuppress(os.Args[2:], os.Stdout, os.Stderr))
			return
		case "rules", "rule":
			os.Exit(runRules(os.Args[2:], os.Stdout, os.Stderr))
			return
		case "tools", "tool":
			os.Exit(runTools(os.Args[2:], os.Stdout, os.Stderr))
			return
		case "pentest":
			os.Exit(runPentest(os.Args[2:], os.Stdout, os.Stderr))
			return
		case "stats":
			os.Exit(runStats(os.Args[2:], os.Stdout, os.Stderr))
			return
		case "trends":
			os.Exit(runTrends(os.Args[2:], os.Stdout, os.Stderr))
			return
		case "risk":
			os.Exit(runRisk(os.Args[2:], os.Stdout, os.Stderr))
			return
		case "update":
			os.Exit(runUpdate(os.Args[2:], os.Stdout, os.Stderr))
			return
		case "version", "--version", "-v":
			fmt.Println(version)
			return
		case "cli", "docs":
			os.Exit(runCLIDocs(os.Args[2:], os.Stdout, os.Stderr))
			return
		case "help", "--help", "-h":
			printHelp(os.Stdout)
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
	flags.IntVar(&port, "port", defaultPort, "loopback port (0 selects a free port)")
	if err := flags.Parse(args); err != nil {
		os.Exit(2)
	}
	if showVersion {
		fmt.Println(version)
		return
	}
	if flags.NArg() > 1 {
		fmt.Fprintln(os.Stderr, "usage: bluntcode [--no-browser] [--port N] [path]")
		fmt.Fprintln(os.Stderr, "       bluntcode doctor [--json] [--fix]")
		fmt.Fprintln(os.Stderr, "       bluntcode config [--json]")
		fmt.Fprintln(os.Stderr, "       bluntcode scan <path> [--profile quick|standard|deep] [--json] [--timeout 30m] [--quiet] [--fail-on high+] [--max-findings N]")
		fmt.Fprintln(os.Stderr, "       bluntcode prune <path> [--keep N]")
		fmt.Fprintln(os.Stderr, "       bluntcode agent [--help] | bluntcode agent docs | bluntcode agent scan <path> [scan flags]")
		fmt.Fprintln(os.Stderr, "       bluntcode llm | bluntcode llms")
		os.Exit(2)
	}
	// Shared bootstrap (single-instance lock, database, tool service, analyzer
	// registry, scan service) lives in bootstrap.go and is reused by `scan`.
	app, release, err := openCore()
	if err != nil {
		// A second launch while the app is already running just reveals the
		// running instance instead of stacking another server on a new port.
		if errors.Is(err, instance.ErrAlreadyRunning) && handoffToRunning(noBrowser) {
			return
		}
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
		// The default port is taken: when its owner is a live Blunt Code
		// server (a previous launch, or one raced here), hand off to it
		// rather than failing or wandering to a random port.
		if port != 0 && handoffToPort(port, noBrowser) {
			return
		}
		fatal(fmt.Errorf("listen on loopback: %w", err))
	}
	defer listener.Close()
	if bound := boundPort(listener); bound != 0 {
		// Best-effort hint so a later handoff knows where this instance
		// serves; stale hints are harmless because handoff probes first.
		writeServerPort(bound)
		defer removeServerPort()
	}
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

const doctorUsage = "usage: bluntcode doctor [--json] [--fix]"

// doctorConfig is the validated command line of `bluntcode doctor`.
type doctorConfig struct {
	json bool
	fix  bool
}

// parseDoctorFlags parses the `bluntcode doctor` command line. --fix is a
// boolean flag (it takes no value) and combines with --json in any order.
// Parse failures and any positional argument are usage errors, exactly as
// before --fix existed.
func parseDoctorFlags(args []string, errOut io.Writer) (doctorConfig, error) {
	flags := flag.NewFlagSet("doctor", flag.ContinueOnError)
	flags.SetOutput(errOut)
	var cfg doctorConfig
	flags.BoolVar(&cfg.json, "json", false, "write JSON diagnostics")
	flags.BoolVar(&cfg.fix, "fix", false, "repair mechanical problems (missing data directories, stale local Semgrep rules, interrupted-install leftovers); repairs are refused while another Blunt Code process holds the data directory")
	if err := flags.Parse(args); err != nil {
		return doctorConfig{}, err
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(errOut, doctorUsage)
		return doctorConfig{}, fmt.Errorf("doctor takes no positional arguments")
	}
	return cfg, nil
}

func runDoctor(args []string) {
	cfg, err := parseDoctorFlags(args, os.Stderr)
	if err != nil {
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
		Fix:     cfg.fix,
	})
	if cfg.json {
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

// boundPort extracts the loopback port from a bound listener, or 0 when the
// address is not a TCP address.
func boundPort(listener net.Listener) int {
	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		return 0
	}
	return addr.Port
}

// serverPortPath locates the hint file where a running server records the
// port it bound, so a later launch can find the existing instance.
func serverPortPath() (string, error) {
	paths, err := config.DefaultPaths()
	if err != nil {
		return "", err
	}
	return filepath.Join(paths.DataDir, "server.port"), nil
}

func writeServerPort(port int) {
	path, err := serverPortPath()
	if err != nil {
		return
	}
	_ = os.WriteFile(path, []byte(fmt.Sprintf("%d", port)), 0o600)
}

func removeServerPort() {
	path, err := serverPortPath()
	if err != nil {
		return
	}
	_ = os.Remove(path)
}

// handoffToPort probes a loopback port for a live Blunt Code server and, when
// found, opens the browser there instead of starting a second instance. It
// reports whether the handoff happened; anything else (foreign owner, dead
// port, stale hint) leaves the caller to proceed normally.
func handoffToPort(port int, noBrowser bool) bool {
	if port <= 0 {
		return false
	}
	url := fmt.Sprintf("http://127.0.0.1:%d/", port)
	client := &http.Client{Timeout: 3 * time.Second}
	response, err := client.Get(url + "api/v1/meta")
	if err != nil {
		return false
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return false
	}
	fmt.Println("Blunt Code already running on " + url)
	if !noBrowser {
		openBrowser(url, slog.New(slog.NewTextHandler(io.Discard, nil)))
	}
	return true
}

// handoffToRunning hands off to the instance holding the single-instance lock
// by probing its recorded port hint.
func handoffToRunning(noBrowser bool) bool {
	path, err := serverPortPath()
	if err != nil {
		return false
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var port int
	if _, err := fmt.Sscanf(strings.TrimSpace(string(raw)), "%d", &port); err != nil {
		return false
	}
	return handoffToPort(port, noBrowser)
}
func fatal(err error) { fmt.Fprintln(os.Stderr, "bluntcode:", err); os.Exit(1) }

// printHelp writes the one-line usage of every subcommand. It goes to stdout
// so `bluntcode help` is pipeline-friendly, unlike the per-command usage
// errors which stay on stderr.
func printHelp(w io.Writer) {
	fmt.Fprintln(w, "Blunt Code "+version+" - local code quality and security analysis for Windows")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "  bluntcode [path] [--no-browser] [--port N]   start the web app (default command)")
	fmt.Fprintln(w, "  "+scanUsage)
	fmt.Fprintln(w, "  "+pruneUsage)
	fmt.Fprintln(w, "  "+doctorUsage)
	fmt.Fprintln(w, "  "+configUsage)
	fmt.Fprintln(w, "  "+workspaceUsage)
	fmt.Fprintln(w, "  bluntcode findings <search|list|preview> [args]")
	fmt.Fprintln(w, "  bluntcode history [path] | bluntcode history compare <id1> <id2>")
	fmt.Fprintln(w, "  "+reportUsage)
	fmt.Fprintln(w, "  bluntcode suppress <list|add|remove|import> <workspace> [args]")
	fmt.Fprintln(w, "  bluntcode rules <list|disable|enable|overrides> <workspace> [args]")
	fmt.Fprintln(w, "  bluntcode tools <list|install|repair|update> [tool-id]")
	fmt.Fprintln(w, "  bluntcode pentest probe <url> [--auth-mode ...] [--scope ...]")
	fmt.Fprintln(w, "  bluntcode stats [path] | bluntcode trends <path> | bluntcode risk <path>")
	fmt.Fprintln(w, "  bluntcode update check [--json]")
	fmt.Fprintln(w, "  bluntcode cli [command]   built-in reference manual and guides")
	fmt.Fprintln(w, "  "+agentUsage+"   agent helper (docs + scan with --json --quiet)")
	fmt.Fprintln(w, "  "+llmUsage)
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Run `bluntcode <command> --help` or `bluntcode cli <command>` for command details.")
	fmt.Fprintln(w, "Web UI documentation is available at /cli when the server is running.")
}
