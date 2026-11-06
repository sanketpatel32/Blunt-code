# Contributing

Use supported Go and Node versions documented by the release. Run
`powershell -File scripts\verify.ps1` before submitting changes — it is the
whole local gate (gofmt, go vet, go build, go test, web typecheck, web tests,
contrast audit), and because this repo has no CI, green there is the only
green there is. `-SkipWeb` skips the slow web half while iterating on
Go-only changes; never ship on a `-SkipWeb` run. Parser fixtures must be
deterministic, small, and captured from machine-readable tool output. Do not
add telemetry, cloud services, source uploads, or automatic source
modification.

## Linux and non-Windows development

The **supported runtime** for real scans and the full desktop stack is **Windows
10/11** (see README). On Linux, `openCore` / managed Sonar paths reject
non-Windows hosts by design — you will not get a faithful “full backend + large
corpus scan” loop without Windows.

**What you can still verify honestly on Linux (or macOS):**

- **Web UI:** `cd web && npm test -- --run` and `npm run build` — exercises the
  production bundle and API contracts against mocks.
- **Go API + database:** e.g. `go test ./internal/api/... ./internal/database/...`
  — handler and repository behavior without the analyzer/runtime graph.

**Baseline limitation (not introduced by feature PRs):** `go test ./...` on
Linux includes Windows-oriented analyzer packages (`internal/analyzers`, etc.);
those failures are **environment/baseline**, not proof that a UI change regressed
production behavior on Windows.

**Authoritative local gate for shipping:** `powershell -File scripts\verify.ps1`
on a Windows machine with the supported toolchain.

Version bumps move `internal/build/version.go`, `web/package.json`,
`scripts/package.ps1`, `CHANGELOG.md` and every agent-doc copy together —
`go test ./cmd/bluntcode` fails while any of them disagree (see
`agentdocs_test.go`).
