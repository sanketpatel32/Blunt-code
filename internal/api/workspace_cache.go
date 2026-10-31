package api

import (
	"context"
	"strings"
	"sync"
	"time"

	"bluntcode/internal/core"
	"bluntcode/internal/discovery"
)

// languageCacheTTL bounds how long a never-scanned workspace's discovered
// language list is reused. Languages only matter for workspaces without a scan
// snapshot, and a folder's language mix rarely changes within minutes.
const languageCacheTTL = 5 * time.Minute

// workspaceCaches keeps the dashboard list off the filesystem and away from
// per-request full finding loads:
//
//   - languages: workspaces that were never scanned have no snapshot, so their
//     language chips need a directory walk. The list endpoint never walks;
//     it reads this cache and, on a miss, warms it in the background (one walk
//     at a time) so the next list refresh has the chips.
//   - comparisons: the new/fixed pair for a finished scan only changes when
//     its previous scan or the workspace's suppressions change. Entries are
//     keyed by scan and previous scan, and suppression changes drop the
//     workspace's entries.
type workspaceCaches struct {
	mu          sync.Mutex
	languages   map[string]languageEntry
	warming     map[string]bool
	comparisons map[string]comparisonEntry
	walk        chan struct{}
}

type languageEntry struct {
	key   string
	names []string
	at    time.Time
}

type comparisonEntry struct {
	workspaceID string
	newCount    int
	fixedCount  int
}

func newWorkspaceCaches() *workspaceCaches {
	return &workspaceCaches{languages: map[string]languageEntry{}, warming: map[string]bool{}, comparisons: map[string]comparisonEntry{}, walk: make(chan struct{}, 1)}
}

func languageKey(root string, patterns []string) string {
	return root + "\x00" + strings.Join(patterns, "\x00")
}

func (c *workspaceCaches) cachedLanguages(id, key string) ([]string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.languages[id]
	if !ok || entry.key != key || time.Since(entry.at) > languageCacheTTL {
		return nil, false
	}
	return entry.names, true
}

func (c *workspaceCaches) storeLanguages(id, key string, names []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.languages[id] = languageEntry{key: key, names: names, at: time.Now()}
}

func (c *workspaceCaches) comparison(scanID, previousID string) (comparisonEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.comparisons[scanID+"|"+previousID]
	return entry, ok
}

func (c *workspaceCaches) storeComparison(scanID, previousID string, entry comparisonEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.comparisons[scanID+"|"+previousID] = entry
}

// invalidate drops everything cached for one workspace.
func (c *workspaceCaches) invalidate(workspaceID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.languages, workspaceID)
	for key, entry := range c.comparisons {
		if entry.workspaceID == workspaceID {
			delete(c.comparisons, key)
		}
	}
}

// workspaceLanguages returns the discovered language names for a workspace
// without a scan snapshot. With wait it walks synchronously on a miss (the
// single-workspace page); without it, a miss returns nil and warms the cache
// in the background (the list).
func (s *Server) workspaceLanguages(ctx context.Context, work core.Workspace, wait bool) []string {
	patterns, err := s.userExcludes(ctx, work.ID)
	if err != nil {
		return nil
	}
	key := languageKey(work.RootPath, patterns)
	if names, ok := s.cache.cachedLanguages(work.ID, key); ok {
		return names
	}
	if wait {
		discovered, err := discovery.Discover(ctx, work.RootPath, patterns)
		if err != nil {
			return nil
		}
		names := languageNames(discovered.Languages)
		s.cache.storeLanguages(work.ID, key, names)
		return names
	}
	s.cache.mu.Lock()
	if s.cache.warming[work.ID] {
		s.cache.mu.Unlock()
		return nil
	}
	s.cache.warming[work.ID] = true
	s.cache.mu.Unlock()
	go func() {
		defer func() {
			s.cache.mu.Lock()
			delete(s.cache.warming, work.ID)
			s.cache.mu.Unlock()
		}()
		s.cache.walk <- struct{}{}
		defer func() { <-s.cache.walk }()
		walkCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		if discovered, err := discovery.Discover(walkCtx, work.RootPath, patterns); err == nil {
			s.cache.storeLanguages(work.ID, key, languageNames(discovered.Languages))
		}
	}()
	return nil
}

// cachedComparisonCounts serves scanComparisonCounts from the cache for
// finished scans; running scans are always computed fresh.
func (s *Server) cachedComparisonCounts(ctx context.Context, scan core.Scan) (int, int, error) {
	if !terminalScanState(scan.State) {
		return s.scanComparisonCounts(ctx, scan)
	}
	previousID, err := s.db.PreviousCompletedScanID(ctx, scan.WorkspaceID, scan.ID)
	if err != nil {
		previousID = ""
	}
	if entry, ok := s.cache.comparison(scan.ID, previousID); ok {
		return entry.newCount, entry.fixedCount, nil
	}
	newCount, fixedCount, err := s.scanComparisonCounts(ctx, scan)
	if err != nil {
		return 0, 0, err
	}
	s.cache.storeComparison(scan.ID, previousID, comparisonEntry{workspaceID: scan.WorkspaceID, newCount: newCount, fixedCount: fixedCount})
	return newCount, fixedCount, nil
}
