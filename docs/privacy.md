# Privacy

Blunt Code analyzes source locally. It has no account, telemetry, analytics,
crash reporting, AI provider, or Blunt Code cloud backend. Source code,
findings, and reports are never uploaded. The server binds only to
`127.0.0.1`, and all data lives under `%LOCALAPPDATA%\BluntCode`.

## Outbound network requests

These are the only requests Blunt Code makes to hosts other than loopback. The
full table, with when each one happens, is in
[analyzers.md → Network use](analyzers.md#network-use).

| Request | Host | What is sent | When |
| :--- | :--- | :--- | :--- |
| Managed tool downloads | `github.com` / `objects.githubusercontent.com`, `files.pythonhosted.org` / `pypi.org`, `binaries.sonarsource.com`, plus the Python runtime and packages uv fetches for Semgrep and Checkov | Plain HTTPS downloads of the pinned artifacts in `internal/tools/manifest.json` | On Install/Repair/Update in Tools, or when a scan needs a missing tool |
| OSV vulnerability lookups | `api.osv.dev` (OSV-Scanner) | Package names and versions from your lockfiles, never source code | Deep scans, unless offline databases are provided |
| Trivy vulnerability database | Trivy's default database registry (`mirror.gcr.io`, falling back to `ghcr.io`) | A database download; nothing from your project | First deep scan with a cold Trivy cache |
| Update check | `api.github.com` | A request for the latest Blunt Code release; the version comparison happens locally | Once each time the web UI is opened, on the About page, and with `bluntcode update check` |
| Update apply | `api.github.com`, `raw.githubusercontent.com`, `github.com` | Download of the release's `install-latest.ps1`, ZIP and checksum | Only when you choose to apply an update |
| Pentest probe | The URL you enter | HTTP requests to that target only | Only when you run a probe |

Semgrep runs with `--metrics=off`, `--disable-version-check` and an empty app
token, so it does not report to Semgrep's servers.

## Offline mode

Turn on **Settings → Offline mode** to refuse every request above: tool
installs, the update check, OSV lookups without a local database, and
cold-cache Trivy scans fail with a clear message instead of reaching the
network. Scans with already-installed tools keep working offline. See
[configuration.md → Offline mode](configuration.md#offline-mode).
