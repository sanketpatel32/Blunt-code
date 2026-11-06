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

## Demo / stress data (development only)

`POST /api/v1/system/seed-demo` is **disabled unless** the server sets
`BLUNTCODE_DEMO_SEED=1`. Optional `BLUNTCODE_DEMO_TOKEN` requires clients to send
matching `X-Bluntcode-Demo-Token`. `GET /api/v1/meta` exposes
`demo_seed_env_enabled` and `demo_seed_requires_token` so the UI can hide the
control when seeding is off. Do **not** enable these in production or on real
project databases — the endpoint is for isolated test data and bounded presets
only.

**Multi-width screenshots (opt-in):** with the API running and demo seed enabled,
`cd web && BASE=http://127.0.0.1:8787 node e2e/visual-scale.mjs` (optional
`DEMO_TOKEN=…`) seeds the `large` preset and writes PNGs under
`/opt/cursor/artifacts/bluntcode-scale/`. This exercises the web shell against
synthetic data; it does **not** replace Windows-native analyzer runs.

**Home ledger paging (opt-in, Windows):** the full desktop server path requires
Windows (managed Sonar bootstrap fails on Linux). On **Windows**, point
`LOCALAPPDATA` at a **fresh empty directory** so the SQLite DB is isolated, set
`BLUNTCODE_DEMO_SEED=1`, start `bluntcode --no-browser --port 9876`, then run:

`cd web && BASE=http://127.0.0.1:9876 npm run verify:synthetic-board`

This calls `POST /api/v1/system/seed-demo` (`large`), asserts each Overview
ledger page renders **25** rows matching the footer range, checks that **Next**
does not repeat workspace ids, and writes PNGs under
`/opt/cursor/artifacts/bluntcode-synthetic-verify/` (set `SKIP_SHOTS=1` for
assertions only). **Vitest** on any OS still pins the same contract with mocks.

Version bumps move `internal/build/version.go`, `web/package.json`,
`scripts/package.ps1`, `CHANGELOG.md` and every agent-doc copy together —
`go test ./cmd/bluntcode` fails while any of them disagree (see
`agentdocs_test.go`).
