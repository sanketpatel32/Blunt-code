# Changelog

All notable changes to Blunt Code are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Fixed

- **Per-language file routing for analyzers:** scan orchestration now hands every analyzer only the selected files whose language it actually supports (ruff receives `.py`/`.pyi`, Biome receives `.js`/`.jsx`/`.mjs`/`.cjs`/`.ts`/`.tsx`/`.mts`/`.cts`). Previously a mixed-language workspace passed the entire file list to every analyzer, which dogfooding on this repository exposed dramatically: ruff parsed a committed minified JavaScript bundle as Python and failed the scan after generating 8 MB of syntax-error JSON. The ruff and Biome adapters additionally filter defensively, so no caller can hand them files they cannot lint, and an empty post-filter selection skips the analyzer instead of invoking a bare `ruff check` that would scan its working directory.
- **Ruff file batching for large workspaces:** scanning a 692-file Python project failed with `fork/exec ... The filename or extension is too long` because the ruff adapter passed every file in a single process invocation, overflowing Windows' 32,767-character command-line limit. Ruff now splits file arguments into batches (like Biome and semgrep always did) and merges the per-batch JSON outputs into one result.
- **Frontend lint debt cleared by dogfooding:** a self-scan surfaced 79 findings in non-test UI source, all fixed: explicit `type` on every action button (43), honest `useEffect`/`useCallback` dependency lists in the scan stream, theme, file-tree, and history hooks (7, including a restructured OS-theme listener and a page-reset trigger keyed to the newest scan), stable React list keys that no longer derive from array indices (skeleton placeholders, file-rule rows, scan event stream), and accessibility repairs (labelled SVG toast icons, a semantic list for language badges, a real `fieldset` for severity filters, list-based filter chips, `aria-hidden` removed from the focusable `/` search hint, non-null assertions replaced with real guards in App routing and the workspace detail view).

## [0.2.1] - 2026-08-22

A batch of 10 QA loops: fuzzing, hostile-payload round-trips, a live-browser
audit, a security pass, lifecycle/concurrency stress tests, and scale
benchmarks. Every fix below ships with a regression test.

### Security

- **CSV formula injection (High):** findings text is untrusted (it derives from scanned code). Cells starting with `=`, `+`, `-`, `@`, tab, or CR are now prefixed with `'` in `findings.csv`, neutralizing spreadsheet formula execution on export-open.
- **SARIF control-byte leak:** DEL (0x7f) was written raw and terminal escape sequences could smuggle into rule ids, messages, and help URIs; all analyzer-derived SARIF strings now pass the same control scrubber as the other formats.
- **Markdown and terminal hardening:** C0/DEL control characters are stripped from report tables and from CLI stdout/stderr output (ANSI poisoning, forged log lines).

### Fixed

- Eight list endpoints returned `null` instead of `[]` when empty (scans, findings, rules, path-overrides, fixed, report findings/metrics/warnings) — fixed at the repository source.
- Frontend crashes on hostile data: invalid date strings crashed every date-rendering page (RangeError); dashboard summaries rendered `NaN`; findings pagination showed "0–NaN of undefined"; `null` tool/tree/rules bodies threw; report payloads without a `scan` object crashed the report view.
- Scan lifecycle: an analyzer panic killed the whole app instead of failing the scan; a database failure during completion left the workspace permanently 409-locked; cancelling during the final analyzer reported `completed` instead of `cancelled`; orphaned Windows process trees could hang a scan forever (now killed with `taskkill /T` plus a wait deadline); concurrent tool installs raced on file locks; cancelling one scan killed the shared SonarQube server out from under another.
- CLI: flags now parse in any position relative to the path argument; the second Ctrl+C always exits with cleanup; NTFS junction and case-variant paths no longer create duplicate workspace rows (fixed at the normalization root for both the CLI and the API); the instance-lock failure path no longer leaks the log-file handle.
- Live-browser audit: wide tables no longer pan the whole page sideways on mobile; the skip-to-content link now actually moves focus; four color tokens were below WCAG AA contrast and were adjusted in both themes.

### Performance

- New `findings(scan_id, fingerprint)` index: the findings page at 50k findings went from 215ms to 2ms; the CSV export of 52k rows went from an effective hang to 868ms; the fixed-findings listing at 20k rows went from 9.1s to 125ms.
- Workspace lists no longer run a full-history query per workspace (51 queries → 2 at 50 workspaces).
- Frontend: `Intl.DateTimeFormat` instances hoisted to module level (~45× faster date rendering); hostile finding messages and long paths are line-clamped instead of stretching the table.

## [0.2.0] - 2026-08-22

A batch of 25 improvement loops focused on automation, exports, and UI polish.

### Added

