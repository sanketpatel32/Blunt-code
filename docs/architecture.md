# Architecture

Blunt Code is a loopback-only Go application with a React browser UI. The Go
process owns workspace paths, direct child-process execution, SQLite persistence
and report generation. The UI receives normalized API data; it never launches
an analyzer or accesses arbitrary folders itself.

`internal/analyzers` is the boundary between scan orchestration and tool
details. Each adapter detects applicability, checks readiness, constructs direct
process arguments, runs locally, and normalizes its own structured output.
`internal/reports` consumes only normalized findings, metrics, and run records.

Normal scans make no network calls. Network use is reserved for explicitly
verified tool installation or updates.

On Windows, a per-user-session named mutex derived from the local data directory
prevents two backends from opening the same SQLite database. `bluntcode doctor`
does not acquire that server lock, so it can inspect a running installation
without starting tools or making network requests.
