# Implementation status

## Completed

- Milestone 0: Go service, SQLite migrations, health/meta APIs, loopback-only HTTP server, Vite-to-embedded-assets build script.
- Milestone 1: persistent workspaces and rules, validated roots, native Windows picker, safe source discovery, language detection, default exclusions, lazy tree API.
- Milestone 2: analyzer registry integration, Quick/Standard/Deep scan profiles (Quick runs Ruff/Biome only; Deep matches Standard in V1), persisted file and directory selection overrides (including more-specific file exceptions), direct-process analyzer runs, scan lifecycle/cancellation, SSE events, normalized-result persistence, partial-failure outcomes, and local Markdown reports.
- Milestones 3–5: Ruff and Biome structured parsers, pinned checksum-verified private installation, tool readiness/install APIs, normalized report rendering/export, and coverage-aware new/fixed/persistent comparison.
- Milestone 6 implementation: Semgrep 1.172.0's Windows wheel and official uv 0.11.16 are checksum-verified before private app-data installation; bundled local rules are extracted and versioned there; scan plans use only those rules with metrics, version checks, inherited app token, and user settings disabled.
- Milestone 7 implementation: managed SonarQube Community Build, SonarScanner, and Java 21 artifacts are pinned and checksum-verified, safely extracted under app data, configured on a dynamically selected loopback port, and bootstrapped with DPAPI-protected local credentials. The adapter starts and stops this private server for scans; a Windows fixture scan completed with Ruff, Semgrep, and SonarQube all succeeding.
- Milestone 8 implementation: path/origin guards, archive traversal protection, crash recovery, process cancellation and Ctrl+C lifecycle shutdown, bounded per-tool process output and timeouts, redacted local diagnostic logs with a 500 MB cap, Windows per-data-directory single-instance protection, persisted offline mode (enforced by tool installation), local-only `bluntcode doctor` diagnostics (including disk-space and JSON output), documentation, notices, checksum-verified local/HTTPS installer paths, build scripts, and focused unit/integration tests.

## Remaining

- Run a full Windows UI E2E before claiming a public V1 release. The managed Semgrep install, bundled-rule fixture scan, managed SonarQube bootstrap/scan, ZIP checksum, and install/uninstall script were exercised locally on Windows. The uv archive and Semgrep Windows wheel are checksum-pinned; Semgrep's resolved transitive Python dependency artifacts are not yet hash-locked or release-audited.
