# Loopback UI harness (Linux / CI)

Serves the **built** web UI from `static/` and an API stack from `api.New(db, bus, nil, nil, paths, …)` — no Windows SonarQube bootstrap.

```bash
cd web && npm ci && npm run build
mkdir -p ../cmd/loopback-harness/static && cp -r dist/* ../cmd/loopback-harness/static/
go build -o loopback-harness ./cmd/loopback-harness
./loopback-harness -port 9876 -data /tmp/blunt-harness-data
cd web && BASE=http://127.0.0.1:9876 npm run verify:synthetic-board
```

`BLUNTCODE_DEMO_SEED=1` is set automatically so `POST /api/v1/system/seed-demo` works. **Synthetic UI/API only** — not native Windows analyzer runs.
