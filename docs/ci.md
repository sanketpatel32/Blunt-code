# CI guide

`bluntcode scan` runs one full scan headlessly — no browser, no server — so
the same analyzer pipeline the desktop app uses can gate a build. Progress
streams to stderr, the summary (or report document) goes to stdout, and the
exit code reflects the scan outcome.

The executable is Windows-only today: data directories resolve under
`LOCALAPPDATA`, and the managed analyzers are windows-amd64 binaries. Run it
on a Windows runner (`windows-latest`) in hosted CI.

## Exit codes

| Code | Meaning |
| :--- | :--- |
| `0` | Scan completed (warnings included — a failed analyzer does not fail the build) |
| `1` | Scan failed, was cancelled, or timed out — or a `--fail-on`/`--max-findings` gate tripped |
| `2` | Usage error: bad flags, an invalid `--fail-on`/`--max-findings`/`--jobs` value, or an unknown `--baseline` (reported before any scan starts) |
| `130` | Stopped with Ctrl+C |

With `--watch`, the gate never exits the process: gate failures print per
scan on stderr and the loop keeps watching. Only Ctrl+C (exit 130) stops it.

The data directory is single-instance locked, exactly like the app. A CI step
that runs while another Blunt Code process holds the lock exits 1 with a
clear message; redirect `LOCALAPPDATA` (see the workflow below) to isolate CI
state completely.

## The gate: `--fail-on` and `--max-findings`

```powershell
# Fail when any unresolved finding is high or critical
.\bluntcode.exe scan "C:\Projects\my-app" --fail-on high+

# Fail when the scan reports more than 50 unresolved findings in total
.\bluntcode.exe scan "C:\Projects\my-app" --max-findings 50

# Both, with a quiet compact JSON summary
.\bluntcode.exe scan "C:\Projects\my-app" --fail-on high+ --max-findings 50 --json --quiet
```

`--fail-on` accepts a comma-separated list of severities: `critical`, `high`,
`medium`, `low`, `info` (case-insensitive, surrounding spaces ignored). A
trailing `+` means "this severity and above", so `high+` is critical and
high. The forms compose:

```text
high+              critical and high
high+,low          critical, high, and low
critical,high      exactly those two severities
```

An invalid value (unknown severity, empty set) is a usage error and exits 2
before any analyzer runs. When the gate trips, the scan exits 1 with a
one-line `fail:` explanation on stderr, for example
`fail: 12 finding(s) at or above high (gate: --fail-on high+)`.

The gate counts only unresolved findings: resolved (fixed) findings and
fingerprint-suppressed findings are excluded, and with `--baseline` only
findings whose fingerprints are new since the baseline count.

## Baselines: adopt the gate without drowning in old debt

`--baseline` takes either the ID of a previous scan of the same workspace or
the path of a SARIF 2.1.0 file:

```powershell
# Reference a previous scan by ID
.\bluntcode.exe scan "C:\Projects\my-app" --fail-on high+ --baseline 0192f0c1-...

# Reference an exported SARIF file (committable, machine-independent)
.\bluntcode.exe scan "C:\Projects\my-app" --fail-on high+ --baseline .\baseline.sarif
```

The usual workflow is the SARIF round-trip, because it needs no database and
survives across machines:

```powershell
# 1. Capture today's findings as the accepted baseline
.\bluntcode.exe scan "C:\Projects\my-app" --format sarif > baseline.sarif

# 2. Commit baseline.sarif, then gate only on new findings
.\bluntcode.exe scan "C:\Projects\my-app" --fail-on high+ --baseline .\baseline.sarif
```

Every scan in baseline mode prints a one-line stderr summary —
`baseline: N known finding(s) excluded from gate, M new finding(s)` — even
without gate flags. Refresh the committed file whenever the team decides to
accept the new level of findings.

Redirection encoding note: the SARIF reader expects UTF-8. Plain `>` works in
PowerShell 7+ and in bash (`./bluntcode.exe scan . --format sarif >
baseline.sarif`). Windows PowerShell 5.1 writes UTF-16 with `>`, which will
not load — redirect through cmd instead:

```powershell
cmd /c ".\bluntcode.exe scan C:\Projects\my-app --format sarif > baseline.sarif"
```

Blunt Code's own SARIF embeds each finding's fingerprint, so its exports
round-trip exactly. Foreign SARIF logs also load as baselines when their
results carry fingerprints; results without any fingerprint are skipped
because they cannot be matched. An unknown scan ID, or an unreadable or
invalid SARIF path, is a usage error (exit 2) before the scan starts — and a
baseline scan ID from a different workspace is rejected the same way.

## Output formats

`--format` selects what lands on stdout; progress and gate/baseline
summaries always stay on stderr, so every format composes with the gate
flags.

| Format | What it prints |
| :--- | :--- |
| `text` (default) | The human summary: severity counts, new/fixed/persistent versus the previous scan, the most serious findings, per-analyzer results, report path |
| `json` | The full versioned JSON report document (`bluntcode/scan-report`, schema version 1) — the same bytes `GET /api/v1/scans/{id}/findings.json` serves |
| `sarif` | The SARIF 2.1.0 log — byte-for-byte the serialization `GET /api/v1/scans/{id}/report.sarif` serves; feeds GitHub code scanning and the baseline round-trip |
| `csv` | The findings spreadsheet — UTF-8 BOM, same columns and formula-neutralization as `GET /api/v1/scans/{id}/findings.csv` |
| `github` | GitHub Actions workflow-command annotations (see below) |

