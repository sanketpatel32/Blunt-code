package main

// `bluntcode scan --watch` turns the one-shot CI scan into a live loop: it
// runs the normal scan once, then keeps watching the workspace and rescans
// automatically when the files a scan would select change. Change detection is
// plain polling — no filesystem-notification dependency — so the loop rewalks
// the workspace exactly the way discovery does every watchPollInterval and
// fires a rescan once the workspace has stayed unchanged for watchQuietWindow.
//
// The pieces in this file are deliberately small and pure so the policy is
// unit-testable without real sleeping: buildWatchSnapshot/diffWatchSnapshots
// fingerprint and compare the workspace, watchDebouncer is a two-state machine
// over (now, changed) inputs, and runWatchLoop is thin glue over those plus
// the per-scan runner from scan.go.

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"

	"bluntcode/internal/discovery"
)

// Watch-mode timing constants. The poll interval is how often the workspace is
// rewalked for changes; the quiet window is how long it must stay unchanged
// before the pending rescan fires (deliberately shorter than the poll interval
// so a single clean poll after a burst of saves is enough). Both are injectable
// through watchLoopOptions so tests can compress them.
const (
	watchPollInterval = 2 * time.Second
	watchQuietWindow  = 1500 * time.Millisecond
)

// watchFileStat fingerprints one file for change detection.
type watchFileStat struct {
	Size    int64
	ModTime time.Time
}

// watchSnapshot maps the workspace-relative slash paths of every file
// discovery would select to its fingerprint. Keys and values are plain data so
// snapshots can be built, diffed, and inspected in tests without a filesystem.
type watchSnapshot map[string]watchFileStat

// buildWatchSnapshot walks root exactly as a scan would — discovery's walk
// with its default excludes, the workspace's .bluntcodeignore, and the given
// user exclude patterns — and fingerprints the selected files by size and
// modification time. Files that vanish between the walk and the stat are
// skipped; the next diff sees them as removed.
func buildWatchSnapshot(ctx context.Context, root string, userExcludes []string) (watchSnapshot, error) {
	result, err := discovery.Discover(ctx, root, userExcludes)
	if err != nil {
		return nil, err
	}
	snapshot := make(watchSnapshot, len(result.Files))
	for _, file := range result.Files {
		info, err := os.Stat(filepath.Join(root, filepath.FromSlash(file.RelativePath)))
		if err != nil {
			continue
		}
		snapshot[file.RelativePath] = watchFileStat{Size: info.Size(), ModTime: info.ModTime()}
	}
	return snapshot, nil
}

// watchDiff is the set of changes between two snapshots; every list is sorted
// so messages and tests stay deterministic.
type watchDiff struct {
	Added   []string
	Removed []string
	Changed []string
}

// Count is the total number of added, removed, and changed files.
func (d watchDiff) Count() int { return len(d.Added) + len(d.Removed) + len(d.Changed) }

// Empty reports whether the two snapshots were identical.
func (d watchDiff) Empty() bool { return d.Count() == 0 }

// diffWatchSnapshots returns what changed between two snapshots: paths only in
// after are added, paths only in before are removed, and paths present in both
// with a different size or modification time are changed (a bare touch counts).
func diffWatchSnapshots(before, after watchSnapshot) watchDiff {
	var diff watchDiff
	for path, stat := range after {
		previous, existed := before[path]
		switch {
		case !existed:
			diff.Added = append(diff.Added, path)
		case previous.Size != stat.Size || !previous.ModTime.Equal(stat.ModTime):
			diff.Changed = append(diff.Changed, path)
		}
	}
	for path := range before {
		if _, stillPresent := after[path]; !stillPresent {
			diff.Removed = append(diff.Removed, path)
		}
	}
	sort.Strings(diff.Added)
	sort.Strings(diff.Removed)
	sort.Strings(diff.Changed)
	return diff
}

// watchDebouncer decides when a dirty workspace has gone quiet: it arms on the
// first change, every further change pushes the fire moment out, and it fires
// once a full quiet window passes with no further changes. It is a pure state
// machine over (now, changed) inputs, so tests drive it with synthetic clocks.
type watchDebouncer struct {
	quietWindow time.Duration
	armed       bool
	lastChange  time.Time
}

// observe records one poll result taken at now and reports whether the quiet
// window has elapsed and a rescan should fire.
func (d *watchDebouncer) observe(now time.Time, changed bool) bool {
	if changed {
		d.armed = true
		d.lastChange = now
		return false
	}
	if d.armed && now.Sub(d.lastChange) >= d.quietWindow {
		d.armed = false
		return true
	}
	return false
}

// reset disarms the debouncer without firing; starting a rescan supersedes any
// pending debounce because the scan itself observes the current file state.
func (d *watchDebouncer) reset() { d.armed = false }

