package scans

import (
	"context"
	"testing"

	"bluntcode/internal/analyzers"
)

func finding(id, fingerprint string) analyzers.Finding {
	return analyzers.Finding{AnalyzerID: id, Fingerprint: fingerprint}
}

// duplicateAnalyzer reports the same identity twice from one run, the way a
// real analyzer does when the same rule fires on repeated identical lines.
type duplicateAnalyzer struct{}

func (duplicateAnalyzer) ID() string          { return "dupe" }
func (duplicateAnalyzer) DisplayName() string { return "Dupe" }
func (duplicateAnalyzer) SupportedLanguages() []analyzers.Language {
	return []analyzers.Language{analyzers.LanguagePython}
}
func (duplicateAnalyzer) Check(context.Context, analyzers.ToolEnvironment) analyzers.ToolStatus {
	return analyzers.ToolStatus{Ready: true, Version: "test"}
}
func (duplicateAnalyzer) EnsureInstalled(context.Context, analyzers.ToolEnvironment) error {
	return nil
}
func (duplicateAnalyzer) Plan(context.Context, analyzers.ScanRequest) (analyzers.AnalyzerPlan, error) {
	return analyzers.AnalyzerPlan{AnalyzerID: "dupe", Version: "test"}, nil
}
func (duplicateAnalyzer) Run(context.Context, analyzers.AnalyzerPlan, analyzers.EventEmitter) (analyzers.AnalyzerResult, error) {
	return analyzers.AnalyzerResult{}, nil
}
func (duplicateAnalyzer) Normalize(context.Context, analyzers.AnalyzerResult) ([]analyzers.Finding, []analyzers.Metric, error) {
	mk := func(line int) analyzers.Finding {
		f := analyzers.Finding{AnalyzerID: "dupe", RuleID: "D1", Severity: analyzers.SeverityLow, Category: analyzers.CategoryStyle, Message: "same issue", RelativePath: "main.py", StartLine: line}
		f.SetFingerprint()
		return f
	}
	return []analyzers.Finding{mk(1), mk(1), mk(2)}, nil, nil
}

// TestDuplicateFindingsPersistWithDistinctFingerprints pins the persistence
// contract of fingerprint V2: three occurrences of one identity survive as
// three rows (none collapsed), all with distinct fingerprints, and the
// first-by-position occurrence keeps the V1 base identity that suppressions
// recorded before V2 still match.
func TestDuplicateFindingsPersistWithDistinctFingerprints(t *testing.T) {
	db, service, work := newSupervisionService(t, duplicateAnalyzer{})
	scan, err := service.DiscoverAndStart(context.Background(), work, "standard", nil)
	if err != nil {
		t.Fatal(err)
	}
	final := waitForTerminalState(t, db, scan.ID)
	if final.State != "completed" {
		t.Fatalf("scan state %q", final.State)
	}
	rows, err := db.Findings(context.Background(), scan.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("want 3 persisted rows for 3 occurrences, got %d (%#v)", len(rows), rows)
	}
	base := analyzers.Finding{AnalyzerID: "dupe", RuleID: "D1", Message: "same issue", RelativePath: "main.py"}
	base.SetFingerprint()
	seen := map[string]bool{}
	var hasBase bool
	for _, row := range rows {
		if seen[row.Fingerprint] {
			t.Fatalf("two rows share fingerprint %s", row.Fingerprint)
		}
		seen[row.Fingerprint] = true
		if row.Fingerprint == base.Fingerprint {
			hasBase = true
		}
	}
	if !hasBase {
		t.Fatal("no row carries the V1 base fingerprint; old suppressions would no longer match")
	}
}

func TestComparisonDoesNotMarkFailedAnalyzerFindingFixed(t *testing.T) {
	result := Compare([]analyzers.Finding{finding("ruff", "same"), finding("ruff", "new")}, []analyzers.Finding{finding("ruff", "same"), finding("sonarqube", "sonar-old")}, NewComparisonCoverage(map[string]bool{"ruff": true}, nil))
	if len(result.New) != 1 || len(result.Persistent) != 1 || len(result.Fixed) != 0 || len(result.NotEvaluatedAnalyzerIDs) != 1 || result.NotEvaluatedAnalyzerIDs[0] != "sonarqube" {
		t.Fatalf("unexpected comparison %#v", result)
	}
}

