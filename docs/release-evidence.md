# Release evidence

This document maps every P0 acceptance case from the improvement plan (IMP-01 through IMP-06) to the evidence that verifies it, per the IMP-15 requirement: *every P0 acceptance case is automated or has reproducible manual evidence before release*. It also inventories the fake-analyzer scenario matrix, golden parser fixtures, server/CLI equivalence evidence, packaging smoke procedure, and the items that remain open.

Status legend:

- **Automated** — verified by a named in-tree test (`go test ./...` / `vitest run`); run locally, the repo has no CI.
- **Partial** — the property is enforced for a representative slice, not exhaustively; the untested remainder is named.
- **Manual procedure** — reproducible steps recorded here, not automated.
- **Open** — no evidence yet; treated as not done.

## IMP-01 — capability and public-contract inventory

| Acceptance case | Status | Evidence |
|---|---|---|
| Analyzer count, displayed count, profile membership agree | Partial | `internal/analyzers/capability_test.go`: `TestCapabilityInventoryCoversShippedAnalyzers` (pins the 12 shipped IDs), `TestProfileAllowsMatchesHistoricalGating` (full quick/standard/deep/pentest matrix), `TestInstallableAndArtifactPolicy`. Served count: `internal/api/analyzers_test.go` `TestListAnalyzersServesCapabilityInventory` (12 rows incl. unregistered pentest), `TestListAnalyzersWithoutScanService` (503 fail-closed). **Open:** docs-vs-inventory agreement (llms.txt / CLI manual claims) has no automated check; verified manually at release (see manual procedures). |
| Every documented CLI command and format parses in a contract test | Automated | `cmd/bluntcode/cli_subcommands_test.go`: `TestCLIDocs` (all 13 documented commands render; unknown command → exit 2), `TestCommandHelpOutputs`, `TestCLISubcommandsIntegration`; `cmd/bluntcode/help_test.go` `TestPrintHelpListsEverySubcommand`. Formats: `TestParseScanFlagsFormatFlag`, `TestParseScanFlagsRejectsBadFormatInput`, `TestRunScanCommandRejectsBadFormatFlagWithExitCodeTwo`, `TestWriteScanJSONShape` (scan_test.go), `TestWriteScanJSONAllTerminalStates` (scan_edge_test.go), `TestConfigReportJSONShapeIsStable` (config_test.go). |
| Clean-machine check honest; no silent runtime assumption | Automated | `cmd/bluntcode/scan_edge_test.go` `TestRunScanCommandOfflineMissingToolsFailsHonestly` (exit 3, per-analyzer "offline mode" errors); `internal/tools/upgrade_safety_test.go` `TestOfflineMissingToolFailsWithoutNetwork` (zero network contacts); `internal/doctor/doctor_test.go` `TestRunChecksLocalFoundationsAndReportsMissingToolsAsWarnings`, `TestRunFailsWhenDataDirectoryIsNotConfigured`. |

## IMP-02 — analyzer-aware discovery

| Acceptance case | Status | Evidence |
|---|---|---|
| Mixed fixture routes per input class to the right analyzers | Partial | Source/lockfile/IaC/license routing: `internal/scans/service_test.go` `TestAdaptersReceiveOnlyFilesForTheirLanguages`; `internal/scans/routing_test.go` `TestDependencyAnalyzerRunsOnLockfileOnlyWorkspace`; `internal/discovery/inputs_test.go` `TestDiscoverTracksDependencyInputs`, `TestLanguageTerraformAndLicenseBasenames`; `internal/analyzers/routing_test.go`; checkov `TestPlanRequiresDeepProfile`/`TestPlanScansWorkspaceDirectoryOnce`; license `TestPlanRequiresRootOrManifests`/`TestNestedLicenseFilesAreRead`. **Open:** git-history as a distinct input class is not distinguished in a test (gitleaks operates on the working tree via discovery today; no analyzer enumerates `.git`). |
| 128 KiB/500-char and 10 MiB limits don't apply to unrelated categories | Automated | `internal/discovery/artifacts_test.go` `TestLooksMinified`, `TestDiscoverSkipsGeneratedContent` (10 MiB refusal), `TestIsArtifactPathMatrix` (lockfiles explicitly NOT artifacts); `internal/scans/service_test.go` `TestScanDropsFindingsInArtifactPaths`, `TestKeepsArtifactFindings` (lockfile + secret findings survive); `TestInstallableAndArtifactPolicy`; `TestDiscoverTracksDependencyInputs` (lockfile excluded from Files yet still a dependency input). |
| Exclusions visible with reasons; unavailable coverage ≠ clean scan | Automated | `internal/discovery/inputs_test.go` `TestDiscoverSkipCountsByReason` (per-reason buckets sum to Skipped); `internal/scans/service_test.go` `TestDiscoverAndStartAppliesBluntCodeIgnore` (snapshot records exclusions), `TestScanDropsFindingsInExcludedPaths`; `internal/discovery/discovery_test.go` `TestDiscoverExcludesStillApplyToNewFileTypes`; honesty: `internal/scans/outcomes_test.go` `TestWarnedRunCompletesWithWarnings`, `TestOnlyWarnedRunStillCompletesWithWarnings`. |

