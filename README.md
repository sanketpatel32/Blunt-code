# Blunt Code

Blunt Code is a Windows-first, local-only code-analysis application. Choose a
project, analyze it, and read one combined report from Ruff, Biome, Semgrep,
and managed SonarQube.

It does not upload source code, findings, or reports; after initial managed
tool setup, scans can run offline. Ruff, Biome, Semgrep, Java, SonarScanner,
and SonarQube are version-pinned and installed only under Blunt Code’s local
app-data folder. Semgrep runs only bundled local rules. See
[docs/analyzers.md](docs/analyzers.md) and [docs/release.md](docs/release.md)
for exact privacy, offline, and validation limits.

## Development

Run `go test ./...` for the backend suite. Run `./scripts/build.ps1` on
Windows to build Vite, copy its output into the Go embed directory, and produce
`bluntcode.exe`. Run `./scripts/package.ps1` to create a checksum file and a
Windows ZIP. Install a release ZIP with
`./scripts/install.ps1 -PackagePath <zip> -Sha256 <sha256>`.
For a hosted release, use the same checksum gate without trusting the download:
`./scripts/install.ps1 -PackageUrl https://.../BluntCode-...zip -Sha256Url https://.../BluntCode-...zip.sha256 -AddToPath`.

Run `bluntcode doctor` (or `bluntcode doctor --json`) for local diagnostics.
It checks the data directory, free disk space, SQLite migrations, loopback binding, and managed
tool readiness without downloading, installing, starting an analyzer, or
sending diagnostics anywhere. Only one backend can use a given data directory
at a time.

Analyzer command output is retained only as bounded, redacted local diagnostic
logs under the app-data directory; no source is uploaded.

## License

MIT. Third-party tools retain their own licenses; see THIRD_PARTY_NOTICES.md.
