## graphify

This project has a knowledge graph at graphify-out/ with god nodes, community structure, and cross-file relationships.

When the user types `/graphify`, invoke the `skill` tool with `skill: "graphify"` before doing anything else.

Rules:
- For codebase questions, first run `graphify query "<question>"` when graphify-out/graph.json exists. Use `graphify path "<A>" "<B>"` for relationships and `graphify explain "<concept>"` for focused concepts. These return a scoped subgraph, usually much smaller than GRAPH_REPORT.md or raw grep output.
- Dirty graphify-out/ files are expected after hooks or incremental updates; dirty graph files are not a reason to skip graphify. Only skip graphify if the task is about stale or incorrect graph output, or the user explicitly says not to use it.
- If graphify-out/wiki/index.md exists, use it for broad navigation instead of raw source browsing.
- Read graphify-out/GRAPH_REPORT.md only for broad architecture review or when query/path/explain do not surface enough context.
- After modifying code, run `graphify update .` to keep the graph current (AST-only, no API cost).

## versioning (ship rule)

Bump version on every shipment. When shipping something new (feature, fix, docs that users see):
- Bump `cmd/bluntcode/main.go` `const version`, `web/package.json` (+ `web/package-lock.json` `""` package), `scripts/package.ps1` `$Version`
- Update `CHANGELOG.md`: promote `[Unreleased]` to `## [x.y.z] - YYYY-MM-DD` with Added/Changed/Fixed
- Commit, `git tag -a vX.Y.Z -m "vX.Y.Z ..."`, `git push origin main`, `git push origin vX.Y.Z`, then `gh release create vX.Y.Z --latest` with `dist/BluntCode-X.Y.Z-windows-amd64.zip` + `.sha256` + `install-latest.ps1`/`install.cmd` via `scripts/package.ps1 -Version X.Y.Z`
