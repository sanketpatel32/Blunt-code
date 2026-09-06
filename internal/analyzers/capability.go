package analyzers

// Capability is the single machine-readable description of one analyzer: what
// inputs it analyzes, how it executes, which scan tiers include it, what it
// needs from the environment, and how orchestration must treat it (install
// eligibility, artifact-finding policy, timeout class). The table below is the
// one place analyzer capability knowledge lives; orchestration, the HTTP API,
// the Tools page, and the docs should read it (directly or through the
// /api/v1/analyzers projection) instead of keeping their own ID-string
// conditionals, which had already drifted apart (11 vs 12 counts, phantom
// profile claims, hardcoded install allowlists).

// InputKind names the class of workspace inputs an analyzer consumes. Discovery
// routes files by language for source analyzers and tracks the non-source
// classes separately (see discovery.Result input lists).
type InputKind string

const (
	InputSource       InputKind = "source"
	InputDependencies InputKind = "dependencies"
	InputIaC          InputKind = "iac"
	InputLicenseFiles InputKind = "license-files"
)

// ExecutionKind describes how an adapter runs.
type ExecutionKind string

const (
	// ExecutionExternal runs a pinned managed binary as a child process.
	ExecutionExternal ExecutionKind = "external"
	// ExecutionInProcess analyzes files inside the Blunt Code process.
	ExecutionInProcess ExecutionKind = "in-process"
	// ExecutionManagedServer starts and stops a private local server (the
	// managed SonarQube JVM) and drives it over loopback HTTP.
	ExecutionManagedServer ExecutionKind = "managed-server"
)

// NetworkUse documents the network behavior of a normal scan.
type NetworkUse string

const (
	// NetworkNone means a scan runs without any outbound network access.
	NetworkNone NetworkUse = "none"
	// NetworkOutbound means the analyzer may contact an external service or
	// download data while scanning; offline mode must fail it honestly instead
	// of letting it dial out.
	NetworkOutbound NetworkUse = "outbound"
	// NetworkLoopbackOnly means the only sockets are to a local server the
	// analyzer itself manages.
	NetworkLoopbackOnly NetworkUse = "loopback-only"
)

// TimeoutClass routes the per-analyzer deadline (see scans.analyzerTimeout):
// "fast" gets the standard budget, "sonar" the extended one.
const (
	TimeoutFast  = "fast"
	TimeoutSonar = "sonar"
)

type Capability struct {
	ID          string        `json:"id"`
	DisplayName string        `json:"display_name"`
	Category    string        `json:"category"`
	Execution   ExecutionKind `json:"execution"`
	// Profiles lists every scan tier that includes this analyzer, in the
	// canonical order quick, standard, deep, pentest.
	Profiles   []string    `json:"profiles"`
	InputKinds []InputKind `json:"input_kinds"`
	// ManagedTool is the tools-manifest id of the pinned artifacts this adapter
	// executes; empty for in-process analyzers. It is also the auto-install
	// eligibility switch: only managed tools may be installed mid-scan.
	ManagedTool string `json:"managed_tool,omitempty"`
	// Network documents network use during a normal scan (see NetworkUse).
	Network NetworkUse `json:"network"`
	// NetworkNote explains Network in one sentence for the UI and docs.
	NetworkNote string `json:"network_note,omitempty"`
	// KeepArtifactFindings keeps findings inside generated artifacts visible
	// (the secret detectors: a credential in shipped output is a real leak).
	KeepArtifactFindings bool `json:"keep_artifact_findings"`
	// TimeoutClass routes the per-analyzer deadline.
	TimeoutClass string `json:"timeout_class"`
	Description  string `json:"description"`
}

// capabilityOrder is the presentation order (registry order).
var capabilityOrder = []string{
	"ruff", "biome", "gitleaks-secrets", "osv-dependencies", "container-trivy",
	"iac-checkov", "semgrep", "sonarqube", "pentest", "secrets", "todo", "license-scan",
}

