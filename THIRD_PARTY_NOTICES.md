# Third-party notices

Blunt Code itself is MIT licensed (see [LICENSE](LICENSE)). This file covers
two different kinds of third-party software:

1. **Code compiled into `bluntcode.exe`** — Go modules and the web UI's npm
   runtime packages. These *are* redistributed in every release ZIP, and their
   notices are reproduced or referenced below.
2. **Managed analyzer tools** — Ruff, Biome, Gitleaks, Semgrep, SonarQube and
   the rest. These are **not** included in the release ZIP. The ZIP built by
   `scripts/package.ps1` contains only `bluntcode.exe`, `README.md`, `LICENSE`,
   this file and `uninstall.ps1`. Each managed tool is downloaded, unmodified,
   on the user's machine directly from the upstream publisher's URL pinned in
   [`internal/tools/manifest.json`](internal/tools/manifest.json), verified
   against the SHA-256 pinned there, and installed under
   `%LOCALAPPDATA%\BluntCode\tools`. Blunt Code runs each tool as a separate
   process with command-line arguments; it does not link to, patch, or
   recompile any of them. Each tool is used under its own upstream license,
   listed below for the exact version that release pins.

Everything in this file was checked against the upstream license files and
package metadata for the versions pinned in `internal/tools/manifest.json` and
`go.mod` / `web/package-lock.json` at Blunt Code 0.50.0. When a pin changes,
update the matching row here in the same commit.

## 1. Managed analyzer tools (downloaded at install time, not redistributed)