## IMP-03 — completeness vs severity

| Acceptance case | Status | Evidence |
|---|---|---|
| Missing analyzer / timeout / invalid JSON / inaccessible input / DB failure / unwritable output never exit 0 | Automated | Exit contract: `cmd/bluntcode/scan_test.go` `TestScanExitCode` (completed→0; warnings/failed/interrupted/timed-out→3; cancelled→4), `TestRunScanCommandRejectsBadFlagsWithExitCodeTwo`; `cmd/bluntcode/scan_edge_test.go` `TestRunScanCommandOfflineMissingToolsFailsHonestly`, `TestRunScanCommandInvalidPathsAreClean`, `TestRunScanCommandInstanceLockContended`, `TestAwaitScanTerminalTimeoutCancels`, `TestAwaitScanTerminalTimeoutGraceExpiry`; baseline: `TestRunScanCommandBaselineErrorsExitTwo`; interrupt 130: `cmd/bluntcode/watch_test.go` `TestWatchLoopInterruptWhileIdleExits130`. Invalid analyzer output: checkov `TestRunRejectsUnexpectedPlanShapes`/`TestNormalizeRejectsFatalExitCode`; secrets/todo `TestNormalizeEmptyStdoutAndBadExit`. DB failure and unwritable output: `internal/scans/service_test.go` `TestTerminalEventEmittedWhenPersistenceFails`, `TestUnwritableReportDirWithFailedAnalyzersStaysFailed`, `TestUnwritableReportDirStillCompletesScan` (service level; the CLI exit path over these states is covered by `TestScanExitCode` mapping failed→3). |
| Zero-finding scan differs visibly from empty selection / zero completed analyzers | Automated | `cmd/bluntcode/scan_edge_test.go` `TestWriteScanJSONAllTerminalStates`, `TestWriteScanJSONEmptyRunListAndZeroTimes`; `TestWriteScanHumanFailed` ("No supported source files were selected."); empty-selection refusals: ruff `TestPlanRejectsSelectionWithoutPythonFiles`, biome `TestPlanRejectsSelectionWithoutWebFiles`, gitleaks `TestPlanRejectsUnscannableSelections`, secrets/todo `TestPlanRejectsEmptySelection`. |
| `--baseline` / `--fail-on` / `--max-findings` share rules across outputs | Partial | One evaluator: `internal/scans/gate_test.go` `TestEvaluateGate`, `TestParseSeveritySpec*`, `TestGateConfigEnabled`; `internal/scans/baseline_test.go` `TestBaselineGateCountsOnlyNewFindings`, `TestBaselineFromSARIFRoundTrip`; suppression-aware: `TestEvaluateGateSkipsSuppressedFindings`; flags: `TestParseScanFlagsGateFlags`, `TestParseScanFlagsRejectsBadGateInput`; `cmd/bluntcode/scan_gate_test.go` `TestScopeGateFindingsFiltersByAnalyzerAndCategory`. `EvaluateGate` is called from the CLI path; the API/report surfaces present the stored gate rather than re-evaluating, so cross-output equivalence rests on the single evaluator plus stored-decision presentation. |

## IMP-04 — network-policy boundary

