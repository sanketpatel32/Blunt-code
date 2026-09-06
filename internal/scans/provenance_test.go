package scans

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"bluntcode/internal/analyzers"
	"bluntcode/internal/core"
)

// mutatingAnalyzer routes to Python files like the blocking one, but its Run
// edits a selected workspace file mid-scan: the drift-detection fixture.
type mutatingAnalyzer struct {
	root   string
	rel    string
	mutate func()
}

func (mutatingAnalyzer) ID() string          { return "mutator" }
func (mutatingAnalyzer) DisplayName() string { return "Mutator" }
func (mutatingAnalyzer) SupportedLanguages() []analyzers.Language {
	return []analyzers.Language{analyzers.LanguagePython}
}
func (mutatingAnalyzer) Check(context.Context, analyzers.ToolEnvironment) analyzers.ToolStatus {
	return analyzers.ToolStatus{Ready: true, Version: "test"}
}
func (mutatingAnalyzer) EnsureInstalled(context.Context, analyzers.ToolEnvironment) error {
	return nil
}
func (mutatingAnalyzer) Plan(context.Context, analyzers.ScanRequest) (analyzers.AnalyzerPlan, error) {
	return analyzers.AnalyzerPlan{AnalyzerID: "mutator", Version: "test"}, nil
}
func (a mutatingAnalyzer) Run(context.Context, analyzers.AnalyzerPlan, analyzers.EventEmitter) (analyzers.AnalyzerResult, error) {
	if a.mutate != nil {
		a.mutate()
	}
	return analyzers.AnalyzerResult{}, nil
}
func (mutatingAnalyzer) Normalize(context.Context, analyzers.AnalyzerResult) ([]analyzers.Finding, []analyzers.Metric, error) {
	return nil, nil, nil
}

func TestInputDigestStableAndSensitive(t *testing.T) {
	base := map[string]string{"a.py": "hash-a", "b.py": "hash-b"}
	if inputDigest(base) != inputDigest(map[string]string{"b.py": "hash-b", "a.py": "hash-a"}) {
		t.Fatal("input digest must be order-independent")
	}
	if inputDigest(base) == inputDigest(map[string]string{"a.py": "hash-a"}) {
		t.Fatal("removing a file must change the input digest")
	}
	if inputDigest(base) == inputDigest(map[string]string{"a.py": "hash-a", "b.py": "hash-changed"}) {
		t.Fatal("changing content must change the input digest")
	}
	if inputDigest(base) == inputDigest(map[string]string{"c.py": "hash-a", "b.py": "hash-b"}) {
		t.Fatal("renaming a file must change the input digest")
	}
	if inputDigest(nil) != "" {
		t.Fatal("an empty input set digests to the empty string")
	}
}

func TestConfigDigestCanonicalizesOrder(t *testing.T) {
	rules := []core.WorkspaceRule{{ID: "1", RuleType: "exclude", Pattern: "dist/**", Enabled: true}, {ID: "2", RuleType: "exclude", Pattern: "vendor/**", Enabled: true}}
	shuffled := []core.WorkspaceRule{rules[1], rules[0]}
	exclusions := []string{"one", "two"}
	shuffledExclusions := []string{"two", "one"}
	if configDigest(rules, exclusions, nil) != configDigest(shuffled, shuffledExclusions, nil) {
		t.Fatal("semantically identical configurations must digest equally")
	}
	if configDigest(rules, exclusions, nil) == configDigest(rules, append(exclusions, "three"), nil) {
		t.Fatal("an added exclusion must change the config digest")
	}
}

// TestScanRecordsProvenance pins the run manifest: a completed scan's
// snapshot carries the input digest, config digest, schema versions, and
// platform — the identity two scans need before they are comparable.
func TestScanRecordsProvenance(t *testing.T) {
	db, scan := runOutcomeScan(t, fakeAnalyzer{})
	if scan.Snapshot == nil {
		t.Fatal("scan has no snapshot")
	}
	snapshot := scan.Snapshot
	if snapshot.InputDigest == "" {
		t.Fatal("snapshot records no input digest")
	}
	if snapshot.ConfigDigest == "" {
		t.Fatal("snapshot records no config digest")
	}
	if snapshot.DiscoveryPolicyVersion < 1 {
		t.Fatalf("snapshot records discovery policy version %d", snapshot.DiscoveryPolicyVersion)
	}
	if snapshot.FingerprintVersion < 1 {
		t.Fatalf("snapshot records fingerprint version %d", snapshot.FingerprintVersion)
	}
	if snapshot.Platform["os"] == "" || snapshot.Platform["arch"] == "" {
		t.Fatalf("snapshot records no platform: %v", snapshot.Platform)
	}
	if snapshot.DriftDetected {
		t.Fatal("an untouched workspace must not be flagged as drifted")
	}
	// The persisted snapshot (what the API and reports read) matches.
	persisted, err := db.Scan(context.Background(), scan.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Snapshot == nil || persisted.Snapshot.InputDigest != snapshot.InputDigest {
		t.Fatalf("persisted snapshot lost the input digest: %+v", persisted.Snapshot)
	}
}

// TestDriftDuringScanRelabels is the IMP-07 acceptance test: editing a file
// mid-scan cannot silently produce a result labeled reproducible for
// different bytes — the scan relabels to completed_with_warnings, the
// snapshot records drift, and the note says why.
func TestDriftDuringScanRelabels(t *testing.T) {
	var workspaceRoot string
	mutator := mutatingAnalyzer{mutate: func() {
		if workspaceRoot == "" {
			return
		}
		if err := os.WriteFile(filepath.Join(workspaceRoot, "main.py"), []byte("x=2"), 0o600); err != nil {
			t.Log(err)
		}
	}}
	db, service, work := newSupervisionService(t, mutator)
	workspaceRoot = work.RootPath
	scan, err := service.DiscoverAndStart(context.Background(), work, "standard", nil)
	if err != nil {
		t.Fatal(err)
	}
	final := waitForTerminalState(t, db, scan.ID)
	if final.State != "completed_with_warnings" {
		t.Fatalf("drifted scan state %q, want completed_with_warnings", final.State)
	}
	if final.Snapshot == nil || !final.Snapshot.DriftDetected {
		t.Fatal("drift was not recorded on the snapshot")
	}
	if final.ErrorSummary == "" {
		t.Fatal("drifted scan carries no note")
	}
}
