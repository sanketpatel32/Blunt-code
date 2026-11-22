package api

import (
	"errors"
	"io"
	"io/fs"
	"net/http"
	"os"
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

// rootCheckConcurrency caps simultaneous root probes for one list page.
const rootCheckConcurrency = 8

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
	if _, err := dir.Readdirnames(1); err != nil && !errors.Is(err, io.EOF) {
		return rootProblemUnreadable
	}
	return ""
}

// rootProblems probes every root concurrently within rootCheckTimeout and
// returns the problem per index ("" for usable or unresolved roots).
func (s *Server) rootProblems(roots []string) []string {
	problems := make([]string, len(roots))
	if len(roots) == 0 {
		return problems
	}
	type result struct {
		index   int
		problem string
	}
	results := make(chan result, len(roots))
	limit := make(chan struct{}, rootCheckConcurrency)
	// done stops queued probes from starting once the response has moved on;
	// a probe already blocked in the kernel finishes into the buffered channel.
	done := make(chan struct{})
	defer close(done)
	check := s.checkRoot
	for i, root := range roots {
		go func() {
			select {
			case limit <- struct{}{}:
			case <-done:
				return
			}
			defer func() { <-limit }()
			results <- result{i, check(root)}
		}()
	}
	timer := time.NewTimer(s.rootTimeout)
	defer timer.Stop()
	for range roots {
		select {
		case r := <-results:
			problems[r.index] = r.problem
		case <-timer.C:
			return problems
		}
	}
	return problems
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
