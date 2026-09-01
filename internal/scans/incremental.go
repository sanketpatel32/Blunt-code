package scans

// Incremental rescans: when a scan opts in (the CLI's --incremental flag, or
// the second and later scans of a --watch loop), the previous completed scan's
// per-file content hashes decide which files still need analysis. Analyzers
// receive only the changed and added files; findings for unchanged files are
// copied forward from the previous scan with their fingerprints intact, so
// suppression and the new/fixed/persistent comparison keep working unchanged.
//
// The equivalence invariant is the contract: an incremental scan over an
// unchanged workspace must produce the same fingerprints and totals as a full
// scan. Anything that jeopardizes it - no previous scan, a previous scan
// without hashes, a different analyzer set (id, version, profile, or Blunt
// Code build), or any lookup error - silently degrades to a full scan.

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	"bluntcode/internal/analyzers"
	"bluntcode/internal/build"
	"bluntcode/internal/core"
	"bluntcode/internal/database"
)

// bluntCodeVersion mirrors the version stamped into scan snapshots; it is part
// of the analyzer identity so a new build (with potentially changed
// normalization) invalidates reuse.
//
// Keep in lockstep with cmd/bluntcode/main.go's version const: it had drifted
// at "0.5.0" through 0.16.0, letting incremental findings survive version
// upgrades that should have invalidated reuse and mislabeling reports.
const bluntCodeVersion = build.Version

// scanHashIdentity is the analyzer configuration a set of file hashes was
// produced under. Any difference between two scans' identities - a changed
// analyzer version, a different profile (deep widens ruff's rule set), or a
// new Blunt Code build - means the previous findings may not reproduce, so
// reuse is refused and every file re-runs.
type scanHashIdentity struct {
	BluntCodeVersion string            `json:"bluntcode_version"`
	Profile          string            `json:"profile"`
	Analyzers        map[string]string `json:"analyzers"`
}

// incrementalState is the prepared plan of one incremental scan. active is
// false (nil state) whenever reuse degraded to a full scan.
type incrementalState struct {
	previousID string
	// unchanged holds the normalized relative paths of selected files whose
	// (path, content hash) matches the previous completed scan.
	unchanged map[string]bool
	// changedByLanguage is the per-language map of files analyzers receive:
	// added or modified files only, in discovery order like scanInputs.
	changedByLanguage map[analyzers.Language][]string
	// changedCount is the number of added plus modified files.
	changedCount int
	// reusedFindings maps analyzer id to the previous scan's findings on
	// unchanged files, and projectFindings maps analyzer id to the previous
	// scan's path-less findings. Project-level findings cannot be attributed
	// to a file, so they are only copied for analyzers that did not re-run
	// at all; for an analyzer that ran, its fresh output is authoritative.
	reusedFindings  map[string][]analyzers.Finding
	projectFindings map[string][]analyzers.Finding
	// reusedMetrics maps analyzer id to the previous scan's metrics; like
	// project findings they are copied only for fully-reused analyzers,
	// because a partial run's metrics describe just the changed subset.
	reusedMetrics map[string][]analyzers.Metric
	// identityAnalyzers lists, in registry order, the analyzers selected for
	// this scan (profile-allowed with matching files) - the set whose results
	// must exist for the scan to complete.
	identityAnalyzers []string
	// changedByAnalyzer caches which analyzers have at least one changed file,
	// so a skipped analyzer can be told apart from a failed one.
	changedForAnalyzer map[string]bool
}

// normalizeReusePath canonicalizes a workspace-relative path for reuse
// matching: slash separators, no leading "./", and "" for path-less
// (project-level) rows, mirroring the fingerprint normalizer in SetFingerprint.
func normalizeReusePath(path string) string {
	if path == "" {
		return ""
	}
	path = filepath.ToSlash(filepath.Clean(path))
	for len(path) > 2 && path[:2] == "./" {
		path = path[2:]
	}
	if path == "." {
		return ""
	}
	return path
}

