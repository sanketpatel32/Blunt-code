# Changelog

All notable changes to Blunt Code are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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
