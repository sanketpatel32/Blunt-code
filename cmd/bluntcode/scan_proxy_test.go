package main

import (
	"bytes"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestRunScanThroughServerProxiesToRunningApp(t *testing.T) {
	token := "proxy-test-token"
	mux := http.NewServeMux()
	authed := func(w http.ResponseWriter, r *http.Request) bool {
		if r.Header.Get("Authorization") != "Bearer "+token {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return false
		}
		return true
	}
	writeJSON := func(w http.ResponseWriter, status int, v any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(v)
	}

	mux.HandleFunc("/api/v1/meta", func(w http.ResponseWriter, r *http.Request) {
		if !authed(w, r) {
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"api_version": "v1"})
	})
	mux.HandleFunc("/api/v1/workspaces", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method", http.StatusMethodNotAllowed)
			return
		}
		if !authed(w, r) {
			return
		}
		var body struct {
			RootPath string `json:"root_path"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.RootPath != "/tmp/demo" {
			http.Error(w, "bad workspace", http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"id": "ws-1", "name": "demo", "root_path": body.RootPath})
	})
	mux.HandleFunc("/api/v1/workspaces/ws-1/scans", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method", http.StatusMethodNotAllowed)
			return
		}
		if !authed(w, r) {
			return
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "bad scan", http.StatusBadRequest)
			return
		}
		if body["profile"] != "quick" || body["jobs"] != float64(2) || body["incremental"] != true {
			http.Error(w, "scan options not proxied", http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"id": "scan-1", "workspace_id": "ws-1", "state": "running", "profile": "quick"})
	})
	mux.HandleFunc("/api/v1/scans/scan-1", func(w http.ResponseWriter, r *http.Request) {
		if !authed(w, r) {
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"id": "scan-1", "workspace_id": "ws-1", "state": "completed", "profile": "quick"})
	})
	mux.HandleFunc("/api/v1/scans/scan-1/events", func(w http.ResponseWriter, r *http.Request) {
		if !authed(w, r) {
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte("event: scan.started\ndata: {\"type\":\"scan.started\",\"scan_id\":\"scan-1\",\"data\":{\"state\":\"running\"}}\n\n"))
		_, _ = w.Write([]byte("event: scan.completed\ndata: {\"type\":\"scan.completed\",\"scan_id\":\"scan-1\",\"data\":{\"state\":\"completed\"}}\n\n"))
		flusher.Flush()
		<-r.Context().Done()
	})
	mux.HandleFunc("/api/v1/scans/scan-1/findings.json", func(w http.ResponseWriter, r *http.Request) {
		if !authed(w, r) {
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"schema":        "bluntcode/scan-report",
			"schemaVersion": 1,
			"workspace":     map[string]any{"name": "demo", "path": "/tmp/demo"},
			"scan":          map[string]any{"id": "scan-1", "profile": "quick", "state": "completed", "started_at": "2026-11-22T00:00:00Z", "finished_at": "2026-11-22T00:00:01Z"},
			"files":         map[string]any{"candidate": 1, "selected": 1, "skipped": 0},
			"severity":      map[string]any{"critical": 0, "high": 1, "medium": 0, "low": 0, "info": 0, "total": 1},
			"analyzers":     []any{},
			"findings":      []any{map[string]any{"severity": "high", "category": "security", "analyzer_id": "semgrep", "rule_id": "demo.rule", "title": "Demo", "message": "demo", "file": "app.py", "start_line": 1}},
			"metrics":       []any{},
			"comparison":    map[string]any{"new": 1, "fixed": 0, "persistent": 0},
			"warnings":      []any{},
		})
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	port := server.Listener.Addr().(*net.TCPAddr).Port

	base := t.TempDir()
	t.Setenv("LOCALAPPDATA", base)
	if err := os.WriteFile(mustServerPortPath(t), []byte(strconv.Itoa(port)), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mustServerTokenPath(t), []byte(token), 0o600); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	cfg := scanConfig{
		path:        "/tmp/demo",
		profile:     "quick",
		format:      scanFormatText,
		timeout:     5 * time.Second,
		quiet:       true,
		jobs:        2,
		incremental: true,
	}
	code, proxied := runScanThroughServer(cfg, &stdout, &stderr)
	if !proxied {
		t.Fatal("expected proxy to handle the scan")
	}
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "Blunt Code scan of demo (quick profile) completed") || !strings.Contains(out, "1 high") {
		t.Fatalf("stdout = %q", out)
	}
}

func mustServerPortPath(t *testing.T) string {
	t.Helper()
	path, err := serverPortPath()
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func mustServerTokenPath(t *testing.T) string {
	t.Helper()
	path, err := serverTokenPath()
	if err != nil {
		t.Fatal(err)
	}
	return path
}
