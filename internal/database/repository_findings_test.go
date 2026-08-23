package database

import (
	"context"
	"path/filepath"
	"testing"

	"bluntcode/internal/analyzers"
	"bluntcode/internal/core"
)

// findingsFilterFixture is one workspace with a completed previous scan (the
// HIGH finding persists into the current scan) and a current scan whose nine
// findings cover every filter dimension: three analyzers, an empty rule id,
// path prefixes that must not cross-match (tests/src vs src), LIKE wildcard
// characters in stored paths, and every severity.
type findingsFilterFixture struct {
	db      *DB
	work    core.Workspace
	current core.Scan
	all     []analyzers.Finding
}

func seedFindingsFilterFixture(t *testing.T) findingsFilterFixture {
	t.Helper()
	db, err := Open(context.Background(), filepath.Join(t.TempDir(), "bluntcode.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()
	work, err := db.CreateWorkspace(ctx, core.Workspace{Name: "Sample", RootPath: "C:/sample"})
	if err != nil {
		t.Fatal(err)
	}
	mk := func(analyzer, rule, path, message string, severity analyzers.Severity, line int) analyzers.Finding {
		f := analyzers.Finding{AnalyzerID: analyzer, RuleID: rule, RelativePath: path, Message: message, Severity: severity, Category: analyzers.CategoryCorrectness, StartLine: line}
		f.SetFingerprint()
		return f
	}
	high := mk("ruff", "HIGH", "src/main.py", "persisting issue", analyzers.SeverityHigh, 10)
	all := []analyzers.Finding{
		mk("ruff", "CRIT", "src/main.py", "critical issue", analyzers.SeverityCritical, 40),
		high,
		mk("ruff", "MED", "src/util/helper.py", "medium issue", analyzers.SeverityMedium, 5),
		mk("biome", "LOW", "src/util/helper.py", "low issue", analyzers.SeverityLow, 30),
		mk("semgrep", "PCT", "100%.py", "percent path issue", analyzers.SeverityLow, 2),
		mk("biome", "INFO", "tests/src/edge.ts", "info issue", analyzers.SeverityInfo, 1),
		mk("ruff", "", "Docs/Readme.MD", "no rule issue", analyzers.SeverityInfo, 0),
		mk("ruff", "UND1", "src/a_b.py", "underscore one", analyzers.SeverityInfo, 7),
		mk("ruff", "UND2", "src/axb.py", "underscore two", analyzers.SeverityInfo, 3),
	}
	previous, err := db.CreateScan(ctx, core.Scan{WorkspaceID: work.ID, State: "queued"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.SaveAnalyzerResult(ctx, previous.ID, AnalyzerRunInput{AnalyzerID: "ruff", Version: "test", State: "succeeded"}, []analyzers.Finding{high}, nil); err != nil {
		t.Fatal(err)
	}
	if err := db.CompleteScan(ctx, previous.ID, "completed", ""); err != nil {
		t.Fatal(err)
	}
	current, err := db.CreateScan(ctx, core.Scan{WorkspaceID: work.ID, State: "queued"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.SaveAnalyzerResult(ctx, current.ID, AnalyzerRunInput{AnalyzerID: "mixed", Version: "test", State: "succeeded"}, all, nil); err != nil {
		t.Fatal(err)
	}
	// The critical finding is dismissed: it stays stored and visible on the
	// list with status suppressed, which the status filters must select.
	critical := all[0]
	if _, err := db.AddSuppression(ctx, work.ID, critical.Fingerprint, "wontfix"); err != nil {
		t.Fatal(err)
	}
	return findingsFilterFixture{db: db, work: work, current: current, all: all}
}

func rulesOf(items []analyzers.Finding) []string {
	rules := make([]string, 0, len(items))
	for _, item := range items {
		rules = append(rules, item.RuleID)
	}
	return rules
}

func equalRules(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestFindingsPageFiltersRuleAnalyzerAndPathPrefix(t *testing.T) {
	fixture := seedFindingsFilterFixture(t)
	ctx := context.Background()
	pageFor := func(t *testing.T, filter FindingFilter) FindingsPage {
		t.Helper()
		page, err := fixture.db.FindingsPage(ctx, fixture.current, filter)
		if err != nil {
			t.Fatalf("filter %#v: %v", filter, err)
		}
		return page
	}

	rule := pageFor(t, FindingFilter{Limit: 25, Rule: "high"})
	if rule.Total != 1 || !equalRules(rulesOf(rule.Items), []string{"HIGH"}) {
		t.Fatalf("rule must match exactly and case-insensitively: total=%d rules=%v", rule.Total, rulesOf(rule.Items))
	}
	// Exact match only: HIGH-ISH is a different rule id.
	if partial := pageFor(t, FindingFilter{Limit: 25, Rule: "HIG"}); partial.Total != 0 || len(partial.Items) != 0 {
		t.Fatalf("rule must be an exact match, not a prefix: %#v", partial.Items)
	}

	analyzer := pageFor(t, FindingFilter{Limit: 25, Analyzer: "biome"})
	if analyzer.Total != 2 || !equalRules(rulesOf(analyzer.Items), []string{"LOW", "INFO"}) {
		t.Fatalf("analyzer filter: total=%d rules=%v", analyzer.Total, rulesOf(analyzer.Items))
	}

	// Prefix match is anchored: "src" must not match "tests/src/edge.ts".
	src := pageFor(t, FindingFilter{Limit: 25, PathPrefix: "src"})
	if src.Total != 6 || !equalRules(rulesOf(src.Items), []string{"UND1", "UND2", "HIGH", "CRIT", "MED", "LOW"}) {
		t.Fatalf("path_prefix must anchor at the start: total=%d rules=%v", src.Total, rulesOf(src.Items))
	}
	// Backslash input normalizes to the same prefix.
	backslash := pageFor(t, FindingFilter{Limit: 25, PathPrefix: `src\util`})
	if backslash.Total != 2 || !equalRules(rulesOf(backslash.Items), []string{"MED", "LOW"}) {
		t.Fatalf("backslash prefix must normalize: total=%d rules=%v", backslash.Total, rulesOf(backslash.Items))
	}
	// Case-insensitive and "./"-tolerant, trailing slash is a boundary.
	upper := pageFor(t, FindingFilter{Limit: 25, PathPrefix: "./SRC/"})
	if upper.Total != 6 {
		t.Fatalf("case and ./ must normalize to the same set: total=%d rules=%v", upper.Total, rulesOf(upper.Items))
	}
	// LIKE wildcards in the stored path must be matched literally.
	underscore := pageFor(t, FindingFilter{Limit: 25, PathPrefix: "src/a_"})
	if underscore.Total != 1 || underscore.Items[0].RuleID != "UND1" {
		t.Fatalf("underscore must match literally, not as a wildcard: %#v", rulesOf(underscore.Items))
	}
	percent := pageFor(t, FindingFilter{Limit: 25, PathPrefix: "100%"})
	if percent.Total != 1 || percent.Items[0].RuleID != "PCT" {
		t.Fatalf("percent must match literally, not as a wildcard: %#v", rulesOf(percent.Items))
	}
	if edge := pageFor(t, FindingFilter{Limit: 25, PathPrefix: "edge"}); edge.Total != 0 {
		t.Fatalf("prefix must not behave as a substring: %#v", rulesOf(edge.Items))
	}

	// Combined filters intersect.
	combined := pageFor(t, FindingFilter{Limit: 25, Analyzer: "ruff", PathPrefix: "src/util", Rule: "MED"})
	if combined.Total != 1 || combined.Items[0].RuleID != "MED" {
		t.Fatalf("combined filters: total=%d rules=%v", combined.Total, rulesOf(combined.Items))
	}
}

func TestFindingsPageMultiValueSeverityAndStatus(t *testing.T) {
	fixture := seedFindingsFilterFixture(t)
	ctx := context.Background()
	pageFor := func(t *testing.T, filter FindingFilter) FindingsPage {
		t.Helper()
		page, err := fixture.db.FindingsPage(ctx, fixture.current, filter)
		if err != nil {
			t.Fatalf("filter %#v: %v", filter, err)
		}
		return page
	}

	both := pageFor(t, FindingFilter{Limit: 25, Severities: []string{"high", "critical"}})
	if both.Total != 2 || !equalRules(rulesOf(both.Items), []string{"HIGH", "CRIT"}) {
		t.Fatalf("severity list must select both values: total=%d rules=%v", both.Total, rulesOf(both.Items))
	}
	// A one-element list is the legacy single-value filter.
	single := pageFor(t, FindingFilter{Limit: 25, Severities: []string{"high"}})
	legacy := pageFor(t, FindingFilter{Limit: 25, Severity: "high"})
	if single.Total != 1 || legacy.Total != 1 || single.Items[0].ID != legacy.Items[0].ID {
		t.Fatalf("list and single-value severity must agree: %#v vs %#v", single.Items, legacy.Items)
	}
	if _, err := fixture.db.FindingsPage(ctx, fixture.current, FindingFilter{Limit: 25, Severities: []string{"high", "bogus"}}); err == nil {
		t.Fatal("an invalid severity in the list must be rejected")
	}

	// The fixture's statuses: CRIT suppressed, HIGH persistent, the rest new.
	newAndPersistent := pageFor(t, FindingFilter{Limit: 25, Statuses: []string{"new", "persistent"}})
	if newAndPersistent.Total != 8 || newAndPersistent.Items[0].Status == "suppressed" {
		t.Fatalf("status list must select new plus persistent without suppressed: total=%d rules=%v", newAndPersistent.Total, rulesOf(newAndPersistent.Items))
	}
	suppressed := pageFor(t, FindingFilter{Limit: 25, Statuses: []string{"suppressed"}})
	if suppressed.Total != 1 || suppressed.Items[0].RuleID != "CRIT" || suppressed.Items[0].Status != "suppressed" {
		t.Fatalf("suppressed selection: %#v", suppressed.Items)
	}
	if _, err := fixture.db.FindingsPage(ctx, fixture.current, FindingFilter{Limit: 25, Statuses: []string{"new", "nope"}}); err == nil {
		t.Fatal("an invalid status in the list must be rejected")
	}
	// A suppression-selecting list overrides the export-style exclusion.
	keeping := pageFor(t, FindingFilter{Limit: 25, Statuses: []string{"suppressed", "new"}, ExcludeSuppressed: true})
	sawSuppressed := false
	for _, item := range keeping.Items {
		if item.RuleID == "CRIT" && item.Status == "suppressed" {
			sawSuppressed = true
		}
	}
	if keeping.Total != 8 || !sawSuppressed {
		t.Fatalf("a list selecting suppressed must keep suppressed rows even with ExcludeSuppressed: total=%d rules=%v", keeping.Total, rulesOf(keeping.Items))
	}
}

// severityRank mirrors the domain order critical > high > medium > low > info
// that severityRankExpr bakes into SQL; the test sorts the seeded findings with
// it and demands the query agree.
func severityRank(severity analyzers.Severity) int {
	switch severity {
	case analyzers.SeverityCritical:
		return 5
	case analyzers.SeverityHigh:
		return 4
	case analyzers.SeverityMedium:
		return 3
	case analyzers.SeverityLow:
		return 2
	}
	return 1
}

func TestFindingsPageSortsByLineRuleAndSeverityRank(t *testing.T) {
	fixture := seedFindingsFilterFixture(t)
	ctx := context.Background()
	sorted := func(t *testing.T, filter FindingFilter) []analyzers.Finding {
		t.Helper()
		page, err := fixture.db.FindingsPage(ctx, fixture.current, filter)
		if err != nil {
			t.Fatalf("filter %#v: %v", filter, err)
		}
		if page.Total != len(fixture.all) {
			t.Fatalf("total=%d want %d", page.Total, len(fixture.all))
		}
		return page.Items
	}

	// line ascending follows start_line; the fixture's lines are unique.
	asc := sorted(t, FindingFilter{Limit: 25, Sort: "line"})
	wantLineAsc := []string{"", "INFO", "PCT", "UND2", "MED", "UND1", "HIGH", "LOW", "CRIT"}
	if !equalRules(rulesOf(asc), wantLineAsc) {
		t.Fatalf("sort=line asc: %v", rulesOf(asc))
	}
	desc := sorted(t, FindingFilter{Limit: 25, Sort: "line", Order: "desc"})
	if !equalRules(rulesOf(desc), reverseStrings(wantLineAsc)) {
		t.Fatalf("sort=line desc: %v", rulesOf(desc))
	}

	// rule ascending puts the empty rule id first (COALESCE ''), then binary.
	ruleAsc := sorted(t, FindingFilter{Limit: 25, Sort: "rule"})
	wantRuleAsc := []string{"", "CRIT", "HIGH", "INFO", "LOW", "MED", "PCT", "UND1", "UND2"}
	if !equalRules(rulesOf(ruleAsc), wantRuleAsc) {
		t.Fatalf("sort=rule asc: %v", rulesOf(ruleAsc))
	}
	ruleDesc := sorted(t, FindingFilter{Limit: 25, Sort: "rule", Order: "desc"})
	if !equalRules(rulesOf(ruleDesc), reverseStrings(wantRuleAsc)) {
		t.Fatalf("sort=rule desc: %v", rulesOf(ruleDesc))
	}

	// severity uses the domain order (critical=5..info=1), never alphabetical:
	// ascending starts at info and ends at critical.
	sevAsc := sorted(t, FindingFilter{Limit: 25, Sort: "severity", Order: "asc"})
	if got := severityRank(sevAsc[0].Severity); got != 1 {
		t.Fatalf("severity asc must start at the lowest rank, got %d (%s)", got, sevAsc[0].Severity)
	}
	if got := severityRank(sevAsc[len(sevAsc)-1].Severity); got != 5 {
		t.Fatalf("severity asc must end at critical, got %d (%s)", got, sevAsc[len(sevAsc)-1].Severity)
	}
	for i := 1; i < len(sevAsc); i++ {
		if severityRank(sevAsc[i].Severity) < severityRank(sevAsc[i-1].Severity) {
			t.Fatalf("severity asc must be non-decreasing by rank: %v", rulesOf(sevAsc))
		}
	}
	sevDesc := sorted(t, FindingFilter{Limit: 25, Sort: "severity", Order: "desc"})
	if sevDesc[0].Severity != analyzers.SeverityCritical || sevDesc[len(sevDesc)-1].Severity != analyzers.SeverityInfo {
		t.Fatalf("severity desc must run critical first, info last: %v", rulesOf(sevDesc))
	}

	// Unknown fields stay rejected.
	if _, err := fixture.db.FindingsPage(ctx, fixture.current, FindingFilter{Limit: 25, Sort: "bogus"}); err == nil {
		t.Fatal("unknown sort field must be rejected")
	}
}

func reverseStrings(values []string) []string {
	out := make([]string, len(values))
	for i, value := range values {
		out[len(values)-1-i] = value
	}
	return out
}

func TestFindingsPagePageModePagination(t *testing.T) {
	fixture := seedFindingsFilterFixture(t)
	ctx := context.Background()

	first, err := fixture.db.FindingsPage(ctx, fixture.current, FindingFilter{Page: 1, PageSize: 5})
	if err != nil {
		t.Fatal(err)
	}
	if first.Total != 9 || len(first.Items) != 5 || first.Limit != 5 || first.Offset != 0 || !first.HasMore || first.NextOffset == nil || *first.NextOffset != 5 {
		t.Fatalf("page 1: %#v", first)
	}
	// Nine rows of five: page 2 carries the four-row remainder.
	second, err := fixture.db.FindingsPage(ctx, fixture.current, FindingFilter{Page: 2, PageSize: 5})
	if err != nil {
		t.Fatal(err)
	}
	if second.Total != 9 || len(second.Items) != 4 || second.HasMore || second.NextOffset != nil {
		t.Fatalf("page 2 must hold the remainder without more pages: %#v", second)
	}
	// A page beyond the end is empty, not an error, and never claims more.
	beyond, err := fixture.db.FindingsPage(ctx, fixture.current, FindingFilter{Page: 3, PageSize: 5})
	if err != nil || beyond.Total != 9 || len(beyond.Items) != 0 || beyond.HasMore {
		t.Fatalf("page beyond the end: %#v (%v)", beyond, err)
	}
	// Page 2 must return exactly the legacy offset-5 window, in order.
	paged, err := fixture.db.FindingsPage(ctx, fixture.current, FindingFilter{Page: 2, PageSize: 5})
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := fixture.db.FindingsPage(ctx, fixture.current, FindingFilter{Limit: 25, Offset: 5})
	if err != nil {
		t.Fatal(err)
	}
	// The legacy window uses the default ordering; page mode shares it.
	if len(legacy.Items) < 4 || len(paged.Items) != 4 {
		t.Fatalf("unexpected window sizes: paged=%d legacy=%d", len(paged.Items), len(legacy.Items))
	}
	legacyWindow := legacy.Items[:4]
	for i := range paged.Items {
		if paged.Items[i].ID != legacyWindow[i].ID {
			t.Fatalf("page window diverges from the legacy offset window at %d: %v vs %v", i, rulesOf(paged.Items), rulesOf(legacyWindow))
		}
	}
	// Filters compose with pagination and keep the count exact.
	filtered, err := fixture.db.FindingsPage(ctx, fixture.current, FindingFilter{Page: 2, PageSize: 2, Severities: []string{"info"}})
	if err != nil || filtered.Total != 4 || len(filtered.Items) != 2 {
		t.Fatalf("filtered page mode must report the filtered total: %#v (%v)", filtered, err)
	}

	// Page size defaults are validated: PageSize alone starts at page 1.
	onlySize, err := fixture.db.FindingsPage(ctx, fixture.current, FindingFilter{PageSize: 200})
	if err != nil || onlySize.Limit != 200 || onlySize.Offset != 0 || len(onlySize.Items) != 9 {
		t.Fatalf("page size alone must page from the start: %#v (%v)", onlySize, err)
	}
	if _, err := fixture.db.FindingsPage(ctx, fixture.current, FindingFilter{Page: 1, PageSize: MaxFindingsPageSize + 1}); err == nil {
		t.Fatal("page_size above the cap must be rejected")
	}
	if _, err := fixture.db.FindingsPage(ctx, fixture.current, FindingFilter{Page: -1, PageSize: 5}); err == nil {
		t.Fatal("negative page must be rejected")
	}
	// The legacy limit whitelist is untouched by page mode.
	if _, err := fixture.db.FindingsPage(ctx, fixture.current, FindingFilter{Limit: 17}); err == nil {
		t.Fatal("legacy limit validation must still reject 17")
	}
}

func TestNormalizePathPrefix(t *testing.T) {
	cases := []struct{ in, want string }{
		{"src", "src"},
		{`src\util`, "src/util"},
		{`.\\src\util`, "src/util"},
		{"./src", "src"},
		{"/src", "src"},
		{"  src/util  ", "src/util"},
		{"src/", "src/"},
		{"", ""},
		{`\`, ""},
		{"./", ""},
	}
	for _, testCase := range cases {
		if got := normalizePathPrefix(testCase.in); got != testCase.want {
			t.Fatalf("normalizePathPrefix(%q) = %q, want %q", testCase.in, got, testCase.want)
		}
	}
}

func TestLikePatternPrefix(t *testing.T) {
	// likePatternPrefix receives an already-normalized prefix (forward slashes).
	cases := []struct{ in, want string }{
		{"src", "src%"},
		{"SRC", "src%"},
		{"src/a_b", `src/a\_b%`},
		{"100%", `100\%%`},
		{"c:/x", "c:/x%"},
	}
	for _, testCase := range cases {
		if got := likePatternPrefix(testCase.in); got != testCase.want {
			t.Fatalf("likePatternPrefix(%q) = %q, want %q", testCase.in, got, testCase.want)
		}
	}
}