`--json` (without `--format`) is a different, compact machine summary and
cannot be combined with `--format json`, `github`, or `sarif`.

**Writing documents to files:** `--output FILE` redirects any document format
(`json`, `github`, `sarif`, `csv`) to a file while progress stays on stderr,
and `--save-baseline FILE` writes the SARIF baseline after any completed scan —
`--save-baseline baseline.sarif` is exactly `--format sarif > baseline.sarif`
without the shell redirect.

**Scoped gates:** `--gate-analyzer semgrep,secrets` and/or
`--gate-category security` count only matching findings toward
`--fail-on`/`--max-findings`. The scope is reported on stderr together with
the gate result so a tripped build explains what it counted.

In `--watch` mode, the `json` and `sarif` formats emit one complete,
newline-separated document per rescan; `--format github` is rejected in
watch mode because annotations make no sense in a loop.

### `--format github`: inline PR annotations

Inside a GitHub Actions step, workflow commands on stdout become inline
annotations on files and pull requests. Severity maps to annotation level:
critical and high become `error`, medium `warning`, low and info `notice`.
GitHub's runner displays at most 10 annotations of each type per step, so
the format truncates overflowing types severity-first and closes with one
`notice` carrying the full counts.

```yaml
- name: Scan with PR annotations and a gate
  shell: pwsh
  run: .\bin\bluntcode.exe scan src --profile standard --format github --fail-on high+
```

## `--jobs`: bounded analyzer parallelism

`--jobs N` (positive integer; anything else is a usage error, exit 2) runs at
most N analyzer pipelines concurrently. The default stays fully sequential;
`--jobs 2` overlaps tool runtimes, which helps most when SonarQube's long
compute-engine waits serialize behind the other analyzers.

## `--incremental`: rescan only what changed

`--incremental` re-runs analyzers only on files that changed since the
workspace's last completed scan (content-hashed with SHA-256 at scan time)
and copies findings for unchanged files into the new scan with their
fingerprints intact, so suppression and the new/fixed/persistent comparison
keep working. Reuse is invalidated automatically whenever the analyzer set,
an analyzer version, the profile, or the Blunt Code version changes, and any
failure to read hashes silently degrades to a full scan. A scan note records
what happened: `incremental: reused findings for N unchanged file(s), ran
analyzers on M file(s)`.

```powershell
.\bluntcode.exe scan "C:\Projects\my-app" --incremental --fail-on high+
```

`--watch` turns incremental on by itself from its second scan onward, so the
flag matters for one-shot CI runs over a warm workspace.

## A complete GitHub Actions workflow

A complete, ready-to-use pipeline: build from source, cache the managed
tools, isolate CI state by redirecting `LOCALAPPDATA` to a workspace-local
directory, scan with a gate, and upload the generated Markdown report when
the gate trips.

```yaml
name: CI

on:
  push:
    branches: [main]
  pull_request:

permissions:
  contents: read

jobs:
  scan:
    name: Blunt Code scan (Windows)
    runs-on: windows-latest
    timeout-minutes: 20
    steps:
      - uses: actions/checkout@v6

      - uses: actions/setup-go@v6
        with:
          go-version-file: go.mod
          cache: true
          cache-dependency-path: go.sum

      - name: Build scanner executable
        shell: pwsh
        run: go build -o bin/bluntcode.exe ./cmd/bluntcode

      # Missing managed tools download into <LOCALAPPDATA>\BluntCode\tools on
      # first use; cache that directory keyed on the pinned tool versions so
      # later runs skip the download.
      - name: Cache managed analyzer tools
        uses: actions/cache@v5
        with:
          path: .blunt-ci-appdata/BluntCode/tools
          key: blunt-managed-tools-${{ runner.os }}-${{ hashFiles('internal/tools/manifest.json') }}

      # Redirecting LOCALAPPDATA keeps the scan's database, logs, and tools in
      # a throwaway workspace-local directory: CI state never leaks into a
      # real profile and never collides with another step.
      - name: Scan with a severity gate
        shell: pwsh
        env:
          LOCALAPPDATA: '${{ github.workspace }}\.blunt-ci-appdata'
        run: .\bin\bluntcode.exe scan src --profile quick --fail-on high+ --quiet

      # Every scan writes one Markdown report to
      # <LOCALAPPDATA>\BluntCode\reports\blunt-code-<workspace>-<timestamp>.md.
      # !cancelled() keeps the upload when the gate trips — exactly when
      # someone needs the report.
      - name: Upload scan report
        if: ${{ !cancelled() }}
        uses: actions/upload-artifact@v6
        with:
          name: blunt-code-scan-report
          path: .blunt-ci-appdata/BluntCode/reports/blunt-code-*.md
          retention-days: 7
          if-no-files-found: error
```

To gate on new findings only, add the baseline round-trip before the scan
step, commit `baseline.sarif` at the repository root, and pass
`--baseline baseline.sarif`. To emit PR annotations, add `--format github`
to the scan step. To upload a SARIF for GitHub code scanning instead, run a
second step capturing stdout and feed it to `github/codeql-action/upload-sarif`.

Related pages: [ignoring-findings.md](ignoring-findings.md) for suppression
and ignore files, [configuration.md](configuration.md) for data layout,
profiles, and environment variables.