| Acceptance case | Status | Evidence |
|---|---|---|
| Outbound-denied environment: scans/doctor attempt no outbound calls | Partial | Per-component no-network assertions: `internal/tools/upgrade_safety_test.go` `TestOfflineMissingToolFailsWithoutNetwork` (`contacted` flag stays false); semgrep `TestPlanUsesOnlyLocalRulesAndDisablesSemgrepNetworkSignals`; osv `TestPlanOfflineModeAndManagedCache`; trivy `TestPlanOfflineRefusesColdCache`; `internal/api/update_test.go` `TestUpdateCheckUpToDateAndOffline`, `TestUpdateApplyBlockedWhenOffline`. **Open:** no whole-scan-under-denied-network assertion (the offline CLI test asserts honest failure, not network silence). |
| Missing/policy-stale vuln data warns or fails; no implicit download; no clean bill | Partial | osv `TestCheckOfflineRequiresDatabases`; trivy `TestPlanOfflineRefusesColdCache`; rule staleness: semgrep `TestCheckRejectsStaleRulesAndAcceptsCurrentPack`, `internal/tools/service_test.go` `TestEnsureSemgrepRestoresBundledRulesOffline`, `internal/doctor/fix_test.go` `TestFixReextractsStaleSemgrepRules`; degraded coverage honesty: `internal/scans/outcomes_test.go` warned-run tests. **Open:** no age/freshness-window test for OSV/Trivy data identity. |
| Pentest requests/redirects can't leave approved scope; cancellation stops requests | Partial | `internal/api/pentest_test.go` `TestPentestProbeRefusesCrossHostRedirect` (redirect target never contacted; surfaced as `dast.redirect.cross-host`; enforced in `internal/api/pentest.go`). **Open:** no test that cancellation halts follow-up probe requests. |

## IMP-05 — loopback API and workspace access

| Acceptance case | Status | Evidence |
|---|---|---|
| Unapproved origin cannot start scans / install tools / read source / modify settings | Partial | Cross-origin mutation rejection + loopback Host enforcement apply middleware-wide to every route: `internal/api/server_test.go` `TestRejectsCrossOriginPost` (foreign Origin POST → 403), `TestRejectsNonLoopbackHost` (→421); `internal/api/staticguard_test.go` `TestStaticGuardBlocksNonLoopbackHosts`; `internal/api/ratelimit_test.go` `TestRateLimitCoversEveryStateChangingMethod`. **By design** (documented boundary): only state-changing methods carry the Origin check (`internal/api/server.go` security middleware); same-user local processes and origin-less GET reads are out of scope. |
| Traversal / workspace-escape links / crafted report paths contained | Automated | `internal/api/server_test.go` `TestRejectsTraversalInTree`, `TestFindingPreviewReturnsContainedSourceExcerpt`, `TestFindingPreviewHandlesDeletedAndMissingPaths`; `internal/workspace/path_test.go` `TestValidateRelativePathRejectsTraversal`, `TestValidateRelativePathResolvesJunctionEscapes`; `internal/analyzers/guard_test.go` `TestRelativeInsideContainment`, `TestRelativeInsideResolvesJunctionEscapes`, `TestFilesForLanguagesInsideCombinesRoutingAndContainment`; crafted inputs: `internal/reports/export_hardening_test.go` `TestSARIFExportParsesBackAndAllowsOnlyHTTPLinks`, `internal/tools/manager_archive_test.go` `TestZIPInstallRejectsTraversalBeforeWriting`, `internal/reports/sarif_read_test.go` `TestReadSARIFRejectsInvalidLogs`. |
| Legitimate launch, refresh, CLI access, SSE reconnect still work | Automated | `internal/api/server_test.go` `TestScanEventsReplaysHistoryThenStreamsLiveInOrder`, `TestScanEventsStreamSendsHeartbeats`; `internal/events/bus_test.go` `TestSubscribeBridgesReplayToLiveWithoutLossOrDuplication`; identity handoff: `cmd/bluntcode/server_port_test.go` `TestHandoffToPortFindsLiveServer`, `TestHandoffToPortRejectsForeignAndDeadPorts` (verifies `/api/v1/meta` identity before handoff); CLI: `TestCLISubcommandsIntegration`. |

## IMP-06 — process-tree supervision and input protection

