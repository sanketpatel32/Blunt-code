# Configuration

Blunt Code keeps everything under one data directory resolved from the
`LOCALAPPDATA` environment variable (falling back to the user cache
directory when `LOCALAPPDATA` is unset):

```text
%LOCALAPPDATA%\BluntCode
├── bluntcode.db       # SQLite database (workspaces, scans, findings, settings)
├── logs\              # Diagnostic logs (bluntcode.log)
├── temp\              # Scan-time scratch space
├── tools\             # Sandboxed managed analyzers, pinned per version
└── reports\           # Generated Markdown reports (created on first report)
```

Everything the app needs is derived from that one root, which is why
redirecting `LOCALAPPDATA` (as the CI workflow does) relocates the entire
installation state. `logs`, `temp`, and `tools` are created up front;
`reports` is deliberately not, so a fresh install carries no empty folder.

## `bluntcode config`

`bluntcode config` prints a read-only report of the effective configuration
— what this installation would do right now — for troubleshooting. It never
acquires the single-instance data-directory lock and never creates anything,
so it is safe to run while the app or a scan is live.

```powershell
.\bluntcode.exe config
.\bluntcode.exe config --json   # the same data as a stable fixed-key JSON shape
```

What each section means:

- **Version** — the Blunt Code build.
- **Paths** — every resolved path (data directory, database, tools,
  reports, logs, temp), each with an `exists` or `missing` marker. `missing`
  on a fresh machine is normal; `missing` on an installation you expected to
  be populated points at a redirected `LOCALAPPDATA`.
- **Environment overrides** — `BLUNTCODE_SONAR_STARTUP_TIMEOUT` in effect,
  including an invalid-value note (below).
- **App settings** — the persisted settings when the database is readable
  (offline mode, open-browser-on-start); the effective defaults when no
  database exists yet; or an explicit "app is running" note when the
  database cannot be opened — the one realistic cause while a live app
  holds SQLite.
- **Managed tools** — the pinned version of every managed analyzer and
  whether that exact version is installed on disk. `not installed` for a
  tool means the next scan that needs it will download it (unless offline
  mode is on).

Any positional argument is a usage error (exit 2); the command takes only
`--json`.

## `BLUNTCODE_SONAR_STARTUP_TIMEOUT`

The managed SonarQube server gets a 10-minute startup budget by default
(cold boots on consumer hardware legitimately exceed three minutes). Set
`BLUNTCODE_SONAR_STARTUP_TIMEOUT` to a Go duration to override it:

```powershell
# PowerShell
$env:BLUNTCODE_SONAR_STARTUP_TIMEOUT = "45s"

# bash
export BLUNTCODE_SONAR_STARTUP_TIMEOUT=5m
```

Valid values are Go duration strings such as `90s`, `45s`, or `5m`.
Unparseable, zero, or negative values log a warning and fall back to the
default — the scan is never failed by a bad value. `bluntcode config` shows
the parsed value, or marks it `invalid (will warn and use default)`. The
health poll starts immediately (an already-running server returns on the
first check), polls every second, and logs progress on status transitions
and roughly every 20 seconds with elapsed time, remaining budget, and the
last observed status.

## Offline mode

Offline mode is a persisted app setting toggled from **Settings** in the web
UI. With it on, scans run entirely from local assets:

- Managed tools are never downloaded. A scan needing an uninstalled tool
  fails that analyzer with a local-dependency message (for example, `semgrep
  is not installed and offline mode is enabled`) instead of reaching the
  network.
- The built-in in-process analyzers (secrets, TODO) are deliberately held
  back in offline mode as well: an offline scan with no available analyzers
  keeps failing honestly instead of being rescued by the built-ins. If you
  need the built-ins, run with offline mode off.
- Everything else — discovery, scans over already-installed tools, reports,
  the CLI — behaves identically.

The practical sequence is: run one scan online per machine so the managed
tools download, then toggle offline mode on.

## Scan profiles

Every scan runs in one of three profiles, selected with `--profile` in the
CLI or the profile picker in the UI:

| Profile | Analyzers | Notes |
| :--- | :--- | :--- |
| **quick** | Ruff + Biome only | Fast feedback between edits; skips semgrep, SonarQube, and the built-ins entirely |
<!-- bluntcode:ignore -->
| **standard** | Ruff, Biome, Semgrep, SonarQube, secrets, TODO | The everyday default; Ruff runs its default rule set (E4, E7, E9, F) |
| **deep** | Same set as standard | Ruff's rule selection widens to `E,W,F,B,SIM,C4,RET,ARG,PLR`; the other analyzers run the same configuration as standard |

The built-in analyzers (secrets, TODO) have no managed tool to download —
they ship in the binary and participate in every standard or deep scan. The
quick tier stays a fast language-specific pass on purpose.

Related pages: [ci.md](ci.md) for headless scans and gates,
[ignoring-findings.md](ignoring-findings.md) for ignore mechanisms and
exclude patterns, [analyzers.md](analyzers.md) for how each managed tool is
pinned and sandboxed.
