<p align="center">
  <img src="web/public/bluntcode-mark.svg" alt="Blunt Code" width="84" />
</p>

<h1 align="center">Blunt Code</h1>
<p align="center"><strong>Local code quality & security — no cloud, no account, no telemetry.</strong><br/>Ruff · Biome · Semgrep · SonarQube · Secrets · TODO — one loopback app for Windows.</p>

<p align="center">
  <a href="https://github.com/sanketpatel32/Blunt-code/releases"><img src="https://img.shields.io/github/v/release/sanketpatel32/Blunt-code?label=version&color=1a1a1e" alt="release" /></a>
  <img src="https://img.shields.io/badge/platform-Windows%2010%2F11-0a66ff" alt="platform" />
  <img src="https://img.shields.io/badge/privacy-100%25%20local-0a6522" alt="privacy" />
  <img src="https://img.shields.io/badge/license-MIT-8a8a8a" alt="license" />
  <img src="https://img.shields.io/badge/UI-shadcn%20%2B%20Tailwind-111827" alt="ui" />
</p>

<p align="center">
  <a href="#-install-in-30-seconds">Install</a> ·
  <a href="#-quick-start">Quick start</a> ·
  <a href="#-cli">CLI</a> ·
  <a href="docs/ci.md">CI guide</a> ·
  <a href="docs/configuration.md">Config</a> ·
  <a href="CHANGELOG.md">Changelog</a>
</p>

---

### What's new in 0.7.0 — shadcn clean UI

Tailwind 3.4 · Radix Dialog/Slot · CVA · lucide-react. Ten iterative loops: glass nav `backdrop-blur(16px)`, pill badges `oklch`, table hover `color-mix(in oklch, accent 4%)`, spring toasts with lifetime bar, shimmer skeletons, staggered dialogs. **1869 modules · 259 tests passing · `—` placeholders never `NaN`.**

> Previous `0.6.0` shipped global search, risk scores, scan pruning, workspace tags, CSV/SARIF/JSONL exports, and gated CI. See [CHANGELOG.md](CHANGELOG.md).

---

## Why Blunt Code

| Without it | With Blunt Code |
|---|---|
| Four linters, four installs, four `PATH` fights | One `bluntcode.exe` — tools sandboxed under `%LOCALAPPDATA%\BluntCode\tools`, never touches global `PATH` |
| Findings scattered across terminals | One report: Markdown / HTML / SARIF / CSV / JSON, filterable by tool · rule · severity · path |
| Cloud gate that ships your source | `127.0.0.1` only. SQLite + reports stay on disk. Offline mode after first download |
| "Fix later" that never sticks | `bluntcode:ignore` at the source, fingerprint suppressions, `.bluntcodeignore` committed with the repo |

---

## Features

**Private by design** — loopback server, zero telemetry, SQLite at `%LOCALAPPDATA%\BluntCode\bluntcode.db`.