var capabilityTable = map[string]Capability{
	"ruff": {
		ID: "ruff", DisplayName: "Ruff", Category: "code-quality",
		Execution: ExecutionExternal, Profiles: []string{ProfileQuick, ProfileStandard, ProfileDeep, ProfilePentest},
		InputKinds: []InputKind{InputSource}, ManagedTool: "ruff",
		Network: NetworkNone, TimeoutClass: TimeoutFast,
		Description: "Python linter and fast static analysis; deep scans widen its rule selection.",
	},
	"biome": {
		ID: "biome", DisplayName: "Biome", Category: "code-quality",
		Execution: ExecutionExternal, Profiles: []string{ProfileQuick, ProfileStandard, ProfileDeep, ProfilePentest},
		InputKinds: []InputKind{InputSource}, ManagedTool: "biome",
		Network: NetworkNone, TimeoutClass: TimeoutFast,
		Description: "JavaScript and TypeScript linter with an injected React domain when the workspace has no own config.",
	},
	"gitleaks-secrets": {
		ID: "gitleaks-secrets", DisplayName: "Gitleaks", Category: "secrets",
		Execution: ExecutionExternal, Profiles: []string{ProfileStandard, ProfileDeep, ProfilePentest},
		InputKinds: []InputKind{InputSource}, ManagedTool: "gitleaks-secrets",
		Network: NetworkNone, KeepArtifactFindings: true, TimeoutClass: TimeoutFast,
		Description: "Committed-credential detector that walks the whole workspace on disk (no git history; --no-git).",
	},
	"osv-dependencies": {
		ID: "osv-dependencies", DisplayName: "OSV Scanner", Category: "dependencies",
		Execution: ExecutionExternal, Profiles: []string{ProfileDeep},
		InputKinds: []InputKind{InputDependencies}, ManagedTool: "osv-dependencies",
		Network:     NetworkOutbound,
		NetworkNote: "Queries the OSV.dev API at scan time unless offline vulnerability databases are provisioned (offline mode fails readiness instead of dialing out).",
		TimeoutClass: TimeoutFast,
		Description: "Dependency vulnerability scanner over lockfiles and manifests; deep scans only.",
	},
	"container-trivy": {
		ID: "container-trivy", DisplayName: "Trivy", Category: "dependencies",
		Execution: ExecutionExternal, Profiles: []string{ProfileDeep},
		InputKinds: []InputKind{InputDependencies, InputIaC}, ManagedTool: "container-trivy",
		Network:     NetworkOutbound,
		NetworkNote: "Downloads its vulnerability database on a cold cache at scan time; warm caches and offline mode scan locally (offline mode fails readiness when the database was never provisioned).",
		TimeoutClass: TimeoutFast,
		Description: "Filesystem scanner for dependency vulnerabilities, secrets, and misconfigurations; deep scans only.",
	},
	"iac-checkov": {
		ID: "iac-checkov", DisplayName: "Checkov", Category: "infrastructure",
		Execution: ExecutionExternal, Profiles: []string{ProfileDeep},
		InputKinds: []InputKind{InputIaC}, ManagedTool: "iac-checkov",
		Network: NetworkNone, TimeoutClass: TimeoutFast,
		Description: "Infrastructure-as-code policy checks (Terraform, Dockerfile, Kubernetes, CloudFormation); deep scans only.",
	},
	"semgrep": {
		ID: "semgrep", DisplayName: "Semgrep", Category: "security",
		Execution: ExecutionExternal, Profiles: []string{ProfileStandard, ProfileDeep, ProfilePentest},
		InputKinds: []InputKind{InputSource}, ManagedTool: "semgrep",
		Network: NetworkNone, TimeoutClass: TimeoutFast,
		Description: "Pattern-based security scanner running the bundled local rulepack; no rule downloads, metrics off.",
	},
	"sonarqube": {
		ID: "sonarqube", DisplayName: "SonarQube", Category: "code-quality",
		Execution: ExecutionManagedServer, Profiles: []string{ProfileStandard, ProfileDeep, ProfilePentest},
		InputKinds: []InputKind{InputSource}, ManagedTool: "sonarqube",
		Network: NetworkLoopbackOnly,
		NetworkNote: "Talks to the locally managed SonarQube server it starts on a dynamic loopback port; never an external host.",
		TimeoutClass: TimeoutSonar,
		Description: "Managed SonarQube Community Build with a private scanner over Python, JavaScript, and TypeScript.",
	},
	"pentest": {
		ID: "pentest", DisplayName: "Pentest & Vulnerability Suite", Category: "security",
		Execution: ExecutionInProcess, Profiles: []string{ProfileStandard, ProfileDeep, ProfilePentest},
		InputKinds: []InputKind{InputSource},
		Network: NetworkNone, TimeoutClass: TimeoutFast,
		Description: "Static pass for exploitable patterns (injection sinks, weak crypto, debug endpoints). Dynamic probes are a separate, explicitly targeted operation.",
	},
	"secrets": {
		ID: "secrets", DisplayName: "Secrets Detector", Category: "secrets",
		Execution: ExecutionInProcess, Profiles: []string{ProfileStandard, ProfileDeep, ProfilePentest},
		InputKinds: []InputKind{InputSource},
		Network: NetworkNone, KeepArtifactFindings: true, TimeoutClass: TimeoutFast,
		Description: "Built-in credential detector over the selected files (40+ languages) with entropy floors and placeholder rejection.",
	},
	"todo": {
		ID: "todo", DisplayName: "TODO Comment Tracker", Category: "maintainability",
		Execution: ExecutionInProcess, Profiles: []string{ProfileStandard, ProfileDeep, ProfilePentest},
		InputKinds: []InputKind{InputSource},
		Network: NetworkNone, TimeoutClass: TimeoutFast,
		Description: "Counts TODO/FIXME debt with owner and ticket attribution from comments.",
	},
	"license-scan": {
		ID: "license-scan", DisplayName: "License Scanner", Category: "compliance",
		Execution: ExecutionInProcess, Profiles: []string{ProfileStandard, ProfileDeep, ProfilePentest},
		InputKinds: []InputKind{InputLicenseFiles, InputDependencies},
		Network: NetworkNone, TimeoutClass: TimeoutFast,
		Description: "License identification from license files and package manifests (SPDX normalization, policy flags).",
	},
}

