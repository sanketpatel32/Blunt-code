# Release checklist

- If `internal/build/version.go` or the logo (`web/public/bluntcode-mark.svg`) changed, refresh the
  embedded Windows resources with `scripts\embed-windows-resources.ps1` and rebuild, so the exe's
  icon and its Properties-sheet version stamp stay current.
- Verify each Windows artifact URL, SHA-256, version, license, and notice.
- Re-test Ruff, Biome, Semgrep local rules, SonarQube bootstrap/scanner/API, and managed Java together.
- For Semgrep, verify the official uv 0.11.16 archive SHA-256, the exact
  `semgrep==1.172.0` dependency resolution, private uv directories, a
  disconnected local-rules scan, and no telemetry/version-check request.
- Do not publish a manifest containing `latest` or an unverified checksum.
- Verify loopback binding, installer signature/checksum, offline analysis, and report path redaction.

## Managed SonarQube Windows lifecycle gate

Before shipping the pinned `26.8.0.126808` Community Build,
`7.3.0.5189` scanner, and Temurin `21.0.12+8` bundle, run this on a clean
Windows account (no system Java and no existing SonarQube service):

1. Start Blunt Code with an empty `%LOCALAPPDATA%\BluntCode`, install the
   SonarQube bundle through the local Tools API, and verify each downloaded
   archive matches the embedded SHA-256. Confirm every extracted file lives
   below `%LOCALAPPDATA%\BluntCode\tools`; nothing is written to the workspace.
2. Start a normal Python or JS/TS scan. Confirm the generated
   `%LOCALAPPDATA%\BluntCode\sonarqube\conf\sonar.properties` has
   `sonar.web.host=127.0.0.1` and a dynamically selected unprivileged port.
   `netstat -ano` must show the Sonar process listening only on `127.0.0.1`.
3. Verify first startup changes the default local administrator password and
   generates one scanner token. The token and password must exist only inside
   `credentials.dpapi`; use a second launch to prove DPAPI restore works and
   confirm neither secret appears in logs, report files, or scanner command
   events.
4. Verify scanner completion, compute-engine polling, issue retrieval, metric
   retrieval, cancellation, and clean application shutdown. Restart Blunt Code
   and repeat a scan to prove the server data and project keys are reusable.
5. Disconnect the network after installation and repeat a scan. It must make
   no non-loopback connections; missing bundles in offline mode must fail with
   a local dependency message rather than downloading.
6. Record the Community Build license review and confirm the selected release
   supports the managed embedded/default database only for this local,
   single-user developer-tool use case.
