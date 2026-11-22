// system handlers handlers.
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"

	"bluntcode/internal/cleaner"
	"bluntcode/internal/config"
	"bluntcode/internal/process"
	"bluntcode/internal/windows/folderpicker"
)

func (s *Server) selectFolder(w http.ResponseWriter, r *http.Request) {
	if runtime.GOOS != "windows" {
		fail(w, 400, "UNSUPPORTED_PLATFORM", "The native folder picker is only available on Windows.")
		return
	}
	path, cancelled, err := folderpicker.Select(r.Context())
	if err != nil {
		fail(w, 500, "FOLDER_PICKER_FAILED", "Could not open the native folder picker.")
		return
	}
	writeJSON(w, 200, map[string]any{"cancelled": cancelled, "path": path})
}

func (s *Server) stopServer(w http.ResponseWriter, r *http.Request) {
	if s.shutdown == nil {
		fail(w, 503, "SERVER_STOP_UNAVAILABLE", "The server cannot be stopped from this session.")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"state": "stopping"})
	// Flush the acknowledgement before the process can exit, or callers see a
	// dropped connection instead of the 202.
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
	go s.shutdown()
}

// openSystemFolder opens one of Blunt Code's own data folders in Windows
// Explorer. The request carries a fixed folder enum, never a path: the server
// resolves the kind against its configured directories, so a caller can never
// direct Explorer (or the process launcher) at an arbitrary location.
func (s *Server) openSystemFolder(w http.ResponseWriter, r *http.Request) {
	if runtime.GOOS != "windows" {
		fail(w, 400, "UNSUPPORTED_PLATFORM", "Opening a folder in the system file browser is only supported on Windows.")
		return
	}
	var input struct {
		Kind string `json:"kind"`
	}
	// decode enforces DisallowUnknownFields, so smuggled extras such as a raw
	// path key are rejected; every malformed shape here is an invalid kind.
	if err := decode(r, &input); err != nil {
		fail(w, 400, "INVALID_FOLDER_KIND", "kind must be data, reports, logs, or tools.")
		return
	}
	dir, ok := folderForKind(s.paths, input.Kind)
	if !ok {
		fail(w, 400, "INVALID_FOLDER_KIND", "kind must be data, reports, logs, or tools.")
		return
	}
	info, err := os.Stat(dir)
	if errors.Is(err, os.ErrNotExist) || err == nil && !info.IsDir() {
		fail(w, 409, "FOLDER_NOT_FOUND", "This folder has not been created yet.")
		return
	}
	if err != nil {
		fail(w, 500, "FOLDER_OPEN_FAILED", "This folder could not be opened.")
		return
	}
	if err := s.openFolder(dir); err != nil {
		s.log.Warn("open folder failed", "kind", input.Kind, "error", err)
		fail(w, 500, "FOLDER_OPEN_FAILED", "Windows Explorer could not open this folder.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) cleanSystem(w http.ResponseWriter, r *http.Request) {
	if s.db != nil {
		if summary, err := s.db.ScanSummary(r.Context()); err == nil && summary.ActiveScans > 0 {
			fail(w, 409, "SCAN_IN_PROGRESS", "Cannot clean storage while scans are running. Please wait for active scans to finish.")
			return
		}
	}

	var input struct {
		Logs   *bool `json:"logs,omitempty"`
		Cache  *bool `json:"cache,omitempty"`
		Vacuum *bool `json:"vacuum,omitempty"`
		All    *bool `json:"all,omitempty"`
	}
	if r.Body != nil {
		bodyBytes, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			fail(w, 400, "INVALID_REQUEST", "Could not read the request body.")
			return
		}
		if len(strings.TrimSpace(string(bodyBytes))) > 0 {
			d := json.NewDecoder(bytes.NewReader(bodyBytes))
			d.DisallowUnknownFields()
			if err := d.Decode(&input); err != nil {
				fail(w, 400, "INVALID_REQUEST", "Malformed JSON request body.")
				return
			}
		}
	}
	opts := cleaner.Options{}
	if input.All == nil && input.Logs == nil && input.Cache == nil && input.Vacuum == nil {
		// A destructive endpoint must never default to "delete everything":
		// the caller picks a target explicitly ({"all":true} included).
		fail(w, 400, "SELECT_TARGET", "Specify what to clean: logs, cache, vacuum, or all.")
		return
	}
	if input.All != nil && *input.All {
		opts.All = true
	} else {
		if input.Logs != nil {
			opts.Logs = *input.Logs
		}
		if input.Cache != nil {
			opts.Cache = *input.Cache
		}
		if input.Vacuum != nil {
			opts.Vacuum = *input.Vacuum
		}
	}

	result, err := cleaner.Clean(r.Context(), s.paths, s.db, opts)
	if err != nil {
		fail(w, 500, "CLEAN_FAILED", err.Error())
		return
	}
	writeJSON(w, 200, result)
}

// folderForKind resolves the fixed folder enum to server-held config paths.
func folderForKind(paths config.Paths, kind string) (string, bool) {
	switch kind {
	case "data":
		return paths.DataDir, true
	case "reports":
		return paths.ReportsDir, true
	case "logs":
		return paths.LogsDir, true
	case "tools":
		return paths.ToolsDir, true
	}
	return "", false
}

// openFolderInExplorer launches Explorer on dir with a direct argument vector
// (no shell), so directories containing spaces are safe by construction.
// explorer.exe routinely exits nonzero (1) even after a successful hand-off to
// the shell; process.Run reports start and timeout failures as errors and exit
// codes as successful results, so that nonzero exit is correctly ignored.
func openFolderInExplorer(dir string) error {
	ctx, cancel := context.WithTimeout(context.Background(), openFolderTimeout)
	defer cancel()
	_, err := process.Run(ctx, process.Request{Command: "explorer.exe", Args: []string{dir}})
	return err
}
