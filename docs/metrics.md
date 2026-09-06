# Finding counts, severity mappings, and the risk ledger

How Blunt Code turns analyzer output into the numbers every surface quotes —
and what those numbers deliberately do not claim. Everything here is
enforced by code; file references point at the source of truth.

## Severity mappings (versioned)

Every adapter projects its tool's own severity vocabulary onto the five
normalized severities (`critical`, `high`, `medium`, `low`, `info`). The
tables live with each adapter and are versioned together as
`analyzers.SeverityMappingVersion` (`internal/analyzers/analyzer.go`); a
change to what a raw value maps to is a re-rating and bumps the version.

| Analyzer | Raw values → normalized | Unrecognized raw value |
|---|---|---|
| sonarqube | BLOCKER→critical, CRITICAL→high, MAJOR→medium, MINOR→low | falls back to `info`, marked unmapped |
| semgrep | ERROR/CRITICAL→high, WARNING→medium, INFO→info | falls back to `low`, marked unmapped |
| biome | FATAL→high, ERROR→medium, WARNING→low | falls back to `info`, marked unmapped |
| trivy | CRITICAL→critical, HIGH→high, MEDIUM→medium, LOW→low | falls back to `info`, marked unmapped (includes `UNKNOWN`) |
| checkov | CRITICAL/HIGH/MEDIUM/LOW/INFO per policy metadata | non-empty unrecognized → adapter default, marked unmapped; empty (offline default) → adapter default, no marker |

Rules for unmapped values, enforced per adapter:

- The tool's own value is preserved verbatim in `raw_severity` (stored,
  exported in JSON, rendered by the Markdown report).
- The normalized severity is the adapter's documented fallback — never a
  guess dressed up as the tool's claim.
- The finding carries `metadata.severity_unmapped = true` so UIs and
  consumers can show the gap as a mapping problem.

Adapters that derive severity from rule identity rather than a tool-reported
severity (ruff, todo, secrets, gitleaks, license) have no raw value to map
and document their projection in the adapter itself. osv normalizes from the
OSV database's own severity/CVSS fields with the raw value preserved.

### Values we do not invent

There is no confidence, exploitability, remediation-certainty, or license
legal-interpretation field anywhere in the finding model, because the
underlying analyzers do not report one. Fields that exist are populated from
tool output; fields that would require inference are absent rather than
fabricated.

## What each count means

- **Active** — findings stored for the scan that are not suppressed. The
  scan record's `total_findings` and severity counts are active-only
  (`database.CompleteScan`, `internal/database/repository.go`). Suppressed
  findings remain stored with their fingerprint and provenance; they are
  excluded from counts, gates, risk, and exports, and reappear if the
  suppression is removed.
- **Suppressed** — workspace-level suppressions keyed by finding
  fingerprint. Suppressing a finding never deletes it and never rewrites
  history; older scans keep their recorded counts.
- **New / persistent / fixed** — comparison classes against the previous
  completed scan, computed by coverage-aware comparison
  (`internal/scans/compare.go`). A previous finding counts as **fixed** only
  when its analyzer succeeded in this scan *and* the finding's file was part
  of this scan's evaluated inputs.
- **Not evaluated** — analyzers whose previous findings could not be
  classified (analyzer failed this scan, or its files were not in this
  scan's inputs). Listed in scan comparisons, the `/compare` API response
  (`not_evaluated`), and report warnings; their findings are never counted
  as fixed.

### Duplicates vs. corroboration

Two findings from the *same* analyzer with the same rule, path, and
normalized message are occurrences of one issue: fingerprint V2 keeps the
first occurrence's identity stable (so suppressions and baselines keep
matching) and gives occurrences 2+ distinct fingerprints, so each row stays
individually addressable (`internal/analyzers/analyzer.go`). The *same*
issue reported by two different analyzers is corroboration, not duplication:
both findings are stored, counted, and displayed with their own provenance —
nothing deduplicates across analyzers, because two tools agreeing is
evidence, not noise.

## The weighted risk ledger

One weighted score per scan: `critical×10 + high×5 + medium×2 + low×1`
(info weighs nothing), bucketed into letter grades **A** < 5, **B** < 20,
**C** < 50, **D** ≥ 50. The same weights and bands are implemented in three
places that must stay in sync: the workspace risk endpoint
(`internal/api/server.go`), the global stats rollup
(`internal/database/repository.go`), and the web client
(`web/src/lib/risk.ts`).

The score is computed from a workspace's latest **producing** scan —
`completed` or `completed_with_warnings`. Failed, cancelled, interrupted,
and running scans never feed risk.

### Coverage and freshness pairing

A low score from a scan that lost analyzer runs is not the assurance of a
low score from a complete scan, so the grade never travels alone. The risk
endpoint (`GET /api/v1/workspaces/{id}/risk`) pairs every score with:

- `scan_state` — the terminal state of the scan behind the grade;
- `finished_at` — when that scan finished (freshness);
- `coverage` — `{total, succeeded, failed, warned}` over the scan's analyzer
  runs, where *warned* counts runs that succeeded with degraded output
  (unparseable batches);
- `complete` — true only when every run succeeded with no warnings.

The dashboard's workspace cards carry the same pairing as
`latest_scan_coverage`, and the Risk Board renders it: the verdict caveat,
the per-row `partial` badge, and the workspace risk card's coverage note all
say when a grade reflects partial coverage.
