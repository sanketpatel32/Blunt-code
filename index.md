# Blunt Code — your code has problems. Here they are.

> Eleven analyzers, one honest report, zero uploads. Local code quality & security for Windows — Ruff, Biome, Semgrep, SonarQube, gitleaks, Trivy, OSV, Checkov, secrets, TODO and license scanning in one loopback app. No cloud, no account, no telemetry.

## The short version

Your code flows through one local executable — `bluntcode.exe` — to eleven bundled analyzers, and comes back as one blunt report: every finding named, severity-graded, and filed in a local SQLite database. Everything stays inside your machine. There is no upload.

- **Analyzers bundled:** Ruff (Python), Biome (JS/TS + React), Semgrep (25-rule security pack), SonarQube (polyglot, managed runtime), Secrets (8 credential families), TODO/FIXME (40+ file types), gitleaks (repo-wide secrets), OSV Scanner + Trivy (dependency CVEs, deep scans), Checkov (infrastructure-as-code, deep scans), License Scanner (copyleft/conflict detection).
- **Two ways to scan:** Blunt Code scans locally — code, analyzers, and the report never leave your disk. Cloud scanners require uploading your source to someone else's server.
- **Numbers:** 11 bundled analyzers · 726 automated tests passing (457 Go · 269 web) · 0 bytes of telemetry · 40+ file types scanned.
- **Statement:** Scan everything. Send nothing.

## Install in 30 seconds

PowerShell:

```
irm https://github.com/sanketpatel32/Blunt-code/releases/latest/download/install-latest.ps1 | iex
```

CMD:

```
curl -fsSL -o "%TEMP%\install-bluntcode.cmd" https://github.com/sanketpatel32/Blunt-code/releases/latest/download/install.cmd && "%TEMP%\install-bluntcode.cmd"
```

Or grab the [portable ZIP + SHA-256](https://github.com/sanketpatel32/Blunt-code/releases/latest). Sandboxed under `%LOCALAPPDATA%` — your PATH is never touched. Rerunning the installer upgrades in place.

## Links

- [GitHub repository](https://github.com/sanketpatel32/Blunt-code)
- [Releases](https://github.com/sanketpatel32/Blunt-code/releases)
- [Changelog](https://github.com/sanketpatel32/Blunt-code/blob/main/CHANGELOG.md)
- [About](https://sanketpatel32.github.io/Blunt-code/about/) · [Privacy](https://sanketpatel32.github.io/Blunt-code/privacy/) · [Contact](https://sanketpatel32.github.io/Blunt-code/contact/)

---

This is the markdown edition of the landing page. The visual version lives at <https://sanketpatel32.github.io/Blunt-code/>. Agent guidance: see [llms.txt](https://sanketpatel32.github.io/Blunt-code/llms.txt).