// watchLoopOptions carries the watch loop's timing constants; production uses
// the package defaults, tests compress them.
type watchLoopOptions struct {
	pollInterval time.Duration
	quietWindow  time.Duration
}

// watchEnv bundles the watch loop's injectable dependencies: where progress
// goes, the interrupt channel, how the workspace snapshot is taken, and how
// one scan is executed. runScan receives the interrupt channel so an in-flight
// scan handles Ctrl+C exactly like the one-shot command (first press cancels
// it, second exits immediately); the loop itself consumes interrupts only
// while idle, so exactly one party selects on the channel at any moment.
type watchEnv struct {
	quiet     bool
	stderr    io.Writer
	interrupt <-chan os.Signal
	snapshot  func() (watchSnapshot, error)
	runScan   func(interrupts <-chan os.Signal) scanRunResult
}

// runWatchLoop runs scans back to back: one scan immediately, then a rescan
// every time the workspace changes and settles. While a scan runs, polls keep
// collecting changes and queue exactly one rescan for when it finishes. The
// loop only returns on Ctrl+C (exit 130, including the in-flight scan's
// interrupt outcomes); scan failures and snapshot errors are printed and
// watched through, and gate results from env.runScan never terminate it.
func runWatchLoop(env watchEnv, opts watchLoopOptions) int {
	if !env.quiet {
		fmt.Fprintf(env.stderr, "watching for changes (poll every %s, rescan after %s of quiet); Ctrl+C stops\n", opts.pollInterval, opts.quietWindow)
	}
	base, err := env.snapshot()
	if err != nil {
		fmt.Fprintf(env.stderr, "bluntcode scan: could not scan workspace: %v\n", err)
		return 1
	}
	last := base
	ticker := time.NewTicker(opts.pollInterval)
	defer ticker.Stop()
	deb := watchDebouncer{quietWindow: opts.quietWindow}
	var (
		scanning          bool
		done              chan scanRunResult
		changedDuringScan bool
	)
	// startScan launches one scan in the background so polling keeps running
	// while analyzers work; the buffered channel lets the goroutine complete
	// even if the loop returns without reading the result.
	startScan := func() {
		scanning = true
		outcome := make(chan scanRunResult, 1)
		done = outcome
		go func() { outcome <- env.runScan(env.interrupt) }()
	}
	// fire starts a rescan: it reports how many files changed since the last
	// scan started, rebases both snapshots on the current file state (so the
	// next rescan counts from this one), supersedes any pending debounce, and
	// launches the scan. A snapshot failure just reports and leaves the loop
	// idle; the next ticks re-detect the pending changes and try again.
	fire := func() {
		current, err := env.snapshot()
		if err != nil {
			fmt.Fprintf(env.stderr, "bluntcode scan: could not scan workspace: %v\n", err)
			return
		}
		total := diffWatchSnapshots(base, current)
		if !env.quiet {
			fmt.Fprintf(env.stderr, "rescan: %d file(s) changed\n", total.Count())
		}
		base, last = current, current
		deb.reset()
		changedDuringScan = false
		startScan()
	}
	startScan() // the first scan runs immediately, before any change detection
	for {
		if scanning {
			select {
			case <-ticker.C:
				current, err := env.snapshot()
				if err != nil {
					fmt.Fprintf(env.stderr, "bluntcode scan: could not scan workspace: %v\n", err)
					continue
				}
				// Keep collecting changes while the scan runs; exactly one
				// rescan is queued no matter how many ticks report changes.
				if !diffWatchSnapshots(last, current).Empty() {
					changedDuringScan = true
				}
				last = current
			case result := <-done:
				scanning = false
				if result.exitNow || result.interruptSeen {
					return 130
				}
				if changedDuringScan {
					changedDuringScan = false
					fire()
					continue
				}
				// Going idle: refresh the poll baseline so changes that landed
				// since the last tick are still caught, and arm the debouncer
				// if there were any.
				if current, err := env.snapshot(); err == nil {
					if !diffWatchSnapshots(last, current).Empty() {
						deb.observe(time.Now(), true)
					}
					last = current
				}
			}
			continue
		}
		select {
		case <-ticker.C:
			current, err := env.snapshot()
			if err != nil {
				fmt.Fprintf(env.stderr, "bluntcode scan: could not scan workspace: %v\n", err)
				continue
			}
			changed := !diffWatchSnapshots(last, current).Empty()
			last = current
			if deb.observe(time.Now(), changed) {
				fire()
			}
		case <-env.interrupt:
			// Between scans a single Ctrl+C stops the loop at once; anything
			// pressed during a scan was handled by the scan itself.
			return 130
		}
	}
}
