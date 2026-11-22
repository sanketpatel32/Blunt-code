package analyzers

import "testing"

// The shipped analyzer set. cmd/bluntcode/bootstrap.go registers exactly these
// adapters; this pin keeps the capability inventory from drifting when an
// analyzer is added or removed.
var shippedAnalyzerIDs = []string{
	"ruff", "biome", "gitleaks-secrets", "osv-dependencies", "container-trivy",
	"iac-checkov", "semgrep", "sonarqube", "pentest", "secrets", "todo", "license-scan",
	"dast-probe",
}

func TestCapabilityInventoryCoversShippedAnalyzers(t *testing.T) {
	if len(capabilityOrder) != len(shippedAnalyzerIDs) {
		t.Fatalf("capability inventory has %d entries, want the %d shipped analyzers", len(capabilityOrder), len(shippedAnalyzerIDs))
	}
	for _, id := range shippedAnalyzerIDs {
		cap, ok := CapabilityFor(id)
		if !ok {
			t.Errorf("shipped analyzer %q has no capability entry", id)
			continue
		}
		if cap.ID != id {
			t.Errorf("capability entry keyed %q carries id %q", id, cap.ID)
		}
		if cap.DisplayName == "" || cap.Category == "" || cap.Description == "" {
			t.Errorf("capability %q is missing display metadata", id)
		}
		if cap.Execution != ExecutionExternal && cap.Execution != ExecutionInProcess && cap.Execution != ExecutionManagedServer {
			t.Errorf("capability %q has unknown execution kind %q", id, cap.Execution)
		}
		if len(cap.InputKinds) == 0 {
			t.Errorf("capability %q declares no input kinds", id)
		}
		if len(cap.Profiles) == 0 {
			t.Errorf("capability %q declares no profiles", id)
		}
		if (cap.ManagedTool != "") != (cap.Execution != ExecutionInProcess) {
			t.Errorf("capability %q: managed tool %q disagrees with execution kind %q", id, cap.ManagedTool, cap.Execution)
		}
	}
	// No extra entries beyond the shipped set.
	seen := map[string]bool{}
	for _, id := range capabilityOrder {
		if seen[id] {
			t.Errorf("capability %q listed twice in capabilityOrder", id)
		}
		seen[id] = true
	}
	for id := range capabilityTable {
		if !seen[id] {
			t.Errorf("capability %q is in the table but not in capabilityOrder", id)
		}
	}
}

// The profile gating encoded in the inventory must reproduce the historical
// orchestrator behavior exactly: quick runs only ruff and biome; the three
// advisory-database analyzers run on deep scans only; everything else runs on
// every non-quick tier (including pentest-profile scans).
func TestProfileAllowsMatchesHistoricalGating(t *testing.T) {
	cases := []struct {
		analyzer string
		quick    bool
		standard bool
		deep     bool
		pentest  bool
	}{
		{"ruff", true, true, true, true},
		{"biome", true, true, true, true},
		{"gitleaks-secrets", false, true, true, true},
		{"osv-dependencies", false, false, true, true},
		{"container-trivy", false, false, true, true},
		{"iac-checkov", false, false, true, true},
		{"semgrep", false, true, true, true},
		{"sonarqube", false, true, true, true},
		{"pentest", false, true, true, true},
		{"secrets", false, true, true, true},
		{"todo", false, true, true, true},
		{"license-scan", false, true, true, true},
		{"dast-probe", false, false, false, true},
		// Unknown fixture adapters keep the historical default.
		{"alpha-fixture", false, true, true, true},
	}
	for _, tc := range cases {
		for profile, want := range map[string]bool{ProfileQuick: tc.quick, ProfileStandard: tc.standard, ProfileDeep: tc.deep, ProfilePentest: tc.pentest} {
			if got := ProfileAllows(profile, tc.analyzer); got != want {
				t.Errorf("ProfileAllows(%s, %s) = %v, want %v", profile, tc.analyzer, got, want)
			}
		}
		// The empty profile must behave like standard everywhere.
		if got := ProfileAllows("", tc.analyzer); got != tc.standard {
			t.Errorf("ProfileAllows(\"\", %s) = %v, want %v (standard behavior)", tc.analyzer, got, tc.standard)
		}
	}
}

func TestInstallableAndArtifactPolicy(t *testing.T) {
	for _, id := range []string{"ruff", "biome", "gitleaks-secrets", "osv-dependencies", "container-trivy", "iac-checkov", "semgrep", "sonarqube"} {
		if !InstallableTool(id) {
			t.Errorf("managed tool analyzer %q should be installable", id)
		}
	}
	for _, id := range []string{"pentest", "secrets", "todo", "license-scan", "dast-probe", "unknown"} {
		if InstallableTool(id) {
			t.Errorf("in-process or unknown analyzer %q must not be installable", id)
		}
	}
	for _, id := range []string{"secrets", "gitleaks-secrets"} {
		if !KeepsArtifactFindings(id) {
			t.Errorf("secret detector %q must keep artifact findings", id)
		}
	}
	for _, id := range []string{"ruff", "semgrep", "license-scan", "unknown"} {
		if KeepsArtifactFindings(id) {
			t.Errorf("analyzer %q must not keep artifact findings", id)
		}
	}
	if TimeoutClassFor("sonarqube") != TimeoutSonar {
		t.Error("sonarqube must use the sonar timeout class")
	}
	for _, id := range []string{"ruff", "semgrep", "unknown"} {
		if TimeoutClassFor(id) != TimeoutFast {
			t.Errorf("analyzer %q must use the fast timeout class", id)
		}
	}
}

// The pentest tier is the security superset: every analyzer deep runs, plus
// the target-driven DAST probe. It must never again run fewer analyzers than
// deep while advertising more (issue #24).
func TestPentestProfileIsDeepPlusDAST(t *testing.T) {
	for _, id := range capabilityOrder {
		if ProfileAllows(ProfileDeep, id) && !ProfileAllows(ProfilePentest, id) {
			t.Errorf("deep runs %q but pentest does not", id)
		}
	}
	if !ProfileAllows(ProfilePentest, "dast-probe") || ProfileAllows(ProfileDeep, "dast-probe") {
		t.Error("dast-probe must be pentest-only")
	}
	if !RequiresTarget("dast-probe") {
		t.Error("dast-probe must require a scan target")
	}
	for _, id := range shippedAnalyzerIDs {
		if id != "dast-probe" && RequiresTarget(id) {
			t.Errorf("%q is file-driven and must not require a target", id)
		}
	}
}

// Offline mode keeps every analyzer that never dials out and withholds
// unconditional registration only from outbound ones (issue #23).
func TestAllowedOfflineFollowsNetworkUse(t *testing.T) {
	for _, id := range shippedAnalyzerIDs {
		cap, _ := CapabilityFor(id)
		if got, want := AllowedOffline(id), cap.Network != NetworkOutbound; got != want {
			t.Errorf("AllowedOffline(%q) = %v, want %v (network %q)", id, got, want, cap.Network)
		}
	}
	for _, id := range []string{"pentest", "secrets", "todo", "license-scan", "ruff", "semgrep", "sonarqube"} {
		if !AllowedOffline(id) {
			t.Errorf("local-only analyzer %q must be allowed offline", id)
		}
	}
	for _, id := range []string{"osv-dependencies", "container-trivy", "dast-probe", "unknown"} {
		if AllowedOffline(id) {
			t.Errorf("network-dependent or unknown analyzer %q must not be allowed offline unconditionally", id)
		}
	}
}
