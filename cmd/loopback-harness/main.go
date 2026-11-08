// Loopback UI harness — development / Playwright evidence only. Not shipped in
// production builds. Serves UI + api.New(..., nil, nil, ...) without Windows Sonar.
package main

import (
	"context"
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"bluntcode/internal/api"
	"bluntcode/internal/config"
	"bluntcode/internal/database"
	"bluntcode/internal/events"
)

//go:embed static/*
var staticFiles embed.FS

func main() {
	port := flag.Int("port", 9876, "loopback TCP port (127.0.0.1 only)")
	data := flag.String("data", "", "required isolated data directory (never use production Blunt Code data)")
	staticDir := flag.String("static-dir", "", "serve UI from this directory instead of embedded static (for before/after captures)")
	demoSeed := flag.Bool("demo-seed", false, "set BLUNTCODE_DEMO_SEED=1 so POST /api/v1/system/seed-demo is accepted (opt-in)")
	devAck := flag.Bool("i-understand-dev-only", false, "required: acknowledge this is dev-only loopback testing")
	flag.Parse()

	if !*devAck {
		log.Fatal(`refusing to start: pass -i-understand-dev-only (dev-only loopback harness; see README)`)
	}
	dataRoot := strings.TrimSpace(*data)
	if dataRoot == "" {
		log.Fatal(`refusing to start: pass -data with a fresh empty directory (never your real Blunt Code app data)`)
	}
	dataRoot = filepath.Clean(dataRoot)
	if err := os.MkdirAll(dataRoot, 0o700); err != nil {
		log.Fatal(err)
	}
	abs, _ := filepath.Abs(dataRoot)
	fmt.Fprintf(os.Stderr, "harness: isolated data directory %s (SQLite only; no analyzers)\n", abs)

	if *demoSeed {
		_ = os.Setenv("BLUNTCODE_DEMO_SEED", "1")
		fmt.Fprintln(os.Stderr, "harness: demo seed API enabled (POST /api/v1/system/seed-demo only)")
	}

	paths, err := config.NewPaths(dataRoot)
	if err != nil {
		log.Fatal(err)
	}
	db, err := database.Open(context.Background(), paths.DBPath)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	server := api.New(db, events.New(), nil, nil, paths, "harness", nil)
	root := http.NewServeMux()
	root.Handle("/api/", server.Handler())
	root.Handle("/", api.StaticGuard(staticHandler(strings.TrimSpace(*staticDir))))

	addr := fmt.Sprintf("127.0.0.1:%d", *port)
	listener, err := net.Listen("tcp4", addr)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("loopback harness on http://" + listener.Addr().String() + "/")
	log.Fatal(http.Serve(listener, root))
}

func staticHandler(staticDir string) http.Handler {
	if staticDir != "" {
		root := http.Dir(filepath.Clean(staticDir))
		files := http.FileServer(root)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/" {
				w.Header().Set("Cache-Control", "no-cache")
			}
			name := strings.TrimPrefix(filepath.ToSlash(filepath.Clean(r.URL.Path)), "/")
			if r.URL.Path != "/" {
				if f, err := root.Open(name); err != nil {
					if !strings.Contains(name, ".") {
						r.URL.Path = "/"
					}
				} else {
					_ = f.Close()
				}
			}
			files.ServeHTTP(w, r)
		})
	}
	content, err := fs.Sub(staticFiles, "static")
	if err != nil {
		panic(err)
	}
	files := http.FileServer(http.FS(content))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			w.Header().Set("Cache-Control", "no-cache")
			files.ServeHTTP(w, r)
			return
		}
		name := strings.TrimPrefix(filepath.ToSlash(filepath.Clean(r.URL.Path)), "/")
		if _, err := fs.Stat(content, name); err != nil && !strings.Contains(name, ".") {
			r.URL.Path = "/"
			files.ServeHTTP(w, r)
			return
		}
		files.ServeHTTP(w, r)
	})
}
