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

Version bumps move `internal/build/version.go`, `web/package.json`,
`scripts/package.ps1`, `CHANGELOG.md` and every agent-doc copy together —
`go test ./cmd/bluntcode` fails while any of them disagree (see
`agentdocs_test.go`).