**Batteries included** — Ruff (Python), Biome (JS/TS + React domain auto-detect), Semgrep (20-rule pack), SonarQube (polyglot), plus in-binary **secrets** (AWS, GitHub, Slack, JWT, Stripe, OpenAI, Anthropic, Azure) and **TODO/FIXME** trackers across 40+ file types (Go, Java, Kotlin, C#, PHP, Rust, Swift, YAML, TOML, `.env`, Dockerfile, PEM…).

**Dashboard that actually helps** — global overview cards (`/api/v1/stats`), recent activity feed with severity dots, risk grades `A–D` (critical×10/high×5/med×2/low×1) with trend, severity distribution bars, stacked history, and suppressions panel (search by reason/fingerprint).

**Workspaces that scale** — up to 3 tag chips `+N` overflow, `Filter by tag` debounced 200ms, sort by Name / Last scan / Findings, `Filter by tag` + `Sort` stay in sync, 51→2 queries optimisation at 50 workspaces.

**Reports you can use** — sticky filter toolbar, severity-tinted row edges, removable chips, toggles `page`/`page_size` (50 rows), `—` placeholders for missing data, hostile-corpus HTML-escaped.

**CLI built for CI** — `bluntcode scan <path> --profile quick|standard|deep --fail-on high+ --max-findings 50 --baseline ./last.sarif --format sarif|jsonl|markdown|github --output findings.csv --jobs 2 --incremental --watch`, exit `0/1/2/130`.

> Full flags: `bluntcode help` · CI recipe: [docs/ci.md](docs/ci.md)

---

## System requirements

| | |
|---|---|
| **OS** | Windows 10 / 11 64-bit (amd64) |
| **Rights** | Standard user — no admin |
| **Network** | Online only first run (tools download). Offline after |
| **Deps** | None — no global Python/Java/Node required |

---

## Install in 30 seconds

### One-line installer (recommended)

PowerShell (`Win+R` → `powershell`):
```powershell
irm https://github.com/sanketpatel32/Blunt-code/releases/latest/download/install-latest.ps1 | iex
```
Cmd (`Win+R` → `cmd`):
```bat
curl -fsSL -o "%TEMP%\install-bluntcode.cmd" https://github.com/sanketpatel32/Blunt-code/releases/latest/download/install.cmd && "%TEMP%\install-bluntcode.cmd"
```
Installs to `%LOCALAPPDATA%\Programs\BluntCode`, verifies SHA-256, creates Start Menu shortcut, launches app. No admin.

**Options** `install-latest.ps1 -Version 0.7.0 -Silent -DesktopShortcut -WhatIf -WaitForCloseSeconds 30`

### Portable ZIP

1. Download `BluntCode-0.7.0-windows-amd64.zip` + `.sha256` from [Releases](https://github.com/sanketpatel32/Blunt-code/releases/latest)
2. Verify & run:
```powershell
$pkg='.\BluntCode-0.7.0-windows-amd64.zip'
if((Get-FileHash $pkg -Algorithm SHA256).Hash -ne (Get-Content "$pkg.sha256").Split()[0]){throw 'checksum mismatch'}
Expand-Archive $pkg -DestinationPath .\BluntCode -Force; .\BluntCode\BluntCode*\bluntcode.exe
```
> `irm | iex` never saves a `.ps1`, so ExecutionPolicy doesn't apply. For a saved `uninstall.ps1`: `powershell -ExecutionPolicy Bypass -File .\uninstall.ps1`.

### From source

Requires Go 1.26+ and Node 18+:
```powershell
git clone https://github.com/sanketpatel32/Blunt-code.git; cd Blunt-code
.\scripts\build.ps1; .\bluntcode.exe
```

---

## Quick start

1. **Launch** from Start Menu or `bluntcode.exe` → opens `http://127.0.0.1:<port>` automatically
2. **Add workspace** → pick a project folder
3. **Files & rules** (optional) → exclude `dist/`, `vendor/`, or add `.bluntcodeignore` to commit excludes
4. **Run scan** → first run downloads tools, next runs instant; progress bar `finished/total` + live analyzer pills
5. **Inspect** → click a finding → source preview with highlight, **Copy location/fingerprint**
6. **Export** → Markdown / HTML / SARIF / CSV (filters honoured) / JSON — or `Scan history → Prune history --keep 20`

Keyboard: `g h/w/t/s/a` navigate · `n` add workspace · `/` search · `?` help · `Ctrl+K` command palette.

---

## CLI

```powershell
bluntcode "C:\Projects\my-app" --no-browser --port 52160
bluntcode doctor              # diagnostics
bluntcode doctor --json --fix # self-heal stale rules / interrupted installs
bluntcode scan "C:\my-app" --profile quick --json --quiet --timeout 10m
bluntcode scan "C:\my-app" --fail-on high+ --max-findings 50 --baseline .\baseline.sarif
bluntcode scan "C:\my-app" --format github   # PR annotations (error/warning/notice)
bluntcode scan "C:\my-app" --format csv --output findings.csv --save-baseline baseline.sarif
bluntcode scan "C:\my-app" --gate-analyzer semgrep,secrets --gate-category security
bluntcode prune "C:\my-app" --keep 20
bluntcode scan "C:\my-app" --jobs 2 --incremental --watch
bluntcode config              # resolved paths, overrides, tool versions
```

`--fail-on` gates **new** findings only when `--baseline` is set. See [docs/ci.md](docs/ci.md).

### 🤖 For AI agents

Blunt Code ships `llm.txt` / `llms.txt` ([llmstxt.org](https://llmstxt.org)) and `bluntcode agent` helpers — all machine-readable, no browser needed.

```powershell
bluntcode llm                  # cat llm.txt (same as llms.txt)
bluntcode agent docs           # same — agent guide to stdout
bluntcode agent scan "C:\my-app" --profile quick --fail-on high+  # forces --json --quiet, progress on stderr
```

`llm.txt` lives at repo root and at `http://127.0.0.1:<port>/llm.txt` when UI is running, plus embedded in binary (`go:embed llm.txt`). See `llm.txt:1` for full JSON schemas, API endpoints, exit codes, and PowerShell examples.

---

## Analyzers & profiles

| Analyzer | Languages | Checks |
|---|---|---|
| **Ruff** | Python | Lint, style, bugbear, `SIM/C4/RET/ARG/PLR` in Deep |
| **Biome** 2.5.6 | JS/TS, React auto-domain | Format, correctness, React hooks |
| **Semgrep** | Python, JS/TS | 20-rule security pack (injection, deserialization, secrets, `postMessage`) |
| **SonarQube** | Polyglot project-wide | Code smells, hotspots, metrics (cold boot ≤10 min, `BLUNTCODE_SONAR_STARTUP_TIMEOUT`) |
| **Secrets** (built-in) | 40+ types inc. `.env`/Dockerfile/YAML | AWS, GitHub, Slack, JWT, Stripe, OpenAI, Anthropic, Azure |
| **TODO** (built-in) | Code & config where comments exist | `TODO/FIXME/HACK/XXX/BUG` with owner `TODO(jane):` |

**Profiles** · Quick `Ruff+Biome` · Standard `+Semgrep+SonarQube` · Deep `+Ruff extended`

Ignore at source: `// bluntcode:ignore` or `// bluntcode:ignore secrets.aws-access-key-id reason: test key` · or suppress by fingerprint with reason (500 chars) · or `.bluntcodeignore` patterns (`dir/**`, `**/name`, basename, `#` comments, 1000/64 KiB cap).

---

## Data & privacy

```
%LOCALAPPDATA%\BluntCode
├── bluntcode.db      # workspaces, scans, findings (SQLite, PRAGMA quick_check in doctor)
├── reports\          # Markdown reports
├── logs\             # redacted diagnostics
└── tools\            # ruff, biome, semgrep, sonarqube (manifest-hashed)
```

Loopback only (`127.0.0.1`), no account, no telemetry. Open folders from **Settings → Data folders**. Full layout: [docs/configuration.md](docs/configuration.md).

---

## Documentation

- [CI](docs/ci.md) · [Ignoring findings](docs/ignoring-findings.md) · [Configuration](docs/configuration.md) · [Architecture](docs/architecture.md) · [Analyzers](docs/analyzers.md) · [Privacy](docs/privacy.md) · [Release](docs/release.md)

---

## Troubleshooting

<details><summary>PowerShell blocks a saved script</summary>

`irm | iex` doesn't save a file → no policy check. For `uninstall.ps1`: `powershell -ExecutionPolicy Bypass -File .\uninstall.ps1`.
</details>
<details><summary>First scan slow</summary>Downloads sandboxed tools & warms SonarQube. Next scans use local cache and `--incremental` (hash `SHA-256` per file, persisted, invalidated on analyzer/profile/version change).</details>
<details><summary>Offline mode</summary>Settings → Offline mode ON after first scan. Built-ins held offline, gated by flag.</details>
<details><summary>"Only one process can use the data directory"</summary>SQLite lock. Close other instance or `bluntcode doctor` to inspect.</details>
<details><summary>Where are reports?</summary>Settings → Data folders → Reports/Logs/Tools/DB, or `bluntcode config`.</details>

---

## Uninstall

```powershell
cd "$env:LOCALAPPDATA\Programs\BluntCode"
.\uninstall.ps1              # keep DB & reports
.\uninstall.ps1 -RemoveData  # full wipe
```
Installer auto-cleans Start Menu shortcut and refuses while app is running.

---

## Contributing

```powershell
go test ./...                # Go vet/build/tests
cd web; npm test; npm run build
.\scripts\package.ps1 -Version 0.7.0
```

See [CONTRIBUTING.md](CONTRIBUTING.md) · [docs/architecture.md](docs/architecture.md) — shadcn tokens live in `web/src/tokens.css` (`--color-paper/ink/accent`, `--radius-*`, `--shadow-*`), components in `web/src/components/ui/*`.

---

## License

MIT — see [LICENSE](LICENSE). Third-party analyzers keep their licenses — see [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md).

<p align="center"><sub>Built for Windows. Audited for hostile corpora, tabular-nums, sticky glass headers, and 0 horizontal scroll at 320px.</sub></p>