| Acceptance case | Status | Evidence |
|---|---|---|
| Hanging / descendant-spawning / output-flooding / cancel-ignoring children terminate within bounds with honest outcomes | Automated | `internal/process/runner_test.go` `TestRunReturnsPromptlyWhenGrandchildHoldsOutputPipes` (15 s bound), `TestRunEndsDescendantsAfterReturn` (Windows job object kill, verified via tasklist), `TestRunBoundsCapturedOutput` (64 KiB cap + Truncated flag), `TestRunDoesNotInvokeShell`; orchestrator cap: `internal/analyzers/analyzers_test.go` `TestRunDirectCapsOutput` (8 MiB); cancellation/panic: `internal/scans/jobs_test.go` `TestCancelWithJobsBoundStopsQueuedAnalyzers`, `TestAnalyzerPanicUnderJobsBoundFailsScanInsteadOfCrashing`; `internal/scans/service_test.go` `TestCancelDuringFinalAnalyzerNormalizeMarksCancelled`, `TestAnalyzerPanicFailsScanInsteadOfCrashing`; concurrency cap: `internal/scans/supervision_test.go` `TestConcurrentScanCapRejectsBeyondCapacity`; analyzer timeouts: `internal/scans` `TestAnalyzerTimeouts`. |
| Hash sources before/after; no unintended edits | Automated | `internal/scans/supervision_test.go` `TestScanLeavesWorkspaceUnchanged` (full-tree SHA-256 map compared before/after incl. `.bluntcodeignore`, lockfile, vendor files). |
| Containment claims tested with a deliberately misbehaving child | Automated | Misbehaving helpers are the test binary itself in `internal/process/runner_test.go`; stray-process lifecycle: `internal/analyzers/sonarqube/stray_windows_test.go` `TestSweepStrayServerProcessesEndsOrphan`, `TestKillOnCloseJobEndsTreeWhenHandleCloses`, `TestTrackProcessInKillOnCloseJobAssignsProcess`; `internal/analyzers/sonarqube/sonarqube_test.go` `TestManagedServerShutdownTerminatesOwnedProcessTreeAfterGracePeriod`, `TestTerminateProcessTreeStopsManagedChildOnWindows`. |

## IMP-15 scenario matrix — fake-analyzer runner

All seven hostile-scenario classes from the plan are automated with in-test fake analyzers (no external downloads):

| Scenario class | Test |
|---|---|
| Success with findings | `internal/scans/service_test.go` `TestPersistsRealNormalizedScanAndMarkdown` (real-fixture normalization) |
| Invalid output | `internal/scans/outcomes_test.go` warned-run tests; per-adapter `TestNormalizeEmptyStdoutAndBadExit` / `TestRunRejectsUnexpectedPlanShapes` / `TestNormalizeRejectsFatalExitCode` |
| Timeouts | `internal/scans` `TestAnalyzerTimeouts` |
| Output overflow | `internal/process/runner_test.go` `TestRunBoundsCapturedOutput`; `internal/analyzers/analyzers_test.go` `TestRunDirectCapsOutput` (8 MiB `DefaultOutputLimit`) |
| Descendants | `internal/process/runner_test.go` `TestRunEndsDescendantsAfterReturn`, `TestRunReturnsPromptlyWhenGrandchildHoldsOutputPipes` |
| Unexpected writes | `internal/scans/supervision_test.go` `TestScanLeavesWorkspaceUnchanged` |
| Analyzer panic | `internal/scans/service_test.go` `TestAnalyzerPanicFailsScanInsteadOfCrashing`; jobs-bound variant in `internal/scans/jobs_test.go` |

## Golden parser fixtures

Versioned golden fixtures pin every real-analyzer parser to recorded output:

- `tests/fixtures/biome/diagnostics.json`
- `tests/fixtures/ruff/diagnostics.json`
- `tests/fixtures/semgrep/results.json` (+ `unsafe_eval.py` source)
- `tests/fixtures/sonarqube/issues.json`
- `internal/analyzers/trivy/testdata/real-report.json`
- `internal/analyzers/checkov/testdata/real-report.json`
- `internal/analyzers/osv/testdata/real-report.json`
- `internal/analyzers/gitleaks/testdata/real-report.json`

Each has a `Normalize`-level test asserting the parsed finding set (see the matching `*_test.go` next to each fixture). Offline integration against *installed* tools (not fixtures) is exercised manually via the smoke procedure below.

## Server/CLI equivalence