// hashFileContent streams the file through sha256 and returns the hex digest.
// Discovery imposes no size limit on selected files, so neither does hashing;
// streaming keeps memory flat for large inputs.
func hashFileContent(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	digest := sha256.New()
	if _, err := io.Copy(digest, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

// hashSelectedFiles hashes every selected, language-classified file (the same
// set scanInputs feeds to analyzers). A file that cannot be read hashes to ""
// and therefore always counts as changed - the analyzers will fail on it too,
// which is the honest outcome.
func hashSelectedFiles(root string, files []core.FileEntry) map[string]string {
	hashes := make(map[string]string, len(files))
	for _, file := range files {
		if !file.Selected || file.Language == "" {
			continue
		}
		hash, err := hashFileContent(filepath.Join(root, filepath.FromSlash(file.RelativePath)))
		if err != nil {
			log.Printf("incremental: hashing %s failed: %v", file.RelativePath, err)
			continue
		}
		hashes[normalizeReusePath(file.RelativePath)] = hash
	}
	return hashes
}

// deepOnlyAnalyzerIDs lists analyzers that query external advisory databases
// or image registries and therefore run only on deep scans; standard scans
// record them as skipped instead of running a half-capability pass.
var deepOnlyAnalyzerIDs = map[string]bool{
	"osv-dependencies": true,
	"container-trivy":  true,
	// iac-checkov joins this set when it registers.
}

// profileAllowsAnalyzer mirrors the profile gating in executeAnalyzer: the
// quick tier runs only the language-specific analyzers, and deep-only
// analyzers wait for the deep tier.
func profileAllowsAnalyzer(profile, analyzerID string) bool {
	if deepOnlyAnalyzerIDs[analyzerID] {
		return profile == analyzers.ProfileDeep
	}
	return profile != analyzers.ProfileQuick || analyzerID == "ruff" || analyzerID == "biome"
}

// selectedAnalyzerIdentity computes the analyzer identity for this scan: the
// profile-allowed analyzers that have at least one matching selected file,
// each with the version the scan snapshot probed at start. ok is false when a
// version is unknown or no analyzer is selected; the caller then skips hash
// persistence (and reuse) rather than recording an identity it cannot stand
// behind.
func (s *Service) selectedAnalyzerIdentity(scan core.Scan, filesByLanguage map[analyzers.Language][]string) (scanHashIdentity, bool) {
	versions := map[string]string{}
	if scan.Snapshot != nil {
		versions = scan.Snapshot.AnalyzerVersions
	}
	identity := scanHashIdentity{BluntCodeVersion: bluntCodeVersion, Profile: scan.Profile, Analyzers: map[string]string{}}
	for _, adapter := range s.registry.All() {
		if !profileAllowsAnalyzer(scan.Profile, adapter.ID()) {
			continue
		}
		if len(filesForLanguages(filesByLanguage, adapter.SupportedLanguages()...)) == 0 {
			continue
		}
		version, known := versions[adapter.ID()]
		if !known || version == "" {
			return scanHashIdentity{}, false
		}
		identity.Analyzers[adapter.ID()] = version
	}
	if len(identity.Analyzers) == 0 {
		return scanHashIdentity{}, false
	}
	return identity, true
}

func (s *Service) analyzerIdentityJSON(scan core.Scan, filesByLanguage map[analyzers.Language][]string) (string, bool) {
	identity, ok := s.selectedAnalyzerIdentity(scan, filesByLanguage)
	if !ok {
		return "", false
	}
	data, err := json.Marshal(identity)
	if err != nil {
		return "", false
	}
	return string(data), true
}

// prepareIncremental decides whether this scan can reuse the previous
// completed scan's findings and, if so, returns the prepared state. Every
// degradation path (no previous scan, no stored hashes, a different analyzer
// identity, any lookup error) logs the reason and returns nil so the caller
// runs a full scan.
func (s *Service) prepareIncremental(ctx context.Context, scan core.Scan, work core.Workspace, files []core.FileEntry, fileHashes map[string]string, filesByLanguage map[analyzers.Language][]string) *incrementalState {
	previousID, err := s.db.PreviousCompletedScanID(ctx, work.ID, scan.ID)
	if errors.Is(err, sql.ErrNoRows) {
		log.Printf("incremental: no previous completed scan for workspace %s; running a full scan", work.ID)
		return nil
	}
	if err != nil {
		log.Printf("incremental: could not resolve the previous completed scan (%v); running a full scan", err)
		return nil
	}
	storedIdentity, err := s.db.ScanHashAnalyzerSet(ctx, previousID)
	if errors.Is(err, sql.ErrNoRows) {
		log.Printf("incremental: previous scan %s recorded no file hashes; running a full scan", previousID)
		return nil
	}
	if err != nil {
		log.Printf("incremental: could not load the previous scan's analyzer identity (%v); running a full scan", err)
		return nil
	}
	identity, ok := s.selectedAnalyzerIdentity(scan, filesByLanguage)
	if !ok {
		log.Printf("incremental: analyzer identity for this scan is unknown; running a full scan")
		return nil
	}
	current, err := json.Marshal(identity)
	if err != nil {
		log.Printf("incremental: could not encode the analyzer identity (%v); running a full scan", err)
		return nil
	}
	if string(current) != storedIdentity {
		log.Printf("incremental: analyzer set changed since scan %s; running a full scan", previousID)
		return nil
	}
	previousHashes, err := s.db.ScanFileHashes(ctx, previousID)
	if err != nil {
		log.Printf("incremental: could not load the previous scan's file hashes (%v); running a full scan", err)
		return nil
	}

	state := &incrementalState{
		previousID:         previousID,
		unchanged:          map[string]bool{},
		changedByLanguage:  map[analyzers.Language][]string{},
		reusedFindings:     map[string][]analyzers.Finding{},
		projectFindings:    map[string][]analyzers.Finding{},
		reusedMetrics:      map[string][]analyzers.Metric{},
		changedForAnalyzer: map[string]bool{},
	}
	for _, adapter := range s.registry.All() {
		if _, selected := identity.Analyzers[adapter.ID()]; selected {
			state.identityAnalyzers = append(state.identityAnalyzers, adapter.ID())
		}
	}
	for _, file := range files {
		if !file.Selected || file.Language == "" {
			continue
		}
		normalized := normalizeReusePath(file.RelativePath)
		hash, hashed := fileHashes[normalized]
		if hashed && previousHashes[normalized] == hash {
			state.unchanged[normalized] = true
			continue
		}
		// Added, modified, or unreadable: the analyzers must see it.
		state.changedCount++
		state.changedByLanguage[analyzers.Language(file.Language)] = append(state.changedByLanguage[analyzers.Language(file.Language)], filepath.Join(work.RootPath, file.RelativePath))
	}
	for _, adapter := range s.registry.All() {
		if len(filesForLanguages(state.changedByLanguage, adapter.SupportedLanguages()...)) > 0 {
			state.changedForAnalyzer[adapter.ID()] = true
		}
	}

	previousFindings, err := s.db.Findings(ctx, previousID)
	if err != nil {
		log.Printf("incremental: could not load the previous scan's findings (%v); running a full scan", err)
		return nil
	}
	for _, finding := range previousFindings {
		switch {
		case state.unchanged[normalizeReusePath(finding.RelativePath)]:
			state.reusedFindings[finding.AnalyzerID] = append(state.reusedFindings[finding.AnalyzerID], finding)
		case normalizeReusePath(finding.RelativePath) == "":
			state.projectFindings[finding.AnalyzerID] = append(state.projectFindings[finding.AnalyzerID], finding)
		}
	}
	previousMetrics, err := s.db.Metrics(ctx, previousID)
	if err != nil {
		log.Printf("incremental: could not load the previous scan's metrics (%v); metrics will not be reused", err)
	} else {
		for _, metric := range previousMetrics {
			state.reusedMetrics[metric.AnalyzerID] = append(state.reusedMetrics[metric.AnalyzerID], metric)
		}
	}
	return state
}

// finishIncremental runs after the analyzers finished on the changed-file
// subset and copies the previous scan's findings for unchanged files into the
// new scan:
//
//   - an analyzer that ran and succeeded gets its reused findings appended to
//     the real run (finding counts updated; runs stay honest);
//   - an analyzer with zero changed files never re-ran, so its entire result
//     set (including path-less project findings and metrics) is copied forward
//     under a zero-duration succeeded run - the run row is what keeps the
//     comparison coverage rule identical to a full scan;
//   - an analyzer that ran and failed contributes nothing, exactly like in a
//     full scan.
//
// Fully-reused analyzers are marked succeeded so an unchanged workspace still
// completes (a full scan of it would too).
func (s *Service) finishIncremental(scan core.Scan, state *incrementalState, counters *scanCounters) {
	for _, analyzerID := range state.identityAnalyzers {
		reused := state.reusedFindings[analyzerID]
		if counters.succeeded(analyzerID) {
			if len(reused) == 0 {
				continue
			}
			if err := s.db.AppendReusedFindings(context.Background(), scan.ID, analyzerID, reused); err != nil {
				log.Printf("incremental: could not copy reused findings from analyzer %s (%v)", analyzerID, err)
				continue
			}
			s.emit(scan.ID, "analyzer.completed", map[string]any{"analyzer_id": analyzerID, "findings": len(reused), "reused": true})
			continue
		}
		if state.changedForAnalyzer[analyzerID] {
			// The analyzer had changed files and still did not succeed: its
			// result set is not authoritative, so nothing is reused.
			continue
		}
		reused = append(append([]analyzers.Finding(nil), reused...), state.projectFindings[analyzerID]...)
		version := ""
		if scan.Snapshot != nil {
			version = scan.Snapshot.AnalyzerVersions[analyzerID]
		}
		note := fmt.Sprintf("findings reused from previous scan %s (%d unchanged file(s); analyzer did not re-run)", state.previousID, len(state.unchanged))
		if _, err := s.db.SaveAnalyzerResult(context.Background(), scan.ID, analyzerRunInputForReuse(analyzerID, version, note), reused, state.reusedMetrics[analyzerID]); err != nil {
			log.Printf("incremental: could not record reused results for analyzer %s (%v)", analyzerID, err)
			continue
		}
		counters.markSucceeded(analyzerID)
		s.emit(scan.ID, "analyzer.completed", map[string]any{"analyzer_id": analyzerID, "findings": len(reused), "reused": true})
	}
}

// analyzerRunInputForReuse shapes the run row for a fully-reused analyzer: a
// succeeded zero-duration run whose error_summary note explains where the
// findings came from (renderers surface it as the run's detail column).
func analyzerRunInputForReuse(analyzerID, version, note string) database.AnalyzerRunInput {
	now := time.Now()
	return database.AnalyzerRunInput{AnalyzerID: analyzerID, Version: version, State: "succeeded", StartedAt: now, FinishedAt: now, Error: note}
}
