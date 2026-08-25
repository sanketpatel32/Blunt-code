<p align="center"><img src="web/public/bluntcode-mark.svg" alt="Blunt Code logo — an accent-blue rounded tile with a bold chevron prompt and cursor" width="96" /></p>

# Blunt Code 🛡️

> **All-in-One Local Code Quality & Security Analyzer for Windows**

**Blunt Code** is a desktop application for Windows that runs deep code analysis on your local software projects and aggregates findings into a single, unified report. It features an interactive, modern web interface that runs 100% locally on your computer—requiring **no cloud accounts, no API keys, and no pre-installed linters or language runtimes**.

---

## 📌 Table of Contents

- [✨ Key Features](#-key-features)
- [💻 System Requirements](#-system-requirements)
- [🚀 Installation Options](#-installation-options)
  - [Option 1: One-Line Installer (Recommended)](#option-1-one-line-installer-powershell-or-cmd-recommended)
  - [Option 2: Standalone ZIP Download (Portable)](#option-2-standalone-zip-download-portable)
  - [Option 3: Build from Source](#option-3-build-from-source)
- [🎯 Quick Start Guide](#-quick-start-guide)
- [🛠️ Managed Analyzers & Languages](#️-managed-analyzers--languages)
- [🖥️ Command-Line Interface (CLI)](#️-command-line-interface-cli)
- [📁 Data Storage & Privacy](#-data-storage--privacy)
- [📚 Documentation](#-documentation)
- [❓ Troubleshooting & FAQ](#-troubleshooting--faq)
- [🗑️ Uninstalling](#️-uninstalling)
- [🤝 Contributing](#-contributing)
- [📜 License](#-license)

---

## ✨ Key Features

- **100% Local & Private:** Runs entirely on loopback (`127.0.0.1`). Your code, findings, reports, and logs never leave your device.
- **Zero-Config Managed Tools:** Automatically installs and sandboxes code analyzers in isolated application directories. No manual `PATH` configuring or Python/Java/Node dependency hassle — and `doctor --fix` self-heals stale rules and interrupted installs.
- **Multi-Tool Coverage:** Runs **Ruff**, **Biome**, **Semgrep**, and **SonarQube** concurrently to check for lint issues, security SAST vulnerabilities, formatting errors, and code smells — plus built-in **secrets** and **TODO/FIXME** detectors that ship in the binary with nothing extra to install, across 40+ file types from Go and Java to `.env` files, Dockerfiles, and YAML.
- **Interactive UI Dashboard:** Built-in web app with a home dashboard of global overview cards, real-time scan logs, file-level previews, severity visualizations, rich filtering by tool/rule/severity, severity trends across scan history, and per-finding suppression (with reason) that hides dismissed findings from future scans and gates.
- **Reports in Every Format:** One-click export to **Markdown**, standalone **HTML**, **SARIF 2.1.0** (VS Code / GitHub code scanning), **CSV**, and **JSON** — or emit **SARIF**, full **JSON** reports, and inline **GitHub Actions annotations** straight from the CLI.
- **CI Gates, Baselines & Watch Mode:** `--fail-on high+` / `--max-findings N` turn a headless scan into a build gate, `--baseline` (a previous scan ID or SARIF file) excludes known findings so gates start passing on day one, `--jobs N` parallelizes analyzers, `--incremental` rescans only files that changed (`--watch` does this automatically from its second scan onward), a committed `.bluntcodeignore` shares excludes per project, and inline `bluntcode:ignore` comments dismiss false positives at the source. See [docs/ci.md](docs/ci.md) for the full CI guide.
- **Dark Mode & Keyboard Shortcuts:** Light/dark theme that follows your OS preference, and keyboard-first navigation (`g`+`h/w/t/s` to jump between pages, `/` to search, `?` for the shortcut cheat sheet).
- **Offline Capable:** Once analyzers are downloaded on initial setup, full scans can run 100% offline without internet access.

---

## 💻 System Requirements

| Requirement | Specification |
| :--- | :--- |
| **Operating System** | Windows 10 or Windows 11 (64-bit / amd64) |
| **User Rights** | Standard user account (Administrator privileges **not** required) |
| **Network** | Internet connection required **only on first run** to download managed analyzer tools |
| **Dependencies** | **None!** Global Python, Java, or Node.js installations are **not required** |

---

## 🚀 Installation Options

Choose the method that suits your workflow best:

### Option 1: One-Line Installer (PowerShell or cmd) (Recommended)

This automated script downloads the latest release, verifies its SHA-256 checksum, installs Blunt Code to `%LOCALAPPDATA%\Programs\BluntCode`, creates a **Start Menu** shortcut, and launches the app.

**From PowerShell** (Press `Win + R`, type `powershell`, and press `Enter`):

```powershell
irm https://github.com/sanketpatel32/Blunt-code/releases/latest/download/install-latest.ps1 | iex
```

**From Command Prompt** (Press `Win + R`, type `cmd`, and press `Enter`) — one paste, using the `curl.exe` that ships with Windows 10+:

```bat
curl -fsSL -o "%TEMP%\install-bluntcode.cmd" https://github.com/sanketpatel32/Blunt-code/releases/latest/download/install.cmd && "%TEMP%\install-bluntcode.cmd"
```

Both commands fetch the installer attached to the latest GitHub release (never a moving branch), verify the archive checksum before installing, and require no administrator rights.

---

### Option 2: Standalone ZIP Download (Portable)

If you prefer to download and verify files manually without running a web-based script:

1. Open the [Latest Release Page](https://github.com/sanketpatel32/Blunt-code/releases/latest).
2. Download `BluntCode-<version>-windows-amd64.zip` and `BluntCode-<version>-windows-amd64.zip.sha256`.
3. Open **PowerShell** in your download folder and run:

```powershell
# 1. Verify file integrity
$package = '.\BluntCode-0.5.0-windows-amd64.zip' # Update filename to match download
$expected = (Get-Content "$package.sha256" -Raw).Trim().Split()[0].ToLowerInvariant()
$actual = (Get-FileHash $package -Algorithm SHA256).Hash.ToLowerInvariant()

if ($actual -ne $expected) {
    throw 'Checksum mismatch! Download may be corrupted or modified.'
}
Write-Host 'SHA256 verified successfully!' -ForegroundColor Green

# 2. Extract and launch
Expand-Archive $package -DestinationPath .\BluntCode -Force
Set-Location .\BluntCode\BluntCode*
.\bluntcode.exe
```

> [!TIP]
> **No Execution Policy changes needed:** the recommended one-line installer pipes the script straight into PowerShell (`irm … | iex`) without saving any `.ps1` file, so Windows' script-execution policy never applies to it. The only script you may ever run from disk is `uninstall.ps1`; if Windows blocks that one, run `powershell -ExecutionPolicy Bypass -File .\uninstall.ps1`.

---

### Option 3: Build from Source

For developers and contributors who wish to compile Blunt Code from source:

**Prerequisites:**
- [Go 1.26 or higher](https://go.dev/dl/)
- [Node.js 18 or higher](https://nodejs.org/) with `npm`

```powershell
# Clone the repository
git clone https://github.com/sanketpatel32/Blunt-code.git
Set-Location Blunt-code

# Build frontend & embed into Go binary
.\scripts\build.ps1

# Run the compiled binary
.\bluntcode.exe
```

---

## 🎯 Quick Start Guide

Using Blunt Code takes less than two minutes:

1. **Launch the App:** Start Blunt Code from your Start Menu or by running `bluntcode.exe`. The app starts a loopback server on `127.0.0.1` and opens your browser automatically.
2. **Add Workspace:** Click **Add workspace** on the main screen and choose your project folder.
3. **Configure File Exclusions (Optional):** Click **Configure files** to exclude build outputs, vendor directories, or binary files.
4. **Run Scan:** Click **Run scan**.
   > *Note: On the first scan, Blunt Code will download managed analyzers. Subsequent scans run locally and instantly.*
5. **Inspect & Preview:** View issues grouped by analyzer. Click any finding to inspect line numbers and source code snippets.
6. **Export Report:** Click **Export** and pick a format — **Markdown** (saved to your local reports folder), or **HTML** / **SARIF** / **CSV** (downloaded instantly, CSV honors your active filters).

---

## 🛠️ Managed Analyzers & Languages

Blunt Code isolates and manages analyzer binaries in `%LOCALAPPDATA%\BluntCode\tools`. They never alter your global `PATH`.

| Analyzer | Languages Covered | What It Checks |
| :--- | :--- | :--- |
| **Ruff** | Python | Ultra-fast Python linting, syntax errors, and style violations. |
| **Biome** | JavaScript, TypeScript | Code formatting, correctness, performance, and modern syntax checks. |
| **Semgrep** | Python, JavaScript, TypeScript | Local security SAST rules, vulnerability patterns, and security risks. |
| **SonarQube** | Polyglot (Project-wide) | Deep code quality, security hotspots, code smells, and structural metrics. |

### Scan Profiles

Every scan runs in one of three profiles:

| Profile | Analyzers | Best For |
| :--- | :--- | :--- |
| **Quick** | Ruff + Biome only | Fast feedback between edits. |
| **Standard** | All four analyzers | The everyday default. |
| **Deep** | All four analyzers, with Ruff widened to an extended rule set (`E,W,F,B,SIM,C4,RET,ARG,PLR`) | Thorough pre-release or nightly audits — catches style, bugbear, simplification, comprehension, and refactoring issues on top of the standard checks. |

---

## 🖥️ Command-Line Interface (CLI)

`bluntcode.exe` supports CLI arguments and flags for headless execution, custom ports, or system diagnostics:

```powershell
# Open Blunt Code with a specific project folder loaded
.\bluntcode.exe "C:\Projects\my-python-app"

# Start the server on port 52160 without launching a web browser window
.\bluntcode.exe --no-browser --port 52160

# Run local system diagnostics
.\bluntcode.exe doctor

# Output diagnostic results in structured JSON (useful for troubleshooting)
.\bluntcode.exe doctor --json

# Run one headless scan (no browser, no server) and print a summary
.\bluntcode.exe scan "C:\Projects\my-python-app" --profile quick

# Machine-readable JSON summary for CI pipelines (progress still goes to stderr)
.\bluntcode.exe scan "C:\Projects\my-python-app" --json --quiet --timeout 10m

# CI gate: fail the build (exit 1) on high+ severity findings or more than 50 total
.\bluntcode.exe scan "C:\Projects\my-python-app" --fail-on high+ --max-findings 50

# Gate against a baseline (previous scan ID or exported SARIF) so pre-existing findings don't fail the build
.\bluntcode.exe scan "C:\Projects\my-python-app" --fail-on high+ --baseline .\last-scan.sarif

# Full JSON report on stdout, a SARIF file for baselines / code-scanning uploads, or GitHub Actions annotations for PR checks
.\bluntcode.exe scan "C:\Projects\my-python-app" --format json
.\bluntcode.exe scan "C:\Projects\my-python-app" --format sarif > .\baseline.sarif
.\bluntcode.exe scan "C:\Projects\my-python-app" --format github

# Run up to 2 analyzers concurrently, rescan only files that changed, or keep watching and rescan on file changes
.\bluntcode.exe scan "C:\Projects\my-python-app" --jobs 2
.\bluntcode.exe scan "C:\Projects\my-python-app" --incremental
.\bluntcode.exe scan "C:\Projects\my-python-app" --watch

# Show the effective configuration (resolved paths, overrides, managed tool versions)
.\bluntcode.exe config

# Diagnose the local installation, repairing mechanical problems (stale rules, interrupted installs)
.\bluntcode.exe doctor --fix
```

`bluntcode scan` exits with code `0` when the scan completes (warnings included), `1` when it fails, is cancelled, or a `--fail-on`/`--max-findings` gate trips, `2` for usage errors, and `130` after Ctrl+C — so CI pipelines can gate on it directly. With `--baseline`, the gate only counts findings that are new since the baseline. For the full CI guide — baselines, output formats, GitHub Actions annotations, and a ready-to-use workflow — see [docs/ci.md](docs/ci.md).

---

## 📁 Data Storage & Privacy

Blunt Code is designed from the ground up for **complete data privacy**:

- **Strict Loopback Binding:** The server listens only on `127.0.0.1`.
- **Zero Telemetry:** No user metrics, project code, or telemetry are ever sent to external cloud servers.
- **Local Application Data:** All databases, logs, tools, and reports stay on your local drive.

```text
%LOCALAPPDATA%\BluntCode
├── bluntcode.db       # SQLite database (workspaces, scan history, findings)
├── reports\           # Generated Markdown scan reports
├── logs\              # Redacted diagnostic log files
└── tools\             # Sandboxed managed tool binaries (Ruff, Biome, Semgrep, SonarQube)
```

For the complete data layout, `bluntcode config`, environment overrides, offline mode, and scan profiles, see [docs/configuration.md](docs/configuration.md).

---

## 📚 Documentation

- [docs/ci.md](docs/ci.md) — CI guide: headless scans, exit codes, `--fail-on`/`--max-findings` gates, baselines, output formats (text/JSON/SARIF/GitHub annotations), and a complete GitHub Actions workflow.
- [docs/ignoring-findings.md](docs/ignoring-findings.md) — Inline `bluntcode:ignore` comments, fingerprint-based suppressions, external-tool ignores, and `.bluntcodeignore` patterns.
- [docs/configuration.md](docs/configuration.md) — Data layout, `bluntcode config`, `BLUNTCODE_SONAR_STARTUP_TIMEOUT`, offline mode, and scan profiles.
- [docs/architecture.md](docs/architecture.md) — How the Go backend, analyzer boundary, and React UI fit together.
- [docs/analyzers.md](docs/analyzers.md) — How managed analyzers are pinned, sandboxed, and invoked.
- [docs/privacy.md](docs/privacy.md) and [docs/release.md](docs/release.md) — Privacy guarantees and the release checklist.

---

## ❓ Troubleshooting & FAQ

<details>
<summary><b>1. PowerShell script execution is blocked by Windows.</b></summary>
<br>

The recommended one-line installer (`irm … | iex`) never saves a script to disk, so Windows' script-execution policy does not apply to it and there is nothing to bypass. If a locally saved copy of a Blunt Code script (such as `uninstall.ps1`) is ever blocked, run it with `powershell -ExecutionPolicy Bypass -File .\uninstall.ps1` — the flag applies only to that single process and leaves your system-wide policy intact.
</details>

<details>
<summary><b>2. Why is the initial scan taking longer than expected?</b></summary>
<br>

The very first time an analyzer (like SonarQube or Semgrep) runs, Blunt Code downloads its sandboxed binary package and initializes the local server environment. Subsequent scans use the pre-booted local setup and complete much faster.
</details>

<details>
<summary><b>3. How do I enable Offline Mode?</b></summary>
<br>

Once managed tools are installed during your first scan, open **Settings** in the Blunt Code web interface and toggle **Offline mode** ON. Blunt Code will perform scans using only local assets without making external network requests.
</details>

<details>
<summary><b>4. Error: "Only one Blunt Code process can use the data directory."</b></summary>
<br>

Blunt Code locks its SQLite database to prevent data corruption. If another instance is running in the background, close it via Task Manager or run:
```powershell
.\bluntcode.exe doctor
```
to inspect active local processes.
</details>

<details>
<summary><b>5. Where are my reports, logs, and analyzer tools stored?</b></summary>
<br>

All Blunt Code data lives under `%LOCALAPPDATA%\BluntCode` (see the layout in [📁 Data Storage & Privacy](#-data-storage--privacy)). The easiest way to get there: open **Settings** in the Blunt Code web interface and use the **Data folders** buttons to launch the data, reports, logs, or tools folder directly in Windows Explorer.
</details>

---

## 🗑️ Uninstalling

To uninstall Blunt Code, use the included PowerShell uninstallation script:

```powershell
# Standard Uninstall (Removes app executable & shortcut; keeps your saved scan database & reports)
Set-Location "$env:LOCALAPPDATA\Programs\BluntCode"
.\uninstall.ps1

# Full Cleanup Uninstall (Removes app AND deletes all local scan databases, logs, and settings)
.\uninstall.ps1 -RemoveData
```

---

## 🤝 Contributing

Contributions, bug reports, and feature requests are welcome!

```powershell
# Run backend tests
go test ./...

# Run web UI tests & build check
Set-Location web
npm test
npm run build
Set-Location ..

# Package a release zip & checksum
.\scripts\package.ps1 -Version 0.4.0
```

For guidelines on coding standards and codebase architecture, see [CONTRIBUTING.md](CONTRIBUTING.md) and [docs/architecture.md](docs/architecture.md).

---

## 📜 License

This project is licensed under the **MIT License**. See [LICENSE](LICENSE) for details.
Managed third-party analyzers retain their respective open-source licenses; see [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md).

