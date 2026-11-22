# Privacy

Blunt Code analyzes source locally. It has no account, telemetry, analytics,
AI provider, or Blunt Code cloud backend. Source, findings, reports, and your
dependency inventory are never uploaded.

## Outbound network requests

Blunt Code does make a small number of outbound requests. All of them are
listed here; anything not on this list is a bug.

| Request | Destination | When | What it reveals |
| :--- | :--- | :--- | :--- |
| Tool install / repair / update | `github.com`, `pypi.org` / `files.pythonhosted.org`, `binaries.sonarsource.com` (pinned versions) | You press Install/Repair/Update on the Tools page, or a scan needs a managed tool that is not installed yet | Your IP address and which tool you are installing |
| Trivy vulnerability database | `mirror.gcr.io` / `ghcr.io` | First deep scan only (cold cache); later scans reuse the cached DB | Your IP address |
| OSV vulnerability databases | `osv-vulnerabilities.storage.googleapis.com` | Deep scans (not in offline mode) refresh the public per-ecosystem archives, e.g. `npm/all.zip` | Your IP address and which ecosystems (npm, PyPI, Go, ...) your project uses — never package names or versions |
| Update check | `api.github.com/repos/sanketpatel32/Blunt-code/releases/latest` | Only when you click **Check for updates** on the About page, run `bluntcode update check`, or turn on **Settings → Check for updates automatically** (off by default) | Your IP address, the `BluntCode-Updater` user agent, and when you launched the app |
| Update apply | `api.github.com`, `raw.githubusercontent.com` | Only when you click **Update** | Same as the update check |

Bundled analyzers run with their own phone-home features switched off:

- **Trivy** — `--disable-telemetry --skip-version-check --skip-check-update
  --offline-scan`: no usage data to Aqua, no version/announcement check, no
  checks-bundle download (the misconfiguration checks embedded in the pinned
  Trivy binary are used), and no package-registry lookups. The only remaining
  request is the cold-cache database download above.
- **Checkov** — `--skip-download`, `BC_SKIP_MAPPING=TRUE`,
  `CKV_SKIP_PACKAGE_UPDATE_CHECK=true`, Prisma Cloud credentials cleared: no
  Prisma Cloud metadata download and no PyPI version check. Checkov findings
  therefore carry Blunt Code's default severities and no guideline link.
- **OSV Scanner** — `--offline --offline-vulnerabilities`: matching happens
  against the local databases, so the dependency inventory is never sent to
  the OSV.dev query API, deps.dev, or package registries (transitive
  resolution of manifests without a lockfile is off).
- **Semgrep** — bundled local rules, metrics off. **SonarQube** — loopback
  only, telemetry and update center off. **Ruff, Biome, Gitleaks** and the
  built-in analyzers make no network requests.

## Offline mode

**Settings → Offline mode** blocks every request in the table above: no tool
installs, no database downloads, no update checks. Deep scans then need the
Trivy and OSV databases from an earlier online deep scan and report "not
ready" instead of dialing out. See [configuration.md](configuration.md#offline-mode).