- **Headless CLI scans:** `bluntcode scan <path>` runs one full scan without the browser or server, with `--profile quick|standard|deep`, `--json` (machine-readable summary for CI), `--timeout`, and `--quiet` flags. Progress streams to stderr and the summary prints to stdout. Exit codes are CI-friendly: `0` when the scan completes (warnings included), `1` when it fails, is cancelled, or times out, `2` for usage errors, and `130` after Ctrl+C.
- **SARIF export:** `GET /api/v1/scans/{id}/report.sarif` serves a SARIF 2.1.0 log with deduplicated rules, severity-to-level mapping, and workspace-relative artifact locations — compatible with VS Code SARIF viewers and GitHub code scanning.
- **HTML report export:** `GET /api/v1/scans/{id}/report.html` serves a standalone, self-contained HTML document (summary strip, analyzer and findings tables, print styles, no external assets).
- **CSV export:** `GET /api/v1/scans/{id}/findings.csv` exports findings honoring the current filters (category, path, query, severity, status), with a UTF-8 BOM so Excel renders it correctly.
- **Global scans feed:** `GET /api/v1/scans` returns recent scans across all workspaces plus a dashboard summary (per-workspace severity totals from each workspace's latest completed scan, scan counts, and active scan count).
- **Fixed-findings listing:** `GET /api/v1/scans/{id}/fixed` lists findings fixed since the previous completed scan, coverage-aware (only analyzers that succeeded in both scans are compared).
- **Open-folder endpoint:** `POST /api/v1/system/open-folder` opens Blunt Code's own `data`/`reports`/`logs`/`tools` folders in Windows Explorer. The request carries a fixed enum — never a caller-supplied path — and the server launches `explorer.exe` directly with no shell.
- **Dark mode:** theme toggle with OS-preference sync, explicit choice locking, and a no-flash bootstrap script (persisted in `localStorage` under `bluntcode-theme`).
- **Home dashboard:** activity summary cards (active scans, critical+high findings, totals, weekly scans), a recent-scans feed with state/profile/severity dots, and a quick-scan action.
- **Report visualizations:** stacked severity distribution bar with legend and per-analyzer severity mini-bars sorted by count; print styles hide app chrome, keep severity colors, and pin the light palette when printing from dark mode.
- **"What changed" panel:** lists findings fixed since the previous scan (top 10 with severity, rule, and location) once a scan reaches a terminal state.
- **Export menu:** Markdown / HTML / SARIF / CSV from the report view, with CSV honoring the active filters and sort.
- **Scan history upgrades:** per-scan severity mini-bars with tooltip breakdown, profile badges, warning/danger hints on the newest row, and per-scan Markdown export.
- **Keyboard shortcuts:** `g` then `h`/`w`/`t`/`s` navigation sequences, `n` for add workspace, `/` to focus search, `?` for a help dialog; suppressed while typing or when a dialog is open.
- **File tree search:** recursive search across loaded folders with auto-expansion, match counts, substring highlighting, per-node loading spinners, retryable load failures, and `/` to focus.
- **Toast notifications:** stacked toasts (bottom-right, max 4, auto-dismiss with pause on hover/focus, error codes as badges) replacing the single-slot notice banner.
- **Loading skeletons:** layout-aware table/cards/lines skeletons with screen-reader labels, plus Installing/Repairing/Updating busy feedback on tool rows.
- **Empty states & all-clear:** icon medallions on every empty state and a celebratory all-clear panel for completed zero-finding scans.
- **UI robustness & accessibility:** error boundary with crash details and route-keyed recovery, a real 404 page, About page in nav, skip-to-content link, dialog focus traps with `aria-modal` and backdrop close, relative timestamps with absolute tooltips, copy finding/location to clipboard, arrow-key row navigation, and `scope=col` table headers.
- **Settings:** buttons to open Blunt Code's data, reports, logs, and tools folders in Explorer.

### Changed

- **Deeper deep profile:** the deep scan profile now runs Ruff with an extended rule selection (`--select=E,W,F,B,SIM,C4,RET,ARG,PLR`), covering style, bugbear, simplification, comprehension, return, argument, and refactor rules. Quick and standard profiles are unchanged. Diagnostic logs now record the executed analyzer command line.
- **Shared findings filter pipeline:** the JSON and CSV findings endpoints are served by one query builder, so filters behave identically across both.
- **Frontend modularized:** the `App.tsx` monolith was split into `pages/`, `components/`, `hooks/`, and `lib/` modules; the web test suite grew from 16 to 124 cases.
- **Findings table:** columns (severity, file, tool, status) are now sortable server-side with `aria-sort`, severity pills are clickable filters, text filters debounce (300 ms) with removable filter chips.

### Fixed

- Unknown URLs now render a friendly 404 page instead of silently showing Home.
- Printing a report from dark mode no longer produces a dark-themed printout; the light palette is pinned for print.
- Theme no longer flashes on load: an inline bootstrap script applies the stored theme before first paint.
