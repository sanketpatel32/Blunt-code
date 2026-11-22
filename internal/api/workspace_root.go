package api

import (
	"errors"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"bluntcode/internal/core"
)

// Root problems reported on workspace views (root_problem) when the folder a
// workspace was registered against can no longer be used.
const (
	rootProblemMissing      = "missing"
	rootProblemNotDirectory = "not_directory"
	rootProblemUnreadable   = "unreadable"
)

// rootCheckTimeout bounds the root probes behind one response: a stat on a
// disconnected network share can block for a long time, and a slow probe
// must not hold the workspace list hostage. Roots still unresolved when it
// elapses are reported healthy (unknown), never missing.
const rootCheckTimeout = 2 * time.Second

// rootProber shares one in-flight probe per root across requests, so a
// folder whose stat hangs pins at most one goroutine however often the
// workspace list is refreshed.
type rootProber struct {
	mu       sync.Mutex
	inflight map[string]*rootProbe
}

type rootProbe struct {
	done    chan struct{}
	problem string
}

// checkWorkspaceRoot classifies a workspace root with one stat and a
// one-entry directory read: missing, not a directory, unreadable, or ""
// when the folder is usable.
func checkWorkspaceRoot(root string) string {
	info, err := os.Stat(root)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return rootProblemMissing
	case err != nil:
		return rootProblemUnreadable
	case !info.IsDir():
		return rootProblemNotDirectory
	}
	dir, err := os.Open(root)
	if err != nil {
		return rootProblemUnreadable
	}
	defer dir.Close()
	names, err := dir.Readdirnames(1)
	if err != nil && !errors.Is(err, io.EOF) {
		return rootProblemUnreadable
	}
	// Listing needs read permission but reaching entries needs search
	// (execute) permission on POSIX; a scan needs both.
	if len(names) > 0 {
		if _, err := os.Lstat(filepath.Join(root, names[0])); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return rootProblemUnreadable
		}
	}
	return ""
}

// rootProblemTimeout is the distinct "we could not tell" verdict: a hung or
// slow mount used to report healthy ("") because unresolved meant usable.
const rootProblemTimeout = "probe_timeout"

// rootProblems probes every root concurrently within rootTimeout and
// returns the problem per index ("" for usable; probe_timeout for roots the
// probe could not resolve in time).
func (s *Server) rootProblems(roots []string) []string {
	problems := make([]string, len(roots))
	if len(roots) == 0 {
		return problems
	}
	probes := make([]*rootProbe, len(roots))
	for i, root := range roots {
		probes[i] = s.probeRoot(root)
	}
	timer := time.NewTimer(s.rootTimeout)
	defer timer.Stop()
	expired := false
	for i, probe := range probes {
		if !expired {
			select {
			case <-probe.done:
				problems[i] = probe.problem
				continue
			case <-timer.C:
				expired = true
			}
		}
		select {
		case <-probe.done:
			problems[i] = probe.problem
		default:
			problems[i] = rootProblemTimeout
		}
	}
	return problems
}

// probeRoot joins the in-flight probe for root or starts one.
func (s *Server) probeRoot(root string) *rootProbe {
	s.probes.mu.Lock()
	defer s.probes.mu.Unlock()
	if probe, ok := s.probes.inflight[root]; ok {
		return probe
	}
	if s.probes.inflight == nil {
		s.probes.inflight = map[string]*rootProbe{}
	}
	probe := &rootProbe{done: make(chan struct{})}
	s.probes.inflight[root] = probe
	check := s.checkRoot
	go func() {
		probe.problem = check(root)
		s.probes.mu.Lock()
		delete(s.probes.inflight, root)
		s.probes.mu.Unlock()
		close(probe.done)
	}()
	return probe
}

func (s *Server) rootProblem(root string) string {
	return s.rootProblems([]string{root})[0]
}

// rootProblemMessage is the user-facing sentence for a root problem.
func rootProblemMessage(work core.Workspace, problem string) string {
	switch problem {
	case rootProblemNotDirectory:
		return "The workspace folder " + work.RootPath + " is now a file, not a folder. Restore the folder or remove this workspace."
	case rootProblemUnreadable:
		return "The workspace folder " + work.RootPath + " cannot be read. Check its permissions or remove this workspace."
	default:
		return "The workspace folder " + work.RootPath + " no longer exists. Restore the folder or remove this workspace."
	}
}

// failRootUnavailable answers a request that needs the workspace's files when
// its root is gone or unreadable, reporting false when the root is usable.
func (s *Server) failRootUnavailable(w http.ResponseWriter, work core.Workspace) bool {
	problem := s.rootProblem(work.RootPath)
	if problem == "" {
		return false
	}
	writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": map[string]any{
		"code":    "WORKSPACE_ROOT_MISSING",
		"message": rootProblemMessage(work, problem),
		"details": map[string]string{"root_path": work.RootPath, "root_problem": problem},
	}})
	return true
}