| Tool | Pinned version | License (SPDX) | Upstream source | License text |
| :--- | :--- | :--- | :--- | :--- |
| Ruff | 0.16.0 | MIT | [astral-sh/ruff](https://github.com/astral-sh/ruff/releases/tag/0.16.0) | [LICENSE](https://github.com/astral-sh/ruff/blob/0.16.0/LICENSE) |
| Biome | 2.5.6 | MIT OR Apache-2.0 | [biomejs/biome](https://github.com/biomejs/biome/releases/tag/%40biomejs%2Fbiome%402.5.6) | [LICENSE-MIT](https://github.com/biomejs/biome/blob/%40biomejs/biome%402.5.6/LICENSE-MIT), [LICENSE-APACHE](https://github.com/biomejs/biome/blob/%40biomejs/biome%402.5.6/LICENSE-APACHE) |
| Gitleaks | 8.30.1 | MIT | [gitleaks/gitleaks](https://github.com/gitleaks/gitleaks/releases/tag/v8.30.1) | [LICENSE](https://github.com/gitleaks/gitleaks/blob/v8.30.1/LICENSE) |
| OSV-Scanner | 2.5.1 | Apache-2.0 | [google/osv-scanner](https://github.com/google/osv-scanner/releases/tag/v2.5.1) | [LICENSE](https://github.com/google/osv-scanner/blob/v2.5.1/LICENSE) |
| Trivy | 0.74.0 | Apache-2.0 | [aquasecurity/trivy](https://github.com/aquasecurity/trivy/releases/tag/v0.74.0) | [LICENSE](https://github.com/aquasecurity/trivy/blob/v0.74.0/LICENSE) |
| Checkov | 3.3.16 | Apache-2.0 | [PyPI `checkov==3.3.16`](https://pypi.org/project/checkov/3.3.16/) | [LICENSE](https://github.com/bridgecrewio/checkov/blob/3.3.16/LICENSE) |
| uv | 0.11.16 | MIT OR Apache-2.0 | [astral-sh/uv](https://github.com/astral-sh/uv/releases/tag/0.11.16) | [LICENSE-MIT](https://github.com/astral-sh/uv/blob/0.11.16/LICENSE-MIT), [LICENSE-APACHE](https://github.com/astral-sh/uv/blob/0.11.16/LICENSE-APACHE) |
| Semgrep (CE) | 1.172.0 | LGPL-2.1-or-later | [PyPI `semgrep==1.172.0`](https://pypi.org/project/semgrep/1.172.0/) (Windows wheel) | [LICENSE](https://github.com/semgrep/semgrep/blob/v1.172.0/LICENSE) |
| SonarQube Community Build | 26.8.0.126808 | LGPL-3.0 platform; bundled analyzers SSALv1 or LGPL-3.0 (see below); embedded Elasticsearch under Elastic-2.0 | [binaries.sonarsource.com](https://binaries.sonarsource.com/Distribution/sonarqube/sonarqube-26.8.0.126808.zip) | [LICENSE.txt](https://github.com/SonarSource/sonarqube/blob/26.8.0.126808/LICENSE.txt), [SSALv1](https://www.sonarsource.com/license/ssal/) |
| SonarScanner CLI | 7.3.0.5189 | LGPL-3.0; its bundled JRE (Eclipse Temurin 17.0.15+6) is GPL-2.0 WITH Classpath-exception-2.0 | [binaries.sonarsource.com](https://binaries.sonarsource.com/Distribution/sonar-scanner-cli/sonar-scanner-cli-7.3.0.5189-windows-x64.zip) | [LICENSE.txt](https://github.com/SonarSource/sonar-scanner-cli/blob/7.3.0.5189/LICENSE.txt) |
| Eclipse Temurin JDK | 21.0.12+8 | GPL-2.0 WITH Classpath-exception-2.0 | [adoptium/temurin21-binaries](https://github.com/adoptium/temurin21-binaries/releases/tag/jdk-21.0.12%2B8) | [OpenJDK LICENSE](https://github.com/openjdk/jdk21u/blob/jdk-21.0.12%2B8/LICENSE); full notices ship in the JDK's `legal/` directory |

### Semgrep (LGPL-2.1-or-later)

- What is installed: the official, unmodified `semgrep==1.172.0` Windows wheel
  from PyPI (SHA-256 pinned in the manifest), which contains the Python CLI and
  the compiled `semgrep-core.exe`. It is installed by uv into a private
  directory and run only as a separate process.
- License text: the wheel itself carries the full LGPL-2.1 text at
  `semgrep-1.172.0.dist-info/licenses/LICENSE`, which stays in place after
  install. Blunt Code does not remove or edit it.
- Source: the corresponding source for this exact version is
  [semgrep/semgrep at tag `v1.172.0`](https://github.com/semgrep/semgrep/tree/v1.172.0).
  Blunt Code makes no modifications to Semgrep, so the upstream source is the
  complete corresponding source.
- Blunt Code is not a derivative work of Semgrep and does not link to it; it
  passes arguments to the executable and reads its JSON output. Replacing the
  installed Semgrep with another build (the user's LGPL right) needs no change
  to Blunt Code.
- Semgrep's transitive Python dependencies (listed in the wheel's `METADATA`
  `Requires-Dist`, e.g. `click`, `requests`, `rich`, `ruamel.yaml`,
  `opentelemetry-*`, `pywin32`) are resolved and downloaded by uv from PyPI at
  install time. Each is installed unmodified with its own license files under
  its `*.dist-info` directory.
- The bundled `blunt-code-local.yaml` Semgrep rules are Blunt Code source and
  are covered by this repository's MIT license. The SonarSource, Semgrep and
  Checkov rule sets that ship inside those tools stay under their own licenses.

### SonarQube Community Build 26.8.0.126808

The SonarQube ZIP's own `COPYING` file says: *"SonarQube Community Build is
released under the LGPLv3 license, and packaged with SSALv1 analyzers."* The
plugin manifests in `lib/extensions/` of this exact build show:

| License | Bundled analyzers (plugin version) |
| :--- | :--- |
| SSALv1 (Sonar Source-Available License v1.0) | C# and VB.NET 10.31.0, Flex 2.18.0, HTML 3.30.0, Java 8.37.0, Java Symbolic Execution 8.16.4, JavaScript/TypeScript/CSS 13.4.0, PHP 3.59.0, Python 5.27.0, Kotlin 3.7.0, Ruby 1.24.0, Scala 1.24.0, XML 2.18.0, IaC 2.13.0, Text 2.47.0 |
| LGPL-3.0 | Go 1.40.0, JaCoCo 1.6.0, Clean as You Code 2.6.0 |
| Not stated in the plugin manifest | Rust 1.8.0 (covered by the SSALv1 statement in `COPYING`) |

The embedded Elasticsearch under `elasticsearch/` is distributed under the
Elastic License 2.0 (`elasticsearch/LICENSE.txt` in the ZIP). Third-party Java
libraries inside SonarQube are listed with their licenses in
`licenses/dependency-license.json` in the ZIP.

**Redistribution:** Blunt Code does not redistribute SonarQube, SonarScanner or
Java. The user's machine downloads them directly from SonarSource and Adoptium,
so they reach the user under the publishers' own terms, with every upstream
notice intact. Anyone who repackages a Blunt Code install with the tools
already in it (an offline mirror, a pre-provisioned image) *is* redistributing
them and must follow each license: for SSALv1, section 3 requires a statement
that the source is available under SSALv1, information on how to get it, and
no removal of notices.

**Use restriction to be aware of:** SSALv1 grants its rights only for a
"Non-competitive Purpose", as defined in section 1 of the license. That
definition excludes offering a product with the same or substantially similar
functionality as SonarQube, and using AI technology that is not part of the
Program to ingest or analyze the data the Program produces. Blunt Code runs
SonarQube as one of several local analyzers on the user's own machine. Users
who are unsure whether their use (for example, feeding SonarQube-derived
findings to an AI agent) fits that definition should read the
[SSALv1 text](https://www.sonarsource.com/license/ssal/) or turn off the
SonarQube analyzer. SonarQube is a trademark of SonarSource SA. Blunt Code is
not affiliated with or endorsed by SonarSource.

### Python runtime used by Semgrep and Checkov

uv installs a managed CPython build from
[astral-sh/python-build-standalone](https://github.com/astral-sh/python-build-standalone)
into Blunt Code's private tools directory. CPython is under the PSF-2.0
license. The libraries it is built with (OpenSSL, SQLite, zlib and others) keep
their own licenses, which ship with the build. Checkov's transitive PyPI
dependencies are resolved and installed by uv the same way as Semgrep's.

## 2. Code compiled into `bluntcode.exe` (redistributed in the release ZIP)

### Go modules (from `go.mod`)

| Module | Version | License (SPDX) |
| :--- | :--- | :--- |
| golang.org/x/sys | v0.37.0 | BSD-3-Clause |
| golang.org/x/exp | v0.0.0-20251023183803-a4bb9ffd2546 | BSD-3-Clause |
| modernc.org/sqlite | v1.46.1 | BSD-3-Clause (contains SQLite, which is public domain) |
| modernc.org/libc | v1.67.6 | BSD-3-Clause (third-party components listed in its `LICENSE-3RD-PARTY.md`) |
| modernc.org/mathutil | v1.7.1 | BSD-3-Clause |
| modernc.org/memory | v1.11.0 | BSD-3-Clause |
| github.com/dustin/go-humanize | v1.0.1 | MIT |
| github.com/google/uuid | v1.6.0 | BSD-3-Clause |
| github.com/mattn/go-isatty | v0.0.20 | MIT |
| github.com/ncruces/go-strftime | v1.0.0 | MIT |
| github.com/remyoudompheng/bigfft | v0.0.0-20230129092748-24d4a6f8daec | BSD-3-Clause |

The Go standard library and runtime linked into the binary are BSD-3-Clause
([go.dev/LICENSE](https://go.dev/LICENSE)).

### Web UI runtime packages (from `web/package-lock.json`)

The embedded web UI bundles 98 npm packages: the direct `dependencies` in
`web/package.json` and their transitive runtime dependencies. Dev-only tooling
(Vite, Vitest, TypeScript, Tailwind, Playwright and others) is not shipped.
Licenses, taken from the lockfile:

| License (SPDX) | Packages |
| :--- | :--- |
| MIT | 82 packages, including `react`, `react-dom`, `@radix-ui/*`, `recharts`, `motion`, `clsx`, `tailwind-merge` |
| ISC | `lucide-react`, `d3-array`, `d3-color`, `d3-format`, `d3-interpolate`, `d3-path`, `d3-scale`, `d3-shape`, `d3-time`, `d3-time-format`, `d3-timer`, `internmap` |
| Apache-2.0 | `class-variance-authority` |
| BSD-3-Clause | `d3-ease` |
| 0BSD | `tslib` |
| MIT AND ISC AND BSD-3-Clause | `victory-vendor` |

To list every package with its exact version and license, run from `web/`:
`npm ls --omit=dev --all --long`, or read the `license` fields of the
non-`dev` entries in `package-lock.json`.

## Maintaining this file

- A change to any `version`, `source_url` or `license_url` in
  `internal/tools/manifest.json`, to `go.mod`, or to the runtime
  `dependencies` in `web/package.json` must update this file in the same
  commit. Check the upstream license at the new tag, not at `main`.
- Do not add a managed tool to the release ZIP without first adding its full
  license text and notices to the ZIP and updating section 1 above.
