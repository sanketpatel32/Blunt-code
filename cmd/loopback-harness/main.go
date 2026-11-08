// Linux UI harness: serves the built static UI and api.New(..., nil, nil, ...) without
// Windows managed SonarQube bootstrap. For Playwright evidence only.
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
	port := flag.Int("port", 9876, "loopback TCP port")
	data := flag.String("data", "", "isolated data directory (default: temp under os.TempDir)")
	flag.Parse()

	dataRoot := strings.TrimSpace(*data)
	if dataRoot == "" {
		var err error
		dataRoot, err = os.MkdirTemp("", "bluntcode-loopback-harness-*")
		if err != nil {
			log.Fatal(err)
		}
		defer os.RemoveAll(dataRoot)
	}
	_ = os.Setenv("BLUNTCODE_DEMO_SEED", "1")

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
	root.Handle("/", api.StaticGuard(staticHandler()))

	listener, err := net.Listen("tcp4", fmt.Sprintf("127.0.0.1:%d", *port))
	if err != nil {
		log.Fatal(err)
	}
	addr := listener.Addr().String()
	fmt.Println("loopback harness on http://" + addr + "/ (data:", dataRoot, ")")
	log.Fatal(http.Serve(listener, root))
}

func staticHandler() http.Handler {
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