- SARIF: `internal/reports/sarif_test.go` `TestSARIFBytesMatchTheAPIDownloadBytes` pins `reports.SARIFBytes` (the CLI export helper) to be byte-identical to the API download route's serialization. `TestReadSARIFRoundTripThroughSARIFBytes` closes the loop through baseline import.
- Markdown: the CLI `--format markdown` prints the same `reports.MarkdownBytes` document `GET /api/v1/scans/{id}/report.md` serves (shared helper; `cmd/bluntcode/scan.go` documents the pairing).
- JSON: the CLI summary (`writeScanJSON`) is a CLI-facing shape pinned by `TestWriteScanJSONShape` / `TestWriteScanJSONAllTerminalStates`; the API report structure is pinned separately by `internal/api/server_test.go` `TestReportEndpointReturnsStructuredReport`. Both serialize the same stored scan; there is no single test diffing CLI JSON against API report JSON field-for-field (the shapes differ by design — status: partial).
- Gates: one evaluator (`internal/scans/gate.go`) feeds CLI exit codes; the API surfaces the stored decision (see IMP-03 row above).

## Packaging smoke — release artifacts

`scripts/smoke.ps1` boots a given exe on a random loopback port with an isolated throwaway `LOCALAPPDATA` (no developer PATH assumptions) and probes health/meta/SPA. Procedure against a release artifact:

1. Download the release zip from the GitHub release page and extract it to a temp directory.
2. `powershell -File scripts/smoke.ps1 -ExePath <temp>\BluntCode-X.Y.Z\bluntcode.exe` — must print all probes passing and exit 0.
3. The release process itself (per AGENTS.md ship rule) additionally builds from the exact tagged commit in a clean worktree and verifies the 4 assets; the zip-sha256 is recorded in the release notes.

**Open:** this is a manual procedure, not an automated packaging test; there is no CI in the repo to run it.

## Leakage checks — logs and exported diagnostics

Automated:

- Export hardening (`internal/reports/export_hardening_test.go`): hostile corpus through HTML/CSV/GitHub/JSON/JSONL/SARIF — SARIF `helpUri` restricted to http(s) (`javascript:`/`data:`/`file:` dropped), CSV formula neutralization, GitHub workflow-command escaping, control-character scrubbing, HTML auto-escaping (`#ZgotmplZ` URL rewriting pinned in `internal/reports/html_test.go`, which also asserts raw markup and raw severity casing never leak).
- Secret findings themselves intentionally surface credential evidence (that is the product's purpose); artifact-path filtering exempts lockfiles and secret detectors deliberately (`internal/scans/service_test.go`).

Manual procedure (before each release): run one scan of a fixture workspace containing a planted fake credential, then inspect `%LOCALAPPDATA%\BluntCode\logs` for the planted secret string (must appear only inside findings payloads, never in request/paths logging) and confirm exported reports contain no absolute local user paths beyond the disclosed workspace root.

**Open:** the manual log inspection is not scripted.

## Open items (honest status)

These are tracked as not-yet-automated; none is claimed as done:

1. **Benchmarks and performance budgets** (IMP-15 items 5–6): no Go benchmarks or timing harness exist. Reproducible baseline procedure: on recorded hardware, `Measure-Command` a `bluntcode scan` over (a) a small fixture repo, (b) a larger monorepo checkout, (c) a workspace with a large planted finding set, cold vs warm cache, recording duration/peak memory/temp disk/output size; budgets to be set from those numbers before any performance claim is documented.
2. **Released-binary environmental matrix** (IMP-15 item 3): spaces-in-path is partially covered incidentally (repo path contains a space); Unicode paths, >260-char long paths, and restricted-permission directories have no dedicated tests or scripted checks.
3. **Docs-vs-inventory agreement** (IMP-01): llms.txt/CLI-manual analyzer claims are not diffed against the capability inventory automatically.
4. **Git-history input class** (IMP-02): no analyzer currently consumes history as a distinct input; nothing to test until one does.
5. **Whole-scan outbound-denied assertion, vuln-data age staleness, pentest cancellation** (IMP-04) and **per-endpoint origin tests** (IMP-05): per-component coverage as listed above; the aggregate assertions are not scripted.
6. **CI**: the repository has no CI (GitHub Actions removed deliberately); every automated item above runs locally via `go test ./...` and `npm test` in `web/`.
