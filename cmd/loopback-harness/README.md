# Loopback UI harness (Linux / CI)

Serves the **built** web UI from `static/` and an API stack from `api.New(db, bus, nil, nil, paths, …)` — no Windows SonarQube bootstrap.

```bash
cd web && npm ci && npm run build
mkdir -p ../cmd/loopback-harness/static && cp -r dist/* ../cmd/loopback-harness/static/
go build -o loopback-harness ./cmd/loopback-harness
./loopback-harness -i-understand-dev-only -demo-seed -port 9876 -data /tmp/blunt-harness-data
cd web && BASE=http://127.0.0.1:9876 npm run verify:synthetic-board
```

**Safety:** binds **127.0.0.1 only**; **requires** `-data` on a disposable directory and `-i-understand-dev-only`. Demo data is **opt-in** via `-demo-seed` (enables `POST /api/v1/system/seed-demo` only). No packaged analyzers, no production app data. **Synthetic UI/API only** — not native Windows analyzer runs.

**Before/after captures:** build two UI trees, then:

```bash
./loopback-harness -i-understand-dev-only -demo-seed -static-dir /path/to/web/dist -data /tmp/harness-before -port 9877 &
LABEL=v0.49.1 BASE=http://127.0.0.1:9877 OUT=/opt/cursor/artifacts/before node web/e2e/design-system-capture.mjs
```
