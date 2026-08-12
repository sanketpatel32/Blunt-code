# Analyzers

Ruff handles Python, Biome handles JavaScript/TypeScript, Semgrep supplies an
optional local ruleset, and SonarQube provides broader project analysis.

All are invoked as direct processes with separate arguments. Blunt Code does
not use a shell, does not modify user code, and records normalized results.

## Managed Semgrep

Semgrep 1.172.0 is installed privately under the Blunt Code application data
directory. Setup first downloads the checksum-pinned official Windows x64 uv
0.11.16 zip, then downloads the SHA-256-verified Semgrep Windows wheel into
that private directory. The manifest identifies the wheel as
`semgrep==1.172.0`. It runs its managed `uv.exe` directly with:

```text
uv tool install --managed-python <verified-local-semgrep-1.172.0.whl>
```

`UV_TOOL_DIR`, `UV_TOOL_BIN_DIR`, `UV_CACHE_DIR`, and
`UV_PYTHON_INSTALL_DIR` all point below
`%LOCALAPPDATA%\BluntCode\tools\semgrep\1.172.0`. No global PATH, user uv
directory, user Python, or system tool installation is used.

Setup also extracts the bundled Blunt Code local rules pack (version 1.0.0) to
`%LOCALAPPDATA%\BluntCode\tools\semgrep\1.172.0\rules`. Semgrep scans use
only that directory as `--config`; they never use `auto`, a registry pack, or
a remote rules URL. A missing local rules pack can be repaired offline when
the managed executable is already present.

Every Semgrep scan passes `--metrics=off`, `--disable-version-check`, and
`--oss-only`, with `SEMGREP_SEND_METRICS=off`,
`SEMGREP_ENABLE_VERSION_CHECK=0`, an empty `SEMGREP_APP_TOKEN`, and a private
`SEMGREP_SETTINGS_FILE`. This prevents inherited account settings, metrics,
and update checks from participating in source analysis.

The first managed install needs network access for uv's managed Python and
Semgrep's transitive Python dependencies; the primary Semgrep wheel is already
verified locally before uv receives it. A release still needs a real
disconnected Windows verification after setup and a transitive package
dependency integrity review; unit tests deliberately use a fake uv process and
do not download Semgrep.

SonarQube is managed as a loopback-only child dependency. Its adapter has
explicit installer, runtime, server, and API-client interfaces. The shipping
release must pin, checksum, and compatibility-test its SonarQube, scanner, and
Java artifacts before enabling installation.

The current implementation creates scanner properties only under Blunt Code
application data and deletes them after the scanner exits. It fails with a
specific readiness error until the release-owned SonarQube artifacts and a
securely bootstrapped token are present; it does not substitute global PATH,
the user's Java installation, or placeholder credentials.