// CapabilityFor returns the inventory entry for an analyzer id.
func CapabilityFor(id string) (Capability, bool) {
	cap, ok := capabilityTable[id]
	return cap, ok
}

// Capabilities returns the full inventory in presentation order.
func Capabilities() []Capability {
	out := make([]Capability, 0, len(capabilityOrder))
	for _, id := range capabilityOrder {
		out = append(out, capabilityTable[id])
	}
	return out
}

// ProfileAllows reports whether a scan tier includes an analyzer. It is the
// single source for profile gating; the empty profile behaves like standard.
// Unknown ids keep the historical default (everything except quick, where only
// the two language linters run) so test fixtures and future adapters are not
// silently disabled.
func ProfileAllows(profile, analyzerID string) bool {
	if profile == "" {
		profile = ProfileStandard
	}
	if cap, ok := capabilityTable[analyzerID]; ok {
		for _, p := range cap.Profiles {
			if p == profile {
				return true
			}
		}
		return false
	}
	return profile != ProfileQuick || analyzerID == "ruff" || analyzerID == "biome"
}

// InstallableTool reports whether a missing analyzer may be installed mid-scan
// from the pinned manifest. Only managed external tools qualify.
func InstallableTool(analyzerID string) bool {
	cap, ok := capabilityTable[analyzerID]
	return ok && cap.ManagedTool != ""
}

// KeepsArtifactFindings reports whether an analyzer's findings inside
// generated artifacts stay visible (secret detectors only).
func KeepsArtifactFindings(analyzerID string) bool {
	if cap, ok := capabilityTable[analyzerID]; ok {
		return cap.KeepArtifactFindings
	}
	return false
}

// TakesDependencyInputs reports whether the analyzer consumes dependency
// manifests/lockfiles as an input class. The scan orchestrator uses it to
// keep such adapters eligible for workspaces whose only dependency signal
// is a lockfile (smart-skipped out of the per-file selection, so no source
// language route exists).
func TakesDependencyInputs(analyzerID string) bool {
	if cap, ok := capabilityTable[analyzerID]; ok {
		for _, kind := range cap.InputKinds {
			if kind == InputDependencies {
				return true
			}
		}
	}
	return false
}

// TimeoutClassFor routes the per-analyzer deadline budget.
func TimeoutClassFor(analyzerID string) string {
	if cap, ok := capabilityTable[analyzerID]; ok && cap.TimeoutClass != "" {
		return cap.TimeoutClass
	}
	return TimeoutFast
}