// The not-evaluated list is derived from the previous findings themselves, not
// a hardcoded analyzer roster: any analyzer id (here "osv") must appear.
func TestComparisonNotEvaluatedListIsDynamic(t *testing.T) {
	result := Compare(nil, []analyzers.Finding{finding("osv", "a"), finding("checkov", "b")}, NewComparisonCoverage(map[string]bool{}, nil))
	if len(result.Fixed) != 0 {
		t.Fatalf("nothing was evaluated, nothing can be fixed: %#v", result)
	}
	if len(result.NotEvaluatedAnalyzerIDs) != 2 || result.NotEvaluatedAnalyzerIDs[0] != "checkov" || result.NotEvaluatedAnalyzerIDs[1] != "osv" {
		t.Fatalf("not-evaluated list must be sorted and dynamic: %v", result.NotEvaluatedAnalyzerIDs)
	}
}

// A previous finding whose file was not part of the current scan (excluded by
// rule, deselected, or filtered as an artifact) must not be counted fixed even
// though its analyzer completed successfully.
func TestComparisonOutOfScopeFileIsNotFixed(t *testing.T) {
	previous := analyzers.Finding{AnalyzerID: "ruff", Fingerprint: "old", RelativePath: "src\\skipped.py"}
	result := Compare(nil, []analyzers.Finding{previous}, NewComparisonCoverage(map[string]bool{"ruff": true}, []string{"src/kept.py"}))
	if len(result.Fixed) != 0 {
		t.Fatalf("out-of-scope file laundered as fixed: %#v", result)
	}
	if len(result.NotEvaluatedAnalyzerIDs) != 1 || result.NotEvaluatedAnalyzerIDs[0] != "ruff" {
		t.Fatalf("ruff must be listed as not fully evaluated: %v", result.NotEvaluatedAnalyzerIDs)
	}
}

// The same finding with its file in scope IS fixed — path separators are
// normalized on both sides before the lookup.
func TestComparisonInScopeFileIsFixed(t *testing.T) {
	previous := analyzers.Finding{AnalyzerID: "ruff", Fingerprint: "old", RelativePath: "src\\skipped.py"}
	result := Compare(nil, []analyzers.Finding{previous}, NewComparisonCoverage(map[string]bool{"ruff": true}, []string{"src\\skipped.py"}))
	if len(result.Fixed) != 1 || len(result.NotEvaluatedAnalyzerIDs) != 0 {
		t.Fatalf("in-scope previous finding should be fixed: %#v", result)
	}
}

// Project-level findings (no file) follow their analyzer only: a successful
// analyzer re-evaluated the whole project, so its project findings can fix.
func TestComparisonProjectLevelFindingFollowsAnalyzer(t *testing.T) {
	previous := analyzers.Finding{AnalyzerID: "osv", Fingerprint: "CVE-1", RelativePath: ""}
	result := Compare(nil, []analyzers.Finding{previous}, NewComparisonCoverage(map[string]bool{"osv": true}, []string{"pkg/go.mod"}))
	if len(result.Fixed) != 1 || len(result.NotEvaluatedAnalyzerIDs) != 0 {
		t.Fatalf("project-level finding from a succeeded analyzer should be fixed: %#v", result)
	}
}

// An empty selected-path list must mean "no file-level information" (legacy
// scans), never "every file was out of scope".
func TestComparisonEmptySelectedListKeepsAnalyzerOnlyContract(t *testing.T) {
	previous := analyzers.Finding{AnalyzerID: "ruff", Fingerprint: "old", RelativePath: "a.py"}
	result := Compare(nil, []analyzers.Finding{previous}, NewComparisonCoverage(map[string]bool{"ruff": true}, nil))
	if len(result.Fixed) != 1 {
		t.Fatalf("nil evaluated-files must keep the analyzer-only contract: %#v", result)
	}
}
