# Ignoring findings

Blunt Code offers three ignore mechanisms. They answer different questions,
so pick by intent:

| Mechanism | Scope | Best for |
| :--- | :--- | :--- |
| Inline `bluntcode:ignore` comment | One line of one file | "This exact spot is fine" — visible to reviewers in the source |
| Workspace suppression (web UI) | One finding fingerprint, per workspace | "This finding is acknowledged/wontfix" — hides it from every future scan, report, and CI gate, restorable anytime |
| External-tool inline comments | Whatever that tool honors | Existing codebases already using `# noqa`, `// biome-ignore`, or `# nosemgrep` |

Separately, `.bluntcodeignore` and per-workspace exclude rules prevent files
from being scanned at all — a different concern from silencing a finding in
a scanned file.

## Inline `bluntcode:ignore` comments (built-in analyzers)

The built-in analyzers — the committed-secrets detector (`secrets`) and the
<!-- bluntcode:ignore -->
TODO/FIXME tracker (`todo`) — honor an inline ignore comment: a line comment
containing the case-sensitive marker `bluntcode:ignore`, with an optional
single rule id and an optional trailing reason.

```python
# bluntcode:ignore — local e2e fixture key, never used against production
<!-- bluntcode:ignore -->
dummy_aws_key = "AKIAIOSFODNN7EXAMPLE"

# bluntcode:ignore secrets.aws-access-key-id recorded in the vault, see runbook
client_key = os.environ["AWS_ACCESS_KEY_ID"]  # bluntcode:ignore todo.todo placeholder

# bluntcode:ignore todo.fixme tracked in issue #431
def legacy_import(): ...
```

The rules:

- The marker is case-sensitive: `bluntcode:ignore`, exactly.
- The comment suppresses findings **on the same line** and findings **on the
  immediately-next line** — so it can sit next to the offending line or on
  the line above it.
- An optional single rule id right after the marker narrows the suppression
  to that rule (for example `bluntcode:ignore secrets.aws-access-key-id` or
  `bluntcode:ignore todo.fixme`); without a rule id, every built-in finding
  on the target line is suppressed.
- Any remaining text after the marker (or rule id) is a free-form reason.
- It applies to the built-in analyzers (`secrets`, `todo`) only. Findings
  from Ruff, Biome, Semgrep, and SonarQube are governed by those tools' own
  mechanisms (below).

Because the comment lives in the source, it travels with the code and shows
up in code review — prefer it when the justification is interesting to
future readers.

## Workspace suppressions (web UI)

Suppression dismisses one finding forever by fingerprint. A fingerprint is
the finding's stable identity (a SHA-256 over rule, path, and location), the
same identity scans compare across runs — so a suppression keeps working
when the surrounding code shifts around.

From a scan report in the web UI, choose **Suppress…** on a finding row. The
dialog takes an optional reason (up to 500 characters) and explains the
consequence: the finding is hidden from future scans, reports, and the CI
gate. Suppressed findings keep appearing in the findings list with a
"Suppressed" status (filterable via the status filter), and the workspace
detail page lists every suppression with its reason, date, and a **Restore**
action — restoring returns the finding to normal counting on the next scan.

What suppression does across the board:

- Applies from the next scan onward; suppressing mid-scan is safe.
- Future scans still store matching findings, but exclude them from the
  scan's totals and severity counts.
- Every report and export (Markdown, HTML, SARIF, CSV, JSON) omits them.
- The new/fixed/persistent comparison never reports a suppressed finding as
  fixed.
- The `--fail-on`/`--max-findings` CI gate never counts them.

The workflow is also exposed as API routes, per workspace:

```text
POST   /api/v1/workspaces/{id}/suppressions           {"fingerprint": "...", "reason": "..."}
GET    /api/v1/workspaces/{id}/suppressions
DELETE /api/v1/workspaces/{id}/suppressions/{fingerprint}
```

`fingerprint` is the finding's 64-character SHA-256 hex; `reason` is
optional (500 characters max). The DELETE returns 404 when the fingerprint
was never suppressed.

## External-tool ignores

Blunt Code runs Ruff, Biome, and Semgrep on the files as they are, so those
tools' own suppression comments pass straight through:

- `# noqa` / `# noqa: RULE` — Ruff (Python), documented in the
  [Ruff error suppression guide](https://docs.astral.sh/ruff/linter/#error-suppression)
- `// biome-ignore lint/rule/NAME: reason` — Biome (JS/TS), documented in
  [Biome's suppression guide](https://biomejs.dev/analyzer/suppressions/)
- `# nosemgrep` / `# nosemgrep: rule-id` — Semgrep, documented in
  [Semgrep's ignoring guide](https://docs.semgrep.dev/ignoring-files-folders-code)

Blunt Code does not interpret these comments; the tool either reports the
finding or does not, and the scan inherits that outcome. SonarQube issues
are managed through SonarQube's own resolution workflow.

## `.bluntcodeignore` versus workspace exclude rules

Both decide which files a scan looks at, and both use the same matcher, but
they live in different places:

- **`.bluntcodeignore`** is a file you commit at the workspace root. Every
  scan — CLI or web UI, on any machine that has the checkout — and the
  file-tree browser honor it. This is the way to share excludes with a team.
- **Workspace exclude rules** are stored in Blunt Code's database per
  workspace (edited in the web UI via **Configure files**). They apply to
  your installation only and must be re-entered on each machine. Use them
  for machine-specific noise the whole team should not inherit.

A `.bluntcodeignore` looks like this:

```text
# one pattern per line; '#' lines are comments, blank lines are ignored
node_modules
dist/**
**/generated
*.min.js
```

The format is a small gitignore-inspired subset. The matcher supports:

- **Basename patterns** — `*.min.js`, `node_modules`, `package-lock.json`
- **Directory prefix form** — `dist/**` excludes everything under `dist/`
- **`**/name` form** — `**/generated` excludes any file or directory named
  `generated` at any depth

Matching is case-insensitive. There is **no negation**: `!`-prefixed lines
are not supported and are skipped (counted for a log line). Details that
keep the file boring: LF or CRLF endings both work, backslashes are
normalized to `/`, the file is capped at 1,000 patterns and 64 KiB, and a
broken or oversized ignore file logs a message and continues — it can never
fail a scan. Patterns from the file merge additively with the workspace's
saved exclude rules; they never cancel includes.

Related pages: [ci.md](ci.md) for how suppression and baselines interact
with the CI gate, [configuration.md](configuration.md) for profiles and the
data layout, [analyzers.md](analyzers.md) for what each analyzer covers.
