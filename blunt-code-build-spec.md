# Blunt Code — Build-Ready Product & Engineering Specification

> **Document purpose:** This file is the authoritative implementation specification for **Blunt Code**.  
> It is intentionally explicit so a coding agent can implement the application with minimal product interpretation or guesswork.
>
> **Project type:** Open-source Windows local development tool  
> **Primary license for Blunt Code source:** MIT  
> **Primary languages analyzed in V1:** Python, JavaScript, TypeScript  
> **Primary operating system in V1:** Windows only  
> **Deployment model:** Local-only web application served by a native local backend  
> **Cloud dependency during analysis:** None  
> **AI/LLM dependency:** None  
> **Telemetry:** None  
> **CI/CD integrations in V1:** None

---

# 1. Product Summary

**Blunt Code** is a local-first code analysis application designed to make static analysis radically easier to use on Windows.

The user installs Blunt Code once, launches it, chooses a local repository or project folder, optionally chooses which files or directories should be included or excluded, and clicks **Analyze**.

Blunt Code then:

1. Detects the repository languages.
2. Determines which analyzers are relevant.
3. Ensures the required analyzers are installed locally.
4. Runs the analyzers against the selected source files.
5. Normalizes results from multiple tools into one internal finding format.
6. Stores the scan and workspace history locally.
7. Displays a polished report in the local web interface.
8. Allows the user to export the report as Markdown.
9. Remembers the workspace and its analysis preferences for future runs.
10. Compares later scans with earlier scans to show new, fixed, and persistent findings.

The initial analyzer stack is:

- **SonarQube Community Build / SonarScanner** — broad code-quality analysis and project-level metrics.
- **Ruff** — Python linting and code-quality analysis.
- **Biome** — JavaScript and TypeScript linting/quality checks.
- **Semgrep** — optional local security/static-analysis layer.

The product must feel like a single application even though it orchestrates several underlying tools.

The user should not need to understand SonarQube setup, scanner configuration, Java runtimes, analyzer CLI syntax, configuration file formats, local ports, database details, process management, or output formats.

Blunt Code owns that complexity.

---

# 2. Product Philosophy

Blunt Code is built around five principles.

## 2.1 One-click analysis

The default workflow must be:

**Open Blunt Code → choose workspace → Analyze → read report.**

Advanced configuration exists, but it must not block normal usage.

## 2.2 Local first

Source code must remain on the user's computer.

Analysis must not upload source files, source snippets, repository metadata, findings, or reports to a remote Blunt Code service.

No Blunt Code server exists in V1.

## 2.3 No AI requirement

Blunt Code must not require an LLM, AI API key, cloud model, local model, or paid inference API.

All findings come from deterministic static-analysis tools.

All summaries in V1 must be generated deterministically from analyzer results.

## 2.4 Sensible defaults

The application should make good decisions automatically.

Examples:

- Detect Python automatically.
- Detect TypeScript automatically.
- Ignore `.git`.
- Ignore `node_modules`.
- Ignore `.venv`.
- Remember exclusions.
- Select relevant analyzers automatically.
- Install required analyzer binaries automatically.
- Keep advanced options out of the primary path.

## 2.5 Analyzer-independent architecture

Blunt Code must not be architected as a hardcoded SonarQube UI.

SonarQube is one analyzer implementation behind a generic analyzer interface.

Adding another analyzer later must not require rewriting the scan engine, database, report engine, or UI.

---

# 3. Project Naming

Use the following naming convention unless changed by the maintainer later.

- Product display name: **Blunt Code**
- Repository name: `blunt-code`
- CLI command: `bluntcode`
- Main executable: `bluntcode.exe`
- Backend package/module namespace: `bluntcode`
- Application data directory: `BluntCode`
- Default localhost UI title: `Blunt Code`
- Default local service name in logs: `bluntcode`

Do not use "BluntCode" in visible marketing copy unless needed for filenames, identifiers, or URLs.

---

# 4. Scope

## 4.1 V1 operating system

V1 supports:

- Windows 10 64-bit
- Windows 11 64-bit

Primary architecture:

- `amd64/x86_64`

ARM64 Windows support is optional after the initial stable release.

Linux and macOS support are explicitly outside V1.

The architecture should avoid unnecessary Windows lock-in in core analysis logic, but Windows-native UX is allowed where it improves the product.

---

# 5. V1 Goals

V1 is complete when a normal Windows developer can:

1. Install Blunt Code with one PowerShell command or a downloadable installer.
2. Run `bluntcode`.
3. Have Blunt Code open the local browser automatically.
4. Add a local project folder using a native folder picker.
5. See a detected file tree.
6. Include or exclude files/directories.
7. Save the folder as a persistent workspace.
8. Click **Analyze**.
9. Watch live scan progress.
10. Receive combined results from relevant analyzers.
11. See findings grouped by severity, category, file, and analyzer.
12. See a high-level project health summary.
13. Inspect a single finding and its source location.
14. See which files were analyzed and skipped.
15. Export a Markdown report.
16. Close Blunt Code and reopen it later.
17. Find the workspace still present.
18. Find previous scan history still present.
19. Re-run analysis without reconfiguration.
20. Compare the new scan with the previous scan.

---

# 6. Explicit Non-Goals for V1

Do not implement the following unless required for internal architecture:

- GitHub Actions integration.
- GitLab CI integration.
- Jenkins integration.
- Azure DevOps integration.
- Cloud synchronization.
- Team accounts.
- User authentication for remote users.
- Remote repository cloning.
- Pull-request comments.
- SaaS hosting.
- Centralized dashboards.
- Organization management.
- Multi-user network access.
- IDE extensions.
- VS Code extension.
- JetBrains plugin.
- Automatic code modification.
- AI-generated fixes.
- LLM summaries.
- Paid analyzer APIs.
- Background source-code upload.
- Browser extension.
- macOS package.
- Linux package.
- Mobile UI.
- Container/Docker requirement for the user.

Docker may be used by maintainers for tests if useful, but the end user must not need Docker.

---

# 7. Primary User Persona

The primary user is a developer who:

- Works mostly with Python and/or TypeScript/JavaScript.
- Wants code-quality information.
- Does not want to spend time configuring multiple analysis tools.
- May already know SonarQube but dislikes the setup and CLI workflow.
- Wants local privacy.
- Does not want recurring AI/API costs.
- Wants repeatable reports for projects.
- May share Blunt Code with friends or coworkers.
- Is comfortable installing an open-source developer tool but expects the tool itself to configure dependencies.

The application should optimize for this user rather than for enterprise SonarQube administrators.

---

# 8. Core User Journey

## 8.1 First installation

The user runs a documented PowerShell command such as:

```powershell
irm <official-install-script-url> | iex
```

The exact release URL is chosen when the GitHub repository exists.

The installer should:

1. Detect supported Windows architecture.
2. Download the appropriate Blunt Code release.
3. Verify checksum.
4. Install under a per-user directory by default.
5. Add `bluntcode` to the user's PATH.
6. Create required application-data directories.
7. Optionally create a Start Menu entry.
8. Optionally create a desktop shortcut.
9. Avoid requiring administrator privileges unless unavoidable.
10. Never install analyzers globally into unrelated system directories.

After installation:

```powershell
bluntcode
```

must launch the application.

## 8.2 First launch

On first launch:

1. Backend starts.
2. SQLite database is created/migrated.
3. A free local port is selected.
4. Server binds only to loopback.
5. Browser opens automatically to the local UI.
6. Home page appears immediately.
7. Background tool validation begins.
8. The user can add a workspace.

The first run may need internet access to download analyzer dependencies.

The UI must clearly distinguish:

- "Blunt Code itself is ready"
- "Analyzer dependencies are being prepared"

The entire UI should not be blocked while one optional analyzer is installing.

## 8.3 Adding a workspace

User clicks:

**Add Workspace**

The backend opens a native Windows folder picker.

After selection:

1. Validate the path exists.
2. Resolve it to an absolute normalized path.
3. Detect whether it is a Git repository.
4. Discover source files.
5. Detect languages.
6. Apply default exclusions.
7. Display the workspace preview.
8. Save it after user confirmation.

The user should not be asked to manually create analyzer configuration files.

## 8.4 Normal analysis

On a saved workspace:

1. User opens workspace.
2. User sees current configuration and last scan.
3. User clicks **Analyze**.
4. Blunt Code snapshots scan configuration.
5. Relevant analyzers run.
6. Progress updates stream into UI.
7. Findings are normalized.
8. Results are stored.
9. Report page opens.
10. User can export Markdown.

---

# 9. UX Requirements

The UI should be clean, compact, and developer-oriented.

Avoid enterprise-dashboard clutter.

## 9.1 Main navigation

Recommended left navigation:

- Home
- Workspaces
- Scan History
- Tools
- Settings
- About

For V1, navigation may be top-level tabs if simpler.

## 9.2 Home page

The Home page should show:

- Blunt Code title.
- "Add Workspace" primary action.
- Recently used workspaces.
- Last scan result for each workspace.
- Analyzer readiness summary.
- A clear "No code leaves your computer" privacy message.

Each recent workspace card shows:

- Workspace name.
- Root path.
- Detected languages.
- Last scan time.
- Finding counts by severity.
- "Open" button.
- "Analyze" quick action.

## 9.3 Workspace page

The workspace page should include:

### Header

- Workspace name.
- Absolute path.
- Git branch if available.
- Latest commit short SHA if available.
- Language badges.
- Last analyzed timestamp.

### Primary actions

- Analyze
- Configure Files
- View Last Report
- Export Last Report
- Workspace Settings

### Summary

Show latest:

- Critical/high findings.
- Medium findings.
- Low findings.
- Total findings.
- New findings since previous scan.
- Fixed findings since previous scan.
- Analyzer statuses.

## 9.4 File selection screen

Display a lazy-loaded tree.

Each directory and file has a checkbox state:

- Included.
- Excluded.
- Partially included.

Default excluded paths should visually indicate why they are excluded.

Examples:

- `node_modules` — default generated/dependency exclusion.
- `.git` — VCS metadata.
- `.venv` — dependency environment.
- `dist` — build artifact.

User actions:

- Include directory.
- Exclude directory.
- Include file.
- Exclude file.
- Reset to defaults.
- Search/filter paths.
- Expand/collapse tree.

Persist choices at workspace level.

Do not store only a giant list of every currently present file if avoidable.

Store rules and explicit overrides so future newly-created files behave predictably.

## 9.5 Scan progress screen

A scan should show stages, not a spinner.

Example:

```text
Preparing workspace
Detecting languages
Checking analyzers
Running Ruff
Running Biome
Running Semgrep
Starting local SonarQube
Running SonarScanner
Collecting SonarQube results
Normalizing findings
Generating report
Complete
```

Each stage should have:

- Pending.
- Running.
- Success.
- Warning.
- Failed.
- Skipped.

Show elapsed time.

Provide a **Cancel Scan** button.

A failure in one analyzer should not necessarily destroy the entire scan.

If Ruff succeeds and SonarQube fails, Blunt Code should still produce a partial report with a clear analyzer error.

## 9.6 Report screen

The report must be readable without understanding the individual analyzers.

Sections:

1. Overview.
2. Scan health.
3. Priority findings.
4. Findings by severity.
5. Findings by category.
6. Findings by analyzer.
7. Findings by file.
8. New findings.
9. Fixed findings.
10. Persistent findings.
11. Project metrics.
12. Analyzer run details.
13. Analyzed files.
14. Skipped files.
15. Scan configuration.
16. Limitations/errors.

Filters:

- Severity.
- Category.
- Tool.
- File.
- Status: new/persistent.
- Text search.

Each finding card should show:

- Severity.
- Category.
- Analyzer.
- Rule identifier.
- Message.
- Relative path.
- Line/column.
- Explanation/remediation when available.
- Fingerprint/status.
- "Open file location" action if practical.

---

# 10. Browser and Native Folder Selection

A pure browser page cannot be relied upon to reveal arbitrary absolute Windows folder paths through drag-and-drop.

Do **not** architect V1 around browser drag-and-drop being the only folder-selection mechanism.

The reliable primary mechanism is:

**Web UI button → local Go backend → native Windows folder picker → backend returns selected path.**

Implement a backend endpoint for interactive folder selection.

Preferred UX:

1. User clicks "Browse Folder".
2. Backend invokes the Windows native directory-selection dialog under the current interactive user session.
3. User selects folder.
4. Backend returns absolute path to frontend.
5. Frontend previews workspace.

Optional enhancements:

- Accept a manually pasted path.
- `bluntcode C:\path\to\repo`.
- `bluntcode .`.
- Windows Explorer context-menu integration.
- Browser drag-and-drop where a supported browser exposes usable filesystem handles.

Important:

- Never silently upload/copy an entire folder into the local web application merely to simulate drag-and-drop.
- Never depend on a Chromium-only experimental browser API for the core workflow.

---

# 11. Recommended Technology Stack

## 11.1 Backend

Use:

- **Go**
- Standard `net/http` or a minimal Go HTTP router.
- SQLite.
- Embedded static frontend assets.

Why:

- Single native executable.
- Good process management.
- Easy Windows distribution.
- Low runtime dependency burden.
- Good concurrency.
- Straightforward filesystem access.
- Can own analyzer child processes.
- Can embed React build output.

Avoid adding a large backend framework unless it provides clear value.

## 11.2 Frontend

Use:

- React
- TypeScript
- Vite
- A lightweight component system or hand-built components.
- React Router if multiple routes are used.
- TanStack Query or a minimal fetch layer for server state.
- SSE for live scan events.

Do not use Electron.

Do not use Tauri for V1.

The browser is the UI shell.

## 11.3 Storage

Use SQLite with migrations.

Recommended Go driver:

- Prefer a driver that minimizes end-user setup.
- A pure-Go SQLite driver is desirable if stable enough for project requirements.
- If a CGO driver is used, ensure Windows release builds remain reproducible and self-contained.

## 11.4 Frontend embedding

Production flow:

1. Build React/Vite.
2. Produce static assets.
3. Embed assets into Go binary using `embed.FS`.
4. Go serves those assets on localhost.

The user should not need Node.js to run Blunt Code.

Node.js is only a development/build dependency.

---

# 12. Local Network Model

The HTTP server must bind to:

```text
127.0.0.1
```

and optionally:

```text
::1
```

It must not bind to:

```text
0.0.0.0
```

by default.

A typical runtime URL:

```text
http://127.0.0.1:47832/
```

Prefer an automatically selected available high port instead of a fixed Blunt Code port.

If the architecture benefits from a stable port, implement collision handling.

The browser should open automatically after the server is ready.

CLI options:

```text
bluntcode
bluntcode .
bluntcode C:\repo
bluntcode --no-browser
bluntcode --port 47832
bluntcode --version
bluntcode doctor
bluntcode tools
```

Only core commands are required for V1.

---

# 13. Data Directories

Use per-user Windows paths.

Recommended:

```text
%LOCALAPPDATA%\BluntCode\
```

Structure:

```text
BluntCode/
  bluntcode.db
  config.json
  logs/
  tools/
  sonar/
  rules/
  cache/
  reports/
  temp/
```

Possible layout:

```text
%LOCALAPPDATA%\BluntCode\tools\ruff\<version>\
%LOCALAPPDATA%\BluntCode\tools\biome\<version>\
%LOCALAPPDATA%\BluntCode\tools\semgrep\<version>\
%LOCALAPPDATA%\BluntCode\tools\sonarqube\<version>\
%LOCALAPPDATA%\BluntCode\tools\sonar-scanner\<version>\
%LOCALAPPDATA%\BluntCode\tools\java\<version>\
```

Do not install tool binaries into project repositories.

Do not modify project source files merely to run analysis unless absolutely required.

Temporary generated configuration should live under Blunt Code's temp directory.

---

# 14. Workspace Model

A workspace is a persistent reference to a local project.

Workspace fields:

```text
id
name
root_path
created_at
updated_at
last_opened_at
last_scan_at
detected_languages
git_repository
default_profile
enabled_analyzers
include_rules
exclude_rules
explicit_path_overrides
settings_json
```

A workspace is identified internally by UUID.

Never use the raw absolute path as the public database primary key.

`root_path` should have a uniqueness constraint after Windows path normalization.

---

# 15. Workspace Path Handling

On Windows:

- Normalize separators.
- Resolve `.` and `..`.
- Prefer absolute canonical paths.
- Handle drive letters case-insensitively.
- Support spaces.
- Support Unicode filenames.
- Support long paths where the OS/configuration allows them.
- Do not follow junctions/symlinks blindly.

The scanner must prevent accidental traversal outside the workspace through symlinks/junctions unless intentionally supported.

Default behavior:

- Analyze only files whose resolved paths remain inside workspace root.
- Mark paths that resolve outside root as skipped.
- Record skip reason.

---

# 16. Default Exclusions

V1 should automatically exclude common generated, dependency, VCS, and cache directories.

Default directory patterns:

```text
.git/
.hg/
.svn/
node_modules/
.venv/
venv/
env/
__pycache__/
.pytest_cache/
.mypy_cache/
.ruff_cache/
.next/
.nuxt/
dist/
build/
out/
coverage/
.cache/
.idea/
.vscode/
```

`.vscode` may be included later if configuration analysis becomes useful, but source analysis should exclude it by default.

Default file exclusions may include:

```text
*.min.js
*.map
*.pyc
*.pyo
```

Do not aggressively exclude lockfiles from workspace discovery; they may be useful for language/package detection even if they are not analyzed.

---

# 17. Source Language Detection

At minimum detect:

## Python

Indicators:

```text
*.py
*.pyi
pyproject.toml
requirements.txt
Pipfile
poetry.lock
uv.lock
```

## JavaScript

Indicators:

```text
*.js
*.jsx
*.mjs
*.cjs
package.json
```

## TypeScript

Indicators:

```text
*.ts
*.tsx
*.mts
*.cts
tsconfig.json
```

A workspace may contain multiple languages.

Store counts:

```json
{
  "python": 32,
  "typescript": 41,
  "javascript": 8
}
```

Do not detect language solely from configuration files.

Count actual candidate source files.

---

# 18. Analyzer Architecture

Every analyzer must implement a common internal interface.

Conceptual Go interface:

```go
type Analyzer interface {
    ID() string
    DisplayName() string
    SupportedLanguages() []Language
    Check(ctx context.Context, env ToolEnvironment) ToolStatus
    EnsureInstalled(ctx context.Context, env ToolEnvironment) error
    Plan(ctx context.Context, req ScanRequest) (AnalyzerPlan, error)
    Run(ctx context.Context, plan AnalyzerPlan, emit EventEmitter) (AnalyzerResult, error)
    Normalize(ctx context.Context, result AnalyzerResult) ([]Finding, []Metric, error)
}
```

The exact signature may differ, but preserve the separation of:

1. Detection.
2. Installation/readiness.
3. Planning.
4. Execution.
5. Parsing.
6. Normalization.

The scan orchestrator must not contain tool-specific parsing logic.

Tool-specific code belongs inside the adapter.

Recommended package structure:

```text
internal/analyzers/
  analyzer.go
  registry.go
  ruff/
  biome/
  semgrep/
  sonarqube/
```

---

# 19. Analyzer Registry

Use a central registry.

Example:

```go
registry.Register(ruff.New(...))
registry.Register(biome.New(...))
registry.Register(semgrep.New(...))
registry.Register(sonarqube.New(...))
```

At scan planning time:

1. Detect workspace languages.
2. Ask each analyzer whether it applies.
3. Apply workspace enable/disable preferences.
4. Build ordered plan.
5. Run relevant analyzers.

The UI should not hardcode the analyzer list.

Expose analyzer metadata through API.

---

# 20. Ruff Adapter

Ruff is the default Python-specific fast analyzer.

Use Ruff when Python source files are present.

Responsibilities:

- Install/manage a pinned compatible Ruff binary.
- Run only against selected workspace paths.
- Prefer machine-readable output.
- Parse findings without scraping colored terminal text.
- Capture rule identifier.
- Capture message.
- Capture file.
- Capture range.
- Capture fix availability when exposed.
- Capture documentation reference if exposed.
- Never automatically apply fixes in V1.

The adapter must support a temporary Blunt Code-generated config layer without overwriting the user's project configuration.

If the project already contains Ruff settings, respect project configuration by default unless doing so prevents selected-file enforcement.

Record:

- Ruff version.
- Effective command.
- Exit code.
- Duration.
- Number of findings.
- Stderr summary.
- Whether project config was detected.

Do not treat Ruff's "findings exist" exit status as a tool crash.

---

# 21. Biome Adapter

Biome is the primary JavaScript/TypeScript-specific analyzer for V1.

Use Biome when JS/TS source is present.

Responsibilities:

- Manage a pinned compatible Biome binary.
- Run lint/analysis without modifying files.
- Prefer structured machine-readable output.
- Parse:
  - rule/category,
  - severity,
  - message,
  - file,
  - source range,
  - advice/remediation where exposed.
- Respect existing project Biome configuration when present.
- Do not force users to create `biome.json`.
- Do not run formatting writes.
- Do not auto-fix.

Important:

Blunt Code is an analyzer/reporting app in V1, not a formatter.

Formatting diagnostics may be hidden by default if they overwhelm quality findings, but this should be a workspace setting.

---

# 22. Semgrep Adapter

Semgrep is the additional security/static-analysis layer.

V1 positioning:

- Optional analyzer implementation.
- May be enabled by default after successful installation if the bundled/downloaded rule strategy is legally and technically clean.
- Must work without uploading source code.
- Analysis must run locally.

Important rule requirement:

**Do not depend on downloading a ruleset from the internet every time the user scans.**

If rule acquisition requires internet:

1. Acquire during first setup or explicit tool update.
2. Store rules locally.
3. Record rule-pack version.
4. Run offline afterward.

If licensing or redistribution of a selected rule pack is unclear:

- Do not bundle it blindly.
- Install/fetch from its official source during dependency setup.
- Preserve required licenses/notices.
- Document behavior.

Semgrep findings should normalize primarily to:

- Vulnerability.
- Security.
- Correctness.
- Maintainability where applicable.

The adapter must distinguish:

- Analyzer executable failure.
- Ruleset unavailable.
- No findings.
- Scan cancelled.

---

# 23. SonarQube Adapter

SonarQube is the broad project-level analysis layer and a key reason Blunt Code exists.

The goal is to remove SonarQube's setup friction from the user experience.

## 23.1 Product behavior

The user should not need to:

- Start SonarQube manually.
- Configure a port manually.
- Remember credentials.
- Create a project manually.
- Generate a token manually.
- Install SonarScanner manually.
- Write `sonar-project.properties` manually.
- Open SonarQube's native UI.
- Run scanner CLI manually.
- Query SonarQube APIs manually.

Blunt Code does these tasks.

## 23.2 Managed local SonarQube

Blunt Code should manage a local SonarQube Community Build instance.

Treat it as a child dependency with its own lifecycle.

Recommended V1 model:

- Use a pinned supported SonarQube Community Build version.
- Use its local embedded/default test/trial database for the single-user local developer-tool use case if compatible with the selected release.
- Do not claim this managed instance is an enterprise/production SonarQube deployment.
- Keep it bound to loopback.
- Keep its storage under Blunt Code application data.
- Start it only when required.
- Reuse it across scans while Blunt Code is running where sensible.
- Shut it down cleanly.

Before release, the maintainer must re-verify the exact supported SonarQube version, runtime requirements, authentication bootstrap behavior, API endpoints, and license obligations.

## 23.3 Version-specific isolation

All SonarQube version-specific behavior belongs inside the SonarQube adapter.

Do not leak SonarQube implementation details across the codebase.

Create explicit interfaces for:

- Server installation.
- Runtime validation.
- Start.
- Health check.
- Initial bootstrap.
- Authentication/token management.
- Project creation.
- Scan launch.
- Compute-engine completion polling.
- Issue retrieval.
- Metric retrieval.
- Shutdown.

## 23.4 SonarQube project keys

Each Blunt Code workspace gets a stable project key.

Example algorithm:

```text
bluntcode:<workspace-uuid>
```

If SonarQube project keys have stricter supported characters for the pinned version, encode safely.

Do not derive the key only from the folder name because multiple folders can share a name.

## 23.5 Temporary scanner configuration

Generate scanner properties into a temporary Blunt Code-owned file/directory.

Never overwrite the user's repository configuration.

Configure:

- Project key.
- Project base directory.
- Sources.
- Exclusions.
- Encoding.
- Server URL.
- Authentication token.
- Language-relevant scanner options as required.

Avoid passing secrets into visible logs.

## 23.6 SonarQube results

After scanning:

1. Wait for background processing to complete.
2. Query the local SonarQube API.
3. Retrieve issues.
4. Retrieve useful metrics.
5. Normalize into Blunt Code findings/metrics.
6. Store only the data required by Blunt Code.
7. Mark the adapter complete.

Potential metrics include, when available:

- Bugs.
- Vulnerabilities.
- Security hotspots.
- Code smells.
- Duplicated lines/density.
- Complexity.
- Cognitive complexity.
- Maintainability rating.
- Reliability rating.
- Security rating.
- Lines of code.

Do not invent metrics that the selected SonarQube version does not expose.

## 23.7 SonarQube failure behavior

If SonarQube cannot start:

- Do not abort Ruff/Biome results.
- Report SonarQube as failed.
- Include a diagnostic summary.
- Offer a "Run Doctor" action.
- Keep the partial report.

---

# 24. Tool Manager

Build a generic Tool Manager.

Responsibilities:

- Maintain tool manifest.
- Know installed versions.
- Check executable existence.
- Check checksum.
- Download official releases.
- Install to Blunt Code private directory.
- Update tools.
- Roll back or retain previous version where practical.
- Report readiness to UI.
- Never modify project source.
- Never rely on global PATH if a managed tool exists.

Conceptual manifest:

```json
{
  "ruff": {
    "version": "PINNED_AT_RELEASE",
    "platform": "windows-amd64",
    "source": "official-release",
    "sha256": "..."
  },
  "biome": {
    "version": "PINNED_AT_RELEASE",
    "platform": "windows-amd64",
    "source": "official-release",
    "sha256": "..."
  }
}
```

Do not place literal `"latest"` in production manifests.

A Blunt Code release must pin compatible analyzer versions.

---

# 25. Dependency Installation Rules

The product promise is minimal configuration.

Therefore:

- Blunt Code automatically installs missing required analyzers.
- Users are not asked where to install them.
- Users are not asked to edit PATH.
- Users are not asked to install Node.js.
- Users are not asked to install Python merely to run Ruff if a standalone Ruff binary is available.
- Users are not asked to globally install Biome.
- Users are not asked to manually start SonarQube.

If Java/runtime support is required for the pinned SonarQube release:

- Prefer a Blunt Code-managed compatible runtime.
- Install it privately under Blunt Code.
- Do not require the user to modify `JAVA_HOME`.
- The SonarQube adapter should set the runtime environment for its process explicitly.

---

# 26. Network and Offline Behavior

Blunt Code is **local-analysis first**, not necessarily "the installer can never use the internet."

Network behavior must be explicit.

## Allowed network operations

Only for:

- Installing Blunt Code.
- Downloading missing analyzer binaries.
- Downloading verified analyzer updates.
- Downloading an approved local Semgrep rules pack if needed.

## Forbidden during source analysis

Blunt Code must not:

- Upload source code.
- Upload snippets.
- Upload file names as telemetry.
- Upload Git remotes.
- Upload findings.
- Send usage analytics.
- Call an AI API.
- Call a Blunt Code cloud API.
- Fetch "recommendations" from a remote service.

After all tools/rules are installed, a normal scan should be able to run without internet connectivity.

Provide a setting:

**Offline Mode**

When enabled:

- Never perform update checks.
- Never download missing tools.
- Report missing dependencies locally.
- Run only installed analyzers.

---

# 27. Tool Readiness UI

Tools page should show:

```text
Ruff        Ready      <version>
Biome       Ready      <version>
Semgrep     Ready      <version>
SonarQube   Ready      <version>
Scanner     Ready      <version>
Java        Managed    <version>
```

Possible statuses:

- Ready.
- Not installed.
- Installing.
- Updating.
- Failed.
- Disabled.
- Unsupported.
- Needs repair.

Actions:

- Install.
- Repair.
- Update.
- Disable.
- Open logs.

Default UX should auto-manage without requiring the user to visit this page.

---

# 28. Scan Profiles

Profiles should exist without cluttering the default workflow.

Default button:

**Analyze**

uses the workspace's default profile.

Profiles:

## Quick

Use only fast language-specific analyzers:

- Ruff for Python.
- Biome for JS/TS.

Goal: seconds.

## Standard — default

Use:

- Ruff.
- Biome.
- Semgrep if enabled.
- SonarQube if enabled.

This is the normal quality scan.

## Deep

Reserved for more expensive/local exhaustive rules and future analyzers.

In V1, Deep may be functionally close to Standard.

Do not invent artificial differences just to populate a menu.

The main workspace screen should not require profile selection before every scan.

---

# 29. Scan State Machine

A scan must have a formal lifecycle.

Recommended states:

```text
queued
preparing
installing_tools
discovering
running
normalizing
generating_report
completed
completed_with_warnings
failed
cancelled
```

Each analyzer run has its own state:

```text
pending
skipped
preparing
running
succeeded
failed
cancelled
```

Persist terminal states.

Do not mark a scan `failed` merely because one optional analyzer failed if useful findings exist.

Use:

`completed_with_warnings`.

---

# 30. Scan Snapshot

When analysis starts, create an immutable scan configuration snapshot.

Store:

- Workspace ID.
- Workspace root path.
- Workspace display name.
- Timestamp.
- Git branch.
- Git commit.
- Dirty state.
- Detected languages.
- Include rules.
- Exclude rules.
- Explicit path overrides.
- Selected profile.
- Enabled analyzers.
- Analyzer versions.
- Blunt Code version.
- Candidate file count.
- Selected file count.

This ensures historical reports remain understandable even if workspace settings later change.

---

# 31. Git Integration

Git support is metadata-only in V1.

If `.git` is detected, capture locally:

- Repository status: yes/no.
- Current branch.
- HEAD commit SHA.
- Short SHA.
- Whether working tree is dirty.
- Optional remote name/URL for display only.

Do not require GitHub access.

Do not send the remote URL anywhere.

Do not make network Git commands.

Do not fetch.

Do not pull.

Do not push.

Do not modify branches.

If Git is unavailable, scanning still works.

---

# 32. File Discovery

Discovery must be efficient.

Algorithm:

1. Walk workspace root.
2. Apply hard safety skips.
3. Apply default excludes.
4. Apply workspace excludes.
5. Apply explicit include overrides.
6. Classify file type/language.
7. Build a candidate list.
8. Store/stream summary.
9. Do not hash giant binaries.
10. Do not read irrelevant binary files into memory.

Detect likely binary files and skip.

Record reason.

Candidate source file should contain:

```text
relative_path
absolute_path_internal_only
language
size
modified_time
selected
selection_reason
```

Never expose unnecessary absolute paths in exported reports unless user requests them.

Prefer relative paths in reports.

---

# 33. Large Repository Handling

Blunt Code must not freeze on large repositories.

Requirements:

- Filesystem walk should be cancellable.
- File tree API should support lazy children.
- Do not send a 100,000-node tree in one JSON response.
- Use pagination or hierarchical fetch.
- Ignore dependency/build directories early.
- Limit log retention.
- Stream progress.
- Do not load all raw analyzer output into RAM if it can be processed incrementally/on disk.
- Set reasonable per-tool output limits with overflow stored in log files.

Warn if selected source size is exceptionally large.

Do not silently truncate findings.

If UI rendering requires paging, paginate.

---

# 34. Finding Normalization Model

Every analyzer finding must map into a common structure.

Recommended:

```json
{
  "id": "uuid",
  "scan_id": "uuid",
  "analyzer_id": "ruff",
  "rule_id": "F401",
  "fingerprint": "stable-hash",
  "severity": "medium",
  "category": "maintainability",
  "title": "Unused import",
  "message": "Imported module is unused",
  "relative_path": "src/example.py",
  "start_line": 12,
  "start_column": 1,
  "end_line": 12,
  "end_column": 14,
  "remediation": null,
  "documentation_url": null,
  "raw_severity": "warning",
  "metadata": {}
}
```

---

# 35. Normalized Severity

Use one stable severity vocabulary:

```text
critical
high
medium
low
info
```

Each analyzer adapter owns its mapping.

Do not map all lint warnings to "high."

Severity normalization must be conservative.

Store original severity as `raw_severity`.

Tool-specific mappings must be documented in code and covered by tests.

---

# 36. Normalized Categories

V1 normalized categories:

```text
bug
vulnerability
security
correctness
maintainability
code_smell
performance
complexity
duplication
style
other
```

Avoid excessive category proliferation.

Keep tool-native category inside metadata if useful.

---

# 37. Stable Finding Fingerprints

To compare scans, findings need stable fingerprints.

Fingerprint inputs should generally include:

- Analyzer ID.
- Rule ID.
- Normalized relative path.
- Stable message signature.
- Nearby structural or source identity when feasible.

Do not use database row ID as the fingerprint.

Line number alone is not stable because code moves.

V1 acceptable fingerprint:

```text
SHA256(analyzer + rule + relative_path + normalized_message)
```

Enhance later with contextual source fingerprints.

The scan diff engine must understand that V1 fingerprints are heuristic.

---

# 38. Scan Comparison

For workspace scan N vs previous comparable scan N-1:

- New = current fingerprint absent previously.
- Persistent = current fingerprint present previously.
- Fixed = previous fingerprint absent currently.

Do not call a finding "fixed" if the relevant analyzer failed in the current scan.

For example:

- Previous SonarQube finding exists.
- Current SonarQube adapter failed.
- It is **unknown**, not fixed.

Comparison should only evaluate an analyzer when it produced a valid current result.

---

# 39. Metrics Model

Separate findings from metrics.

Example metric:

```json
{
  "analyzer_id": "sonarqube",
  "key": "cognitive_complexity",
  "label": "Cognitive Complexity",
  "value": 83,
  "unit": null
}
```

Metrics may be project-level or file-level.

Do not force every tool into metrics.

---

# 40. Analyzer Run Record

Store:

```text
id
scan_id
analyzer_id
version
state
started_at
finished_at
duration_ms
exit_code
finding_count
warning_count
error_summary
log_path
metadata_json
```

Do not store authentication tokens in this table.

---

# 41. SQLite Schema

Exact implementation can evolve through migrations, but V1 should include equivalent tables.

## `schema_migrations`

```text
version
applied_at
```

## `settings`

```text
key TEXT PRIMARY KEY
value_json TEXT NOT NULL
updated_at
```

## `workspaces`

```text
id TEXT PRIMARY KEY
name TEXT NOT NULL
root_path TEXT NOT NULL UNIQUE
created_at
updated_at
last_opened_at
last_scan_at
default_profile
settings_json
```

## `workspace_rules`

```text
id TEXT PRIMARY KEY
workspace_id TEXT NOT NULL
rule_type TEXT NOT NULL
pattern TEXT NOT NULL
source TEXT NOT NULL
enabled INTEGER NOT NULL
created_at
```

`rule_type` examples:

```text
include
exclude
```

`source`:

```text
default
user
```

## `workspace_path_overrides`

```text
id TEXT PRIMARY KEY
workspace_id TEXT NOT NULL
relative_path TEXT NOT NULL
mode TEXT NOT NULL
created_at
updated_at
```

Mode:

```text
include
exclude
```

Unique:

```text
(workspace_id, relative_path)
```

## `scans`

```text
id TEXT PRIMARY KEY
workspace_id TEXT NOT NULL
state TEXT NOT NULL
profile TEXT NOT NULL
started_at
finished_at
bluntcode_version
git_branch
git_commit
git_dirty
candidate_file_count
selected_file_count
total_findings
critical_count
high_count
medium_count
low_count
info_count
error_summary
snapshot_json
report_markdown_path
```

## `analyzer_runs`

As described above.

## `findings`

```text
id TEXT PRIMARY KEY
scan_id TEXT NOT NULL
analyzer_run_id TEXT NOT NULL
analyzer_id TEXT NOT NULL
rule_id TEXT
fingerprint TEXT NOT NULL
severity TEXT NOT NULL
category TEXT NOT NULL
title TEXT
message TEXT NOT NULL
relative_path TEXT
start_line INTEGER
start_column INTEGER
end_line INTEGER
end_column INTEGER
remediation TEXT
documentation_url TEXT
raw_severity TEXT
metadata_json TEXT
```

Indexes:

```text
scan_id
fingerprint
severity
category
analyzer_id
relative_path
```

## `metrics`

```text
id TEXT PRIMARY KEY
scan_id TEXT NOT NULL
analyzer_id TEXT NOT NULL
scope TEXT NOT NULL
relative_path TEXT
metric_key TEXT NOT NULL
label TEXT NOT NULL
value_text TEXT
value_number REAL
unit TEXT
metadata_json TEXT
```

## `tool_installations`

```text
tool_id TEXT NOT NULL
version TEXT NOT NULL
path TEXT NOT NULL
status TEXT NOT NULL
checksum TEXT
installed_at
last_checked_at
metadata_json
PRIMARY KEY(tool_id, version)
```

---

# 42. Database Migration Rules

- Never mutate schema ad hoc on startup.
- Use numbered migrations.
- Migrations are transactional where SQLite allows.
- Back up database before destructive migration.
- Do not delete scan history on upgrade.
- Include migration tests from the oldest supported schema to current.

---

# 43. Local API

Prefix:

```text
/api/v1
```

Return JSON unless endpoint is report download or event stream.

## Health

```http
GET /api/v1/health
```

Returns app/server readiness.

## App metadata

```http
GET /api/v1/meta
```

Returns:

- Blunt Code version.
- OS.
- architecture.
- data directory.
- API version.

Avoid exposing secrets.

## Workspaces

```http
GET /api/v1/workspaces
POST /api/v1/workspaces
GET /api/v1/workspaces/{id}
PATCH /api/v1/workspaces/{id}
DELETE /api/v1/workspaces/{id}
```

Deleting a workspace from Blunt Code must **not** delete the source repository.

Ask separately whether history should be deleted from Blunt Code.

## Folder selection

```http
POST /api/v1/system/select-folder
```

Backend opens native folder picker.

Response:

```json
{
  "cancelled": false,
  "path": "C:\\Users\\..."
}
```

## Discovery

```http
POST /api/v1/workspaces/{id}/discover
GET /api/v1/workspaces/{id}/tree
GET /api/v1/workspaces/{id}/tree?path=src
```

## Rules

```http
GET /api/v1/workspaces/{id}/rules
PUT /api/v1/workspaces/{id}/rules
```

## Scans

```http
GET /api/v1/workspaces/{id}/scans
POST /api/v1/workspaces/{id}/scans
GET /api/v1/scans/{scan_id}
POST /api/v1/scans/{scan_id}/cancel
```

## Events

Use Server-Sent Events:

```http
GET /api/v1/scans/{scan_id}/events
```

Events:

```text
scan.started
scan.stage
analyzer.started
analyzer.progress
analyzer.completed
analyzer.failed
finding.count
scan.warning
scan.completed
scan.cancelled
```

## Findings

```http
GET /api/v1/scans/{scan_id}/findings
```

Query parameters:

```text
severity
category
analyzer
path
status
q
page
page_size
sort
```

## Report

```http
GET /api/v1/scans/{scan_id}/report
GET /api/v1/scans/{scan_id}/report.md
```

`report.md` should return a Markdown download with safe filename.

## Tools

```http
GET /api/v1/tools
POST /api/v1/tools/{tool_id}/install
POST /api/v1/tools/{tool_id}/repair
POST /api/v1/tools/{tool_id}/update
```

---

# 44. API Error Format

Use consistent error payloads.

```json
{
  "error": {
    "code": "TOOL_NOT_READY",
    "message": "SonarQube could not be started.",
    "details": {
      "tool": "sonarqube"
    },
    "request_id": "..."
  }
}
```

Do not return raw stack traces to frontend in production.

Detailed stack traces go to local logs.

---

# 45. Concurrency

V1 default:

- Only one active scan per workspace.
- Prefer only one SonarQube scan at a time.
- Other fast analyzer scans may run concurrently only if orchestration remains predictable.

Simplest safe implementation:

- Global scan worker queue.
- Allow one full workspace scan at a time.

This is acceptable for initial V1.

Architecture can later support limited concurrency.

If the user clicks Analyze twice:

- Do not spawn duplicate scans.
- Show current scan.
- Offer cancel/restart.

---

# 46. Cancellation

Every long operation must accept context cancellation.

On cancel:

1. Mark scan cancelling.
2. Cancel analyzer contexts.
3. Terminate child processes gracefully.
4. Force-kill after timeout if necessary.
5. Stop SonarScanner run if possible.
6. Keep completed analyzer results if useful.
7. Mark scan `cancelled`.
8. Never mark absent findings as fixed.

On Windows, process-tree cleanup matters.

Ensure child processes do not remain orphaned.

---

# 47. Process Execution

Centralize child process execution.

Create a process runner abstraction that provides:

- Context cancellation.
- Stdout capture.
- Stderr capture.
- Streaming events.
- Exit code.
- Duration.
- Environment overrides.
- Working directory.
- Redaction.
- Process-tree cleanup.
- Output size controls.

Never build shell strings from user paths.

Use argument arrays.

Avoid:

```text
cmd.exe /C "<concatenated user input>"
```

when direct process execution is possible.

---

# 48. Security Requirements

Even though Blunt Code is local, treat the local web app as software handling sensitive source code.

Requirements:

- Bind only to loopback.
- Reject non-loopback Host/origin patterns where practical.
- Use a per-process random session token or equivalent local session protection if needed.
- Protect state-changing endpoints from cross-origin requests.
- Do not enable permissive CORS.
- Validate all workspace IDs.
- Validate all requested paths remain in workspace.
- Prevent path traversal.
- Do not serve arbitrary files from disk.
- Escape findings before rendering HTML.
- Never render analyzer messages as trusted HTML.
- Redact tokens from logs.
- Do not store SonarQube credentials in plaintext logs.
- Do not expose analyzer admin endpoints through Blunt Code.

---

# 49. Privacy Requirements

The application must have a visible privacy statement.

Suggested copy:

> Blunt Code analyzes your code locally. It does not upload your source code, findings, or reports to a Blunt Code cloud service. Internet access is used only when needed to install or update local analysis tools.

No telemetry in V1.

No analytics SDK.

No crash upload service.

Local crash logs are acceptable.

---

# 50. Secrets

Blunt Code is not initially a secret scanner unless Semgrep rules provide relevant checks.

Never put:

- SonarQube admin password.
- SonarQube token.
- Download credentials.
- Future private credentials.

into exported Markdown.

If tokens must be persisted:

- Prefer OS-protected storage such as Windows Credential Manager/DPAPI.
- If token lifetime can be limited to the local process, prefer ephemeral tokens.

---

# 51. Report Generation

The report engine consumes normalized data only.

It should not know how Ruff JSON or SonarQube APIs work.

Input:

```text
Scan
Workspace snapshot
Analyzer runs
Findings
Metrics
Comparison
Warnings
```

Output:

- Structured report model.
- UI view.
- Markdown exporter.

The HTML "report" is rendered by the React interface from the report model.

Do not generate a second independent report logic path that can disagree with Markdown.

Use one report model.

---

# 52. Markdown Report Format

Filename:

```text
blunt-code-<workspace-name>-<YYYYMMDD-HHMMSS>.md
```

Sanitize filename.

Recommended report:

```markdown
# Blunt Code Analysis Report

## Executive Summary

## Project Information

## Scan Configuration

## Health Overview

## Priority Findings

## Findings by Severity

### Critical
### High
### Medium
### Low
### Info

## Security Findings

## Correctness / Bugs

## Maintainability & Code Smells

## Complexity & Duplication

## Findings by File

## New Since Previous Scan

## Fixed Since Previous Scan

## Persistent Findings

## Analyzer Summary

### Ruff
### Biome
### Semgrep
### SonarQube

## Project Metrics

## Analyzed Files

## Skipped Files

## Warnings and Incomplete Analysis

## Reproduction Details

## Appendix
```

The report must explicitly state if analysis was partial.

Example:

```text
Analysis completeness: PARTIAL

SonarQube failed to start. Ruff, Biome, and Semgrep completed successfully.
Findings that existed only in previous SonarQube scans are not considered fixed.
```

---

# 53. Executive Summary Without AI

Do not call an AI to write a summary.

Use deterministic templates.

Example logic:

```text
If critical > 0:
  "Immediate attention recommended: X critical findings were detected."

Else if high > threshold:
  "The project contains X high-severity findings that should be prioritized."

Else:
  "No critical findings were detected. Review medium and low findings for maintainability improvements."
```

Also summarize:

- Top categories.
- Most affected files.
- New vs fixed count.
- Failed analyzers.

Do not claim code is "secure" because there are zero findings.

Use language like:

"No critical findings were reported by the analyzers that completed."

---

# 54. Report Score

Avoid an opaque fake 0–100 score in V1 unless the scoring model is transparent and tested.

Prefer explicit counts and analyzer-provided ratings.

If a Blunt Code score is added later:

- Publish formula.
- Keep versioned.
- Do not hide raw findings.

---

# 55. Finding Detail

The detail drawer/page should include:

```text
Severity
Category
Status
Analyzer
Rule
File
Range
Message
Description
Remediation
Analyzer metadata
First seen scan
Last seen scan
```

Optional:

- Copy finding.
- Copy file path.
- Open file in Explorer.
- Open parent folder.

Do not assume VS Code is installed.

---

# 56. History

Workspace Scan History page:

Each row:

```text
Date
Commit
Profile
Status
Critical
High
Medium
Low
New
Fixed
Duration
```

Actions:

- Open report.
- Export Markdown.
- Compare with previous.
- Delete local scan record.

Deleting scan history should delete Blunt Code's generated report/log references associated with that scan, subject to log retention policy.

Never delete repository source.

---

# 57. History Retention

Default:

- Keep scan metadata/findings until user deletes them.
- Keep detailed analyzer logs for the most recent N scans or a configurable size cap.
- Markdown reports can be regenerated from database if data remains.

Suggested initial log retention:

- Last 20 scans per workspace.
- Or max 500 MB total logs, whichever is reached first.

Do not delete findings simply because logs are rotated.

---

# 58. Settings

Keep settings minimal.

## General

- Open browser automatically.
- Check for Blunt Code updates.
- Offline Mode.
- Data directory display.
- Log level.

## Analysis

- Default scan profile.
- Default analyzer enablement.
- Include style/formatting findings.
- Maximum analyzer duration, advanced.

## Tools

- Managed versions.
- Check for analyzer updates.
- Repair.

## Privacy

Display:

- No telemetry.
- Local analysis.
- Network behavior.

No account settings.

---

# 59. Update Strategy

Blunt Code should be updateable without reinstalling manually.

V1 minimum:

```text
bluntcode update
```

or an installer script that safely replaces the executable.

Recommended release flow:

- GitHub Releases.
- Versioned ZIP or installer.
- SHA-256 checksums.
- Signed binaries later if possible.

Do not automatically replace the running executable without a safe mechanism.

Automatic background update is not required.

An update check may notify the user, unless Offline Mode is active.

---

# 60. Installer

Provide two install options.

## PowerShell installer

Primary developer-friendly method.

Responsibilities:

- TLS download.
- Architecture selection.
- Checksum verification.
- Per-user install.
- PATH update.
- Upgrade existing install.
- Uninstall instructions.

## Downloadable installer

Recommended later in V1 stabilization:

- MSI or a reputable Windows installer format.
- Per-user by default.
- Start Menu shortcut.
- Optional desktop shortcut.
- Add CLI to PATH.

Do not require the Windows user to know Go/Node/Python/Java package managers.

---

# 61. `bluntcode doctor`

Implement a local diagnostics command.

Output:

```text
Blunt Code: OK
Database: OK
Data directory: OK
Loopback port: OK
Ruff: OK
Biome: OK
Semgrep: OK
SonarQube installation: OK
SonarQube startup: OK
SonarScanner: OK
Managed Java runtime: OK
Disk space: OK
```

If something fails, provide actionable output.

`doctor` must never upload diagnostics.

Add:

```text
bluntcode doctor --json
```

later if useful.

---

# 62. Logging

Use structured logs.

Each log entry ideally includes:

```text
timestamp
level
component
scan_id
workspace_id
analyzer
message
```

Never include entire source lines by default.

Components:

```text
app
http
workspace
scanner
tools
ruff
biome
semgrep
sonarqube
report
database
```

Production default log level:

`info`.

Keep a human-readable local log file.

---

# 63. Observability in UI

When a scan fails, user should not need to open raw logs first.

Show:

- Human explanation.
- Failing stage.
- Analyzer.
- Suggested recovery.
- "View technical details."
- "Run Doctor."
- "Repair Tool."

Raw command arguments may be shown after redaction.

---

# 64. Error Categories

Define internal error codes.

Examples:

```text
WORKSPACE_NOT_FOUND
WORKSPACE_PATH_INVALID
WORKSPACE_PATH_INACCESSIBLE
TOOL_NOT_INSTALLED
TOOL_DOWNLOAD_FAILED
TOOL_CHECKSUM_FAILED
TOOL_EXECUTION_FAILED
TOOL_TIMEOUT
SONAR_START_FAILED
SONAR_HEALTH_TIMEOUT
SONAR_AUTH_FAILED
SONAR_SCAN_FAILED
SONAR_RESULT_FETCH_FAILED
DATABASE_ERROR
SCAN_CANCELLED
PATH_OUTSIDE_WORKSPACE
REPORT_GENERATION_FAILED
NETWORK_DISABLED
UNSUPPORTED_PLATFORM
```

Frontend behavior should depend on codes, not string matching.

---

# 65. Timeouts

Do not let child processes hang forever.

Suggested configurable defaults:

- Tool download: 10 minutes.
- SonarQube startup: 3 minutes.
- Fast analyzer: 10 minutes.
- SonarQube scan: 30 minutes.
- Sonar result processing: 5 minutes.

These are starting defaults and should be adjusted after real tests.

A timeout should produce partial results where possible.

---

# 66. Frontend Route Proposal

```text
/
 /workspaces
 /workspaces/:workspaceId
 /workspaces/:workspaceId/files
 /workspaces/:workspaceId/scans
 /scans/:scanId
 /scans/:scanId/findings/:findingId
 /tools
 /settings
 /about
```

---

# 67. Frontend Components

Recommended component boundaries:

```text
AppShell
Sidebar
TopBar
WorkspaceCard
WorkspaceHeader
LanguageBadges
FileTree
FileTreeNode
RuleEditor
ScanButton
ScanProgress
ScanStageList
AnalyzerStatus
SeverityBadge
CategoryBadge
FindingTable
FindingFilters
FindingDetail
MetricCard
ScanComparison
ReportHeader
ToolsTable
ToolStatusBadge
EmptyState
ErrorPanel
PrivacyNotice
```

Do not put all scan UI in one giant component.

---

# 68. Frontend State

Separate:

- Server state.
- UI state.

Server state includes:

- Workspaces.
- Scans.
- Findings.
- Tools.
- Settings.

UI state includes:

- Expanded file-tree nodes.
- Active filters.
- Selected tab.
- Modal state.

Do not duplicate backend scan truth in a large frontend global store.

SSE events should invalidate/update server-state queries.

---

# 69. Visual Direction

Blunt Code should look modern but understated.

Desired feeling:

- Developer tool.
- Fast.
- Local.
- Direct.
- No enterprise sales aesthetic.

Possible style:

- Neutral light and dark themes.
- Monospace for paths/rules.
- Sans-serif for general UI.
- Severity colors should be accessible.
- Avoid using color alone to communicate severity.
- Dense tables may be used for findings.
- Large whitespace is fine on dashboard, but findings pages should support information density.

Dark mode is desirable in V1 but not more important than core functionality.

---

# 70. Accessibility

V1 should include:

- Keyboard-accessible controls.
- Visible focus states.
- Semantic buttons.
- Form labels.
- Table semantics.
- Contrast-aware severity indicators.
- Non-color severity text.
- Screen-reader labels for icon-only controls.

---

# 71. Analyzer Output Parsing

Rules:

- Prefer JSON/SARIF/structured output.
- Do not parse human terminal tables unless there is no structured alternative.
- Keep raw output in bounded diagnostic logs.
- Treat parser failure separately from analyzer execution failure.
- Save fixture outputs in tests.
- Version parsers when upstream formats change materially.

---

# 72. SARIF

If an analyzer provides SARIF cleanly, support SARIF as an adapter input format.

Do not force all tools to SARIF if their native JSON is richer or more stable.

Possible internal helper:

```text
SARIF -> normalized findings
```

This can simplify future analyzers.

---

# 73. Workspace Configuration vs Project Configuration

Blunt Code should respect existing project configs when reasonable:

- `pyproject.toml`
- Biome config
- Semgrep config if user intentionally has one
- Sonar properties where safe

However:

- Blunt Code's file inclusion/exclusion choices must remain authoritative for what it intentionally scans.
- Never silently edit the repository's config files.
- Generate overlay/temp configuration instead.

Show when project configuration affects results.

---

# 74. File Selection Persistence

User requirement:

Blunt Code remembers what was analyzed before.

Implement:

- Persistent workspace include/exclude rules.
- Persistent explicit overrides.
- Scan snapshot of exact selected files or a compact manifest.

For each completed scan, store the set of selected relative paths if size is reasonable.

If the repo is huge, store a compressed manifest or hash plus exceptions.

The report should show:

- selected count,
- analyzed count,
- skipped count.

A selected file may still be skipped by a particular analyzer if unsupported.

---

# 75. Scan File Manifest

Recommended table if exact history is needed:

## `scan_files`

```text
scan_id
relative_path
language
selected
analyzed_by_json
skip_reason
size_bytes
content_hash_optional
PRIMARY KEY(scan_id, relative_path)
```

Avoid hashing every file by default if it harms speed.

Use file hashes later for improved diffing/caching.

---

# 76. Caching

V1 may implement conservative caching.

Do not skip an analyzer merely because timestamps appear unchanged unless correctness is guaranteed.

Useful safe caches:

- Tool downloads.
- Language discovery.
- File tree.
- SonarQube installation.
- Rule packs.

Future:

- Per-file analyzer cache keyed by content hash + analyzer version + config hash.

Do not over-engineer analysis caching before V1 works reliably.

---

# 77. Tool Update Compatibility

Blunt Code owns a tested compatibility matrix.

Example internal release metadata:

```json
{
  "bluntcode": "1.0.0",
  "tools": {
    "ruff": "x",
    "biome": "y",
    "semgrep": "z",
    "sonarqube": "a",
    "sonar_scanner": "b",
    "java": "c"
  }
}
```

Do not update analyzer major versions independently without testing parsers and behavior.

---

# 78. Open-Source and Licensing

Blunt Code itself should be MIT licensed.

However, analyzer dependencies retain their own licenses.

Requirements before publishing binary releases:

1. Create `THIRD_PARTY_NOTICES.md`.
2. Record each dependency.
3. Record license.
4. Record whether it is bundled or downloaded.
5. Preserve required copyright/license files.
6. Review redistribution terms.
7. Do not state that SonarQube, Ruff, Biome, or Semgrep are MIT merely because Blunt Code is MIT.
8. Do not imply affiliation with SonarSource, Astral, Biome maintainers, or Semgrep.

SonarQube's upstream open-source repository currently uses LGPLv3; Blunt Code's code can remain under its own license when interacting through normal process/API boundaries, subject to the dependency's license obligations.

This specification is not legal advice.

Before each public release, verify current upstream licensing.

---

# 79. SonarQube Local Database Positioning

The managed local SonarQube use case is specifically:

- Single local developer.
- Local analysis.
- Blunt Code-managed.
- Not a shared enterprise server.
- Not high availability.
- Not a production multi-user SonarQube service.

The current SonarQube Community Build documentation provides an embedded H2 database by default and describes it as suitable for development/testing/trials rather than production.

This aligns with Blunt Code's initial managed local analysis-engine concept, but the exact supported behavior must be verified against the pinned SonarQube release.

If a future SonarQube release makes this model unsuitable, the adapter must be replaceable without changing Blunt Code's overall architecture.

Possible future alternatives:

- Managed local PostgreSQL.
- External user-provided SonarQube.
- SonarQube disabled by default.
- Another analyzer replacing part of SonarQube's role.

---

# 80. External SonarQube Mode — Future-Compatible

Do not require this in early V1 UI, but design the adapter so it can later support:

```text
Managed Local
External Local/Remote
Disabled
```

External mode could accept:

- Server URL.
- Token.
- Project configuration.

Never implement this at the cost of the one-click managed local path.

---

# 81. Build Repository Structure

Recommended monorepo:

```text
blunt-code/
  README.md
  LICENSE
  THIRD_PARTY_NOTICES.md
  SECURITY.md
  CONTRIBUTING.md
  CHANGELOG.md

  cmd/
    bluntcode/
      main.go

  internal/
    app/
    api/
    analyzers/
      analyzer.go
      registry.go
      ruff/
      biome/
      semgrep/
      sonarqube/
    config/
    database/
      migrations/
    discovery/
    events/
    gitmeta/
    logging/
    process/
    reports/
    scans/
    security/
    tools/
    windows/
      folderpicker/
      process/
    workspace/

  web/
    package.json
    vite.config.ts
    src/
      api/
      components/
      features/
        workspaces/
        files/
        scans/
        findings/
        tools/
        settings/
      routes/
      styles/
      App.tsx
      main.tsx

  scripts/
    install.ps1
    uninstall.ps1
    package.ps1

  tests/
    fixtures/
      ruff/
      biome/
      semgrep/
      sonarqube/
    integration/
    e2e/

  docs/
    architecture.md
    analyzers.md
    privacy.md
    release.md
```

Exact naming can differ, but preserve modular boundaries.

---

# 82. Backend Service Boundaries

Recommended internal services:

## WorkspaceService

- Create.
- List.
- Update.
- Remove.
- Discover.
- Persist rules.
- Resolve paths.

## ScanService

- Start.
- Plan.
- Cancel.
- Track state.
- Compare.
- Persist.

## ToolService

- Read manifest.
- Validate.
- Install.
- Repair.
- Update.

## AnalyzerRegistry

- Applicability.
- Analyzer lookup.
- Metadata.

## ReportService

- Build report model.
- Export Markdown.

## Database

- Repositories/queries.
- Migrations.

## EventBus

- Publish scan progress.
- Subscribe SSE connections.

Do not let HTTP handlers directly orchestrate analyzers.

---

# 83. Scan Orchestration Pseudocode

```text
StartScan(workspaceID):
    workspace = loadWorkspace(workspaceID)

    if activeScanExists(workspaceID):
        return existing scan / conflict

    scan = createScanSnapshot(workspace)

    publish(scan.started)

    files = discovery.discover(workspace, rules)
    update scan counts

    plan = analyzerRegistry.plan(workspace, files, profile)

    for tool required by plan:
        if missing:
            if offline mode:
                mark analyzer unavailable
            else:
                ToolService.ensure(tool)

    results = []

    for analyzer in plan in controlled order:
        if cancelled:
            stop

        publish(analyzer.started)

        result = analyzer.run(...)

        if result failed:
            store failure
            publish(analyzer.failed)
            continue

        normalized = analyzer.normalize(result)
        persist normalized
        results += normalized

        publish(analyzer.completed)

    comparison = compareWithPreviousValidScan(scan)

    report = ReportService.build(scan, results, comparison)

    persist report metadata
    export cached markdown if desired

    if no analyzer succeeded:
        scan.state = failed
    else if any analyzer failed:
        scan.state = completed_with_warnings
    else:
        scan.state = completed

    publish(scan.completed)
```

---

# 84. SonarQube Orchestration Pseudocode

```text
EnsureSonar():
    verify installation
    verify runtime
    if already healthy:
        return

    start local server
    wait for health with timeout

    if first bootstrap:
        initialize credentials securely
        create/retrieve scanner token

RunSonar(workspace, scan):
    EnsureSonar()

    projectKey = stableProjectKey(workspace.id)

    ensure project exists

    tempConfig = build scanner config:
        base directory
        selected source roots
        exclusions
        project key
        local server URL
        token

    run sonar-scanner

    read scanner task identifier

    poll local compute task until complete

    fetch issues via local API
    fetch metrics via local API

    normalize
```

All actual API details must be implemented against the pinned tested SonarQube version.

---

# 85. Startup Lifecycle

On `bluntcode`:

1. Parse CLI.
2. Initialize paths.
3. Initialize logging.
4. Acquire single-instance lock if implemented.
5. Open SQLite.
6. Run migrations.
7. Start local HTTP server.
8. Start lightweight background tool validation.
9. Open browser.
10. Serve until terminated.

Do not start SonarQube on every Blunt Code launch.

Start SonarQube lazily when:

- A scan requiring it starts.
- Tool Doctor explicitly validates it.

This keeps idle startup fast.

---

# 86. Single Instance

Preferred V1 behavior:

Only one Blunt Code backend per user data directory.

If `bluntcode` is run while already active:

1. Detect existing healthy instance.
2. If a path argument was supplied, send "open/add workspace" intent to existing instance.
3. Open browser to existing instance.
4. Exit second process.

This avoids database/process conflicts.

Can be deferred if it materially delays MVP, but design with it in mind.

---

# 87. CLI Path Argument

Support:

```powershell
cd C:\Repos\Example
bluntcode .
```

and:

```powershell
bluntcode C:\Repos\Example
```

Behavior:

- Launch/reuse Blunt Code.
- Add workspace if unknown.
- Open that workspace in browser.

This is a core convenience feature and helps compensate for browser folder-access limitations.

---

# 88. Windows Explorer Integration — Nice V1 Enhancement

Optional install checkbox or later setting:

**Add "Open in Blunt Code" to Explorer**

Right-clicking a folder launches:

```text
bluntcode "<folder-path>"
```

Do not make shell integration mandatory.

Do not require admin rights if per-user shell registration is possible.

---

# 89. Performance Targets

Targets are guidelines, not guarantees.

Blunt Code itself:

- Initial local UI should appear within ~2 seconds on a normal machine, excluding first install.
- Workspace discovery for an ordinary repo should feel near-instant.
- Ruff/Biome stage should begin quickly.
- UI remains responsive during all scans.
- No operation should block the UI thread.

SonarQube can take materially longer to start; show explicit progress.

---

# 90. First-Run Analyzer Setup UX

The user should not see a wall of dependency prompts.

Example:

```text
Preparing code analyzers for first use...

Ruff            Ready
Biome           Downloading...
Semgrep         Waiting
SonarQube       Waiting

You can continue using Blunt Code while setup completes.
```

If the user clicks Analyze before setup finishes:

- Prioritize tools required by that workspace.
- Show analyzer preparation as scan stages.

---

# 91. Analyzer Failures and Partial Reports

Example outcome:

```text
Ruff       Success — 12 findings
Biome      Success — 4 findings
Semgrep    Success — 1 finding
SonarQube  Failed — server startup timeout
```

Overall scan:

`Completed with warnings`

The report still contains 17 findings.

Do not throw away successful results.

---

# 92. Empty Result Semantics

If no findings exist:

Do not say:

"Your code is perfect."

Say:

"No findings were reported by the analyzers that completed for this scan."

Show:

- Analyzers that completed.
- Files analyzed.
- Limitations.
- Date/tool versions.

---

# 93. Testing Strategy

V1 needs real tests.

## Unit tests

Cover:

- Path normalization.
- Default excludes.
- Language detection.
- Severity normalization.
- Category normalization.
- Fingerprint generation.
- Scan comparison.
- Report generation.
- Markdown escaping.
- API validation.
- Tool manifest handling.
- Error mappings.

## Parser fixture tests

Store representative structured outputs for:

- Ruff.
- Biome.
- Semgrep.
- SonarQube API payloads.

For each fixture:

- Parse.
- Normalize.
- Assert exact findings.

## Integration tests

- SQLite migrations.
- Workspace create/discover.
- Run Ruff on fixture project.
- Run Biome on fixture project.
- Semgrep adapter if available.
- SonarQube lifecycle against pinned test installation.
- Cancellation.
- Partial analyzer failure.
- Markdown export.

## Frontend tests

- Workspace list.
- Add folder flow mocked.
- File tree selection.
- Scan progress.
- Finding filters.
- Report rendering.
- Error states.

## End-to-end Windows tests

On Windows CI or a Windows test machine:

1. Install Blunt Code.
2. Run executable.
3. Add fixture workspace.
4. Analyze.
5. Assert report generated.
6. Restart app.
7. Assert history persists.
8. Uninstall.

---

# 94. Test Fixture Repositories

Create tiny deterministic fixture repos.

## `python-clean`

Contains valid Python with minimal findings.

## `python-bad`

Contains intentional Ruff and security findings.

## `typescript-clean`

Minimal TS project.

## `typescript-bad`

Intentional Biome/Semgrep findings.

## `mixed-project`

Python + TypeScript.

## `large-tree`

Generated large directory tree with:

- `node_modules`.
- `.venv`.
- build output.
- source.

Used to verify exclusions and performance.

---

# 95. Security Tests

Test:

- `../` traversal in tree endpoints.
- URL-encoded traversal.
- Workspace path outside root.
- Symlink/junction escape.
- HTML in analyzer message.
- Malicious filenames.
- Quote characters in paths.
- PowerShell metacharacters.
- Command injection attempts.
- Cross-origin POST attempts.
- Oversized analyzer output.

---

# 96. Release Build

A release should produce:

```text
bluntcode-windows-amd64.zip
bluntcode.exe
checksums.txt
install.ps1
LICENSE
THIRD_PARTY_NOTICES.md
```

If an installer is built:

```text
BluntCode-Setup-x.y.z.exe
```

or MSI equivalent.

The executable must include frontend assets.

Do not ship `node_modules`.

---

# 97. Versioning

Use semantic versioning.

```text
0.1.0
0.2.0
...
1.0.0
```

Before 1.0, database migrations must still be treated seriously because users will accumulate scan history.

Expose version:

```powershell
bluntcode --version
```

UI About page shows:

- Blunt Code version.
- Build commit.
- API version.
- Tool versions.

---

# 98. Development Commands

Recommended developer experience:

```text
make dev
make test
make lint
make build
make package
```

On Windows where `make` may not be present, provide PowerShell equivalents:

```powershell
.\scripts\dev.ps1
.\scripts\test.ps1
.\scripts\build.ps1
.\scripts\package.ps1
```

Do not require developers to guess setup steps.

---

# 99. Developer Mode

Development may run frontend/backend separately:

```text
Go backend: localhost backend port
Vite frontend: localhost dev port
```

Vite proxies `/api` to backend.

Production uses embedded frontend.

---

# 100. Configuration

Global configuration must have safe defaults.

Example conceptual config:

```json
{
  "openBrowser": true,
  "offlineMode": false,
  "updateChecks": true,
  "defaultProfile": "standard",
  "logLevel": "info",
  "analyzers": {
    "ruff": true,
    "biome": true,
    "semgrep": true,
    "sonarqube": true
  }
}
```

Do not expose every analyzer CLI option in V1.

Power users can get advanced controls later.

---

# 101. Data Integrity

Before scan start:

- Verify workspace still exists.
- Verify database writable.
- Verify sufficient disk space roughly.
- Verify selected files exist.

Use database transactions when saving:

- Scan completion.
- Findings batches.
- Report metadata.

A crash should not leave a scan pretending to be complete.

At next startup, scans left in active transient states should become:

`interrupted`.

Then UI can display:

"Previous scan was interrupted."

---

# 102. Crash Recovery

At startup:

1. Find scans in `preparing`, `running`, `normalizing`, `generating_report`.
2. Mark as interrupted/failed due to prior process exit.
3. Do not infer fixed findings from interrupted scan.
4. Clean stale temp directories.
5. Detect stale analyzer child processes only if safely identifiable.

Do not kill unrelated Java/Python/Node processes.

---

# 103. Temporary Files

Use scan-specific temp directory:

```text
%LOCALAPPDATA%\BluntCode\temp\<scan-id>\
```

Store:

- Generated analyzer config.
- Intermediate structured output.
- Temporary manifests.

On success:

- Delete unnecessary temp data.
- Keep only bounded logs/report artifacts.

On crash:

- Clean stale temp directories older than a safe threshold at next startup.

---

# 104. Report Reproducibility

Report appendix must include:

- Blunt Code version.
- Analyzer versions.
- Scan timestamp.
- Profile.
- Git commit if available.
- Dirty state.
- File counts.
- Analyzer success/failure status.

This allows a developer to understand how the report was produced.

Do not include local secrets.

---

# 105. Prioritization Rules

In the report's "Priority Findings":

Order by:

1. Critical.
2. High.
3. New findings before persistent findings.
4. Vulnerability/security/bug/correctness before style.
5. Stable path/line tie-break.

Do not claim a style issue is more important than a high security finding because one analyzer gave it a larger numeric code.

---

# 106. Scan Comparison UX

Summary:

```text
New        7
Fixed      12
Persistent 18
```

Use a positive presentation for fixed findings without turning the product into a gamified score.

If previous scan is not comparable because analyzer set changed:

Display:

"Comparison is partial because analyzer coverage changed."

---

# 107. Filters and Pagination

Findings endpoint should paginate.

Default:

- 50 findings per page.

Allow:

- 25.
- 50.
- 100.

Search fields:

- Message.
- Rule ID.
- Relative path.

Sort:

- Severity.
- Path.
- Analyzer.
- Status.

Do not fetch 50,000 findings into React at once.

---

# 108. Markdown Safety

When exporting analyzer messages:

- Escape Markdown where necessary.
- Use backticks around paths/rules.
- Avoid broken tables from `|`.
- Avoid treating analyzer text as raw HTML.
- Do not embed arbitrary source code by default.
- If short source snippets are later included, fence and escape properly.

---

# 109. HTML Safety

React should render analyzer messages as text.

Do not use `dangerouslySetInnerHTML` for analyzer output.

If Markdown rendering is introduced:

- Use a safe renderer.
- Disable raw HTML or sanitize it.

---

# 110. Opening Files

V1 optional action:

**Open containing folder**

Backend can use Windows Explorer.

Potential:

```text
explorer.exe /select,"C:\...\file.py"
```

Use direct argument execution safely.

Opening in editor should be future work unless editor detection is straightforward.

---

# 111. Tool Installation Security

For downloaded binaries:

- Use HTTPS official upstream source.
- Verify expected SHA-256.
- Pin version.
- Reject checksum mismatch.
- Download to temp.
- Verify.
- Atomically move into tool directory.
- Do not execute a partially downloaded binary.
- Store install metadata.

If upstream distribution cannot provide a stable checksum, the maintainer must define a trusted release-manifest strategy.

---

# 112. PowerShell Installer Security

The install script should be readable and version-controlled.

It should:

- Use strict error handling.
- Validate download.
- Avoid disabling execution security globally.
- Avoid writing machine-wide registry values unnecessarily.
- Avoid requiring admin by default.
- Never invoke arbitrary dynamic code from untrusted endpoints.

Document manual ZIP install as fallback.

---

# 113. Maintainer Release Checklist

Before every release:

- Unit tests pass.
- Integration tests pass.
- Windows E2E passes.
- Frontend build clean.
- `go test ./...` clean.
- Tool parser fixtures current.
- Analyzer versions pinned.
- Checksums updated.
- Licenses rechecked.
- Third-party notices updated.
- Database migration tested.
- Upgrade from previous release tested.
- Fresh install tested.
- Offline scan tested after dependencies installed.
- SonarQube fresh bootstrap tested.
- Partial scan tested.
- Cancellation tested.
- Uninstall tested.
- No telemetry/network surprises observed.

---

# 114. MVP Milestones

Implement in this order.

## Milestone 0 — Repository foundation

Deliver:

- Go app.
- React/Vite app.
- Dev proxy.
- Embedded production assets.
- SQLite connection.
- Migration system.
- Structured logging.
- Version command.
- Health API.

Acceptance:

`bluntcode` serves UI locally.

## Milestone 1 — Workspaces

Deliver:

- Native Windows folder picker.
- Create/list/open workspace.
- File discovery.
- Language detection.
- Default exclusions.
- File tree.
- Persistent selection rules.

Acceptance:

User can add a Python/TS repo, restart app, and workspace remains.

## Milestone 2 — Generic analyzer engine

Deliver:

- Analyzer interface.
- Registry.
- Process runner.
- Tool manager.
- Scan state machine.
- SSE progress.
- Persistence.

Acceptance:

Fake/test analyzer can run end-to-end.

## Milestone 3 — Ruff

Deliver:

- Tool installation.
- Python detection.
- Ruff execution.
- Structured parsing.
- Finding normalization.
- Report display.

Acceptance:

Python fixture produces deterministic findings.

## Milestone 4 — Biome

Deliver:

- Managed Biome.
- JS/TS execution.
- Parser.
- Normalization.

Acceptance:

TS fixture produces combined Ruff/Biome report in mixed repo.

## Milestone 5 — Report engine and Markdown export

Deliver:

- Unified report model.
- Filters.
- Findings detail.
- Markdown export.
- Scan history.
- Basic comparison.

Acceptance:

User downloads valid `.md` report.

## Milestone 6 — Semgrep

Deliver:

- Local Semgrep execution.
- Offline rule-pack strategy.
- Security finding normalization.
- Tool readiness UX.

Acceptance:

Security fixture yields expected local findings with network disconnected after setup.

## Milestone 7 — SonarQube managed adapter

Deliver:

- Managed SonarQube install.
- Managed runtime if needed.
- Lazy startup.
- Health checks.
- Bootstrap/auth.
- SonarScanner.
- Project mapping.
- Result polling.
- Issue import.
- Metrics import.
- Shutdown/cleanup.

Acceptance:

Fresh Windows machine can install Blunt Code, select repo, click Analyze, and receive SonarQube findings without manually configuring SonarQube.

## Milestone 8 — Hardening

Deliver:

- Cancellation.
- Crash recovery.
- Doctor.
- Partial scan UX.
- Large repo optimization.
- Security tests.
- Installer.
- Upgrade path.
- Third-party notices.

Acceptance:

Release candidate suitable for friends/public GitHub use.

---

# 115. V1 Definition of Done

Blunt Code V1 is done only when all of the following are true.

## Installation

- One-command PowerShell install works on a fresh supported Windows machine.
- User does not need Go.
- User does not need Node.js.
- User does not need to manually install Ruff/Biome.
- SonarQube prerequisites are handled by Blunt Code's managed setup.

## Launch

- `bluntcode` starts app.
- Browser opens.
- Localhost only.
- No account required.

## Workspace

- Native folder selection works.
- Python/JS/TS detection works.
- Selections/exclusions persist.
- Workspace history persists.

## Analysis

- Ruff works.
- Biome works.
- Semgrep works according to selected V1 policy.
- SonarQube works through managed adapter.
- Partial results survive individual analyzer failure.
- Scan can be cancelled.

## Report

- Findings normalized.
- Filters work.
- Previous scan comparison works.
- Markdown download works.
- Analyzer versions included.
- Partial coverage clearly disclosed.

## Privacy

- No source upload.
- No telemetry.
- No AI API.
- Normal scan works offline after tool setup.

## Quality

- Tests exist.
- Errors actionable.
- No orphan analyzer processes in normal cancel/exit paths.
- Database survives restart and upgrade.
- Third-party licenses documented.

---

# 116. Things the Implementation Agent Must Not Do

This section is normative.

Do not:

1. Replace the local web UI with Electron.
2. Require Docker.
3. Require a cloud backend.
4. Add user accounts.
5. Add AI features.
6. Call OpenAI, Anthropic, Gemini, or other LLM APIs.
7. Upload repositories.
8. Add telemetry "for convenience."
9. Make ESLint the mandatory JS/TS analyzer in place of Biome.
10. Auto-fix source code.
11. Rewrite project config files.
12. Make users configure SonarQube manually.
13. Require users to start SonarQube manually.
14. Use browser folder drag-drop as the sole path-selection mechanism.
15. Store scan history in browser localStorage as the primary database.
16. Put all analyzer logic in HTTP handlers.
17. Parse colored CLI output when a structured output exists.
18. Shell-concatenate user-supplied paths.
19. Bind the web UI to the LAN by default.
20. Mark findings fixed when the corresponding analyzer failed.
21. Block report creation because one optional analyzer failed.
22. Call a no-findings scan "perfect" or "secure."
23. Download analyzer binaries on every scan.
24. Depend on internet access for every Semgrep scan.
25. Hardcode current upstream "latest" versions.
26. Delete user repositories when a workspace is removed.
27. Hide analyzer failures.
28. Use a fake proprietary quality score without a documented formula.
29. Require administrator rights without a real technical reason.
30. over-engineer CI/team/cloud features before the local V1 is excellent.

---

# 117. Preferred Engineering Decisions When Ambiguous

If the specification does not answer a minor implementation question, choose the option that best satisfies this order:

1. Protect source-code privacy.
2. Minimize user setup.
3. Preserve correctness of analysis.
4. Keep V1 local.
5. Keep architecture analyzer-independent.
6. Keep Windows installation simple.
7. Keep UI simple.
8. Prefer explicit failure over silent behavior.
9. Prefer stable machine-readable integration APIs.
10. Prefer maintainable straightforward code over clever abstraction.

---

# 118. Future Analyzer Possibilities

Not V1 requirements.

The architecture should make it possible to add:

Python:

- mypy.
- Bandit.
- Pyright.

JS/TS:

- ESLint.
- TypeScript compiler diagnostics.

Cross-language/security:

- additional SARIF tools.
- dependency vulnerability analyzers.
- secret scanners.

Do not implement these merely because the interface exists.

---

# 119. Future Product Possibilities

Not V1:

- Compare arbitrary scans.
- Trend graphs.
- Watch mode.
- Git pre-commit scan.
- GitHub Actions.
- PR annotations.
- IDE integration.
- Custom analyzer plugins.
- Configurable rule policies.
- Baseline suppression.
- SARIF export.
- JSON export.
- Static HTML export.
- Shareable sanitized reports.
- Multi-repo dashboard.
- Linux/macOS.
- Team server.
- External SonarQube mode.
- Plugin marketplace.

---

# 120. Suggested V1 Home Screen Copy

Headline:

> **Your code. Analyzed locally. Without the setup circus.**

Supporting line:

> Blunt Code runs local code-quality tools behind one simple interface and combines the results into one report.

Primary action:

> **Add Workspace**

Privacy note:

> Your source code stays on this computer.

This copy is optional, but captures product intent.

---

# 121. Suggested Scan Summary Example

```text
Analysis complete

3,412 source files considered
286 source files analyzed
4 analyzers completed
0 analyzers failed

Critical   0
High       3
Medium     18
Low        27
Info       9

New        6
Fixed      11
Persistent 40

Top area: Maintainability
Most affected file: src/services/example.ts

[View Report] [Export Markdown]
```

Do not calculate "most affected file" if data is incomplete in a misleading way.

---

# 122. Suggested Partial Summary Example

```text
Analysis completed with warnings

Ruff       Completed
Biome      Completed
Semgrep    Completed
SonarQube  Failed

The report contains results from the analyzers that completed.
SonarQube-only findings from previous scans are not marked as fixed.

[View Report] [Repair SonarQube] [View Details]
```

---

# 123. Acceptance Scenario — Python Project

Given:

- Fresh Blunt Code install.
- Python repository.
- No global Ruff.

When:

1. User adds repository.
2. Clicks Analyze.

Then:

- Blunt Code detects Python.
- Ruff is automatically prepared.
- Semgrep is prepared if enabled.
- SonarQube is managed if enabled.
- Selected Python files are analyzed.
- Findings appear in one report.
- User can export Markdown.
- No source is sent to a remote Blunt Code service.

---

# 124. Acceptance Scenario — TypeScript Project

Given:

- TypeScript repository with `node_modules`.

When:

- Workspace is discovered.

Then:

- TypeScript is detected.
- `node_modules` is excluded by default.
- Biome is selected.
- User is not required to install Node globally just for Blunt Code's managed analyzer path if the chosen Biome distribution is standalone.
- Report shows Biome and other enabled analyzer results.

---

# 125. Acceptance Scenario — Mixed Project

Given:

```text
backend/*.py
frontend/*.ts
frontend/node_modules/*
```

Then:

- Python and TypeScript detected.
- Ruff targets Python.
- Biome targets JS/TS.
- Semgrep targets supported selected source.
- SonarQube scans configured selected scope.
- Results merge without duplicate database model types.

---

# 126. Acceptance Scenario — Offline

Given:

- Blunt Code and analyzers already installed.
- Network disconnected.

When:

- User scans saved workspace.

Then:

- Normal analysis starts.
- No mandatory cloud request occurs.
- Installed analyzers work.
- Report is generated.

If an analyzer dependency is missing:

- It is marked unavailable.
- User is told Offline Mode/network prevents installation.
- Other analyzers continue.

---

# 127. Acceptance Scenario — Analyzer Failure

Given:

- SonarQube fails.
- Ruff succeeds.

Then:

- Scan status is `completed_with_warnings`.
- Ruff findings appear.
- Sonar failure is visible.
- Previous Sonar-only findings are not reported as fixed.
- Markdown report documents incomplete coverage.

---

# 128. Acceptance Scenario — Restart

Given:

- Workspace scanned yesterday.

When:

- User closes and restarts Blunt Code.

Then:

- Workspace appears in recent list.
- File-selection preferences remain.
- Previous report remains.
- User clicks Analyze without reconfiguration.
- Current scan compares with previous compatible scan.

---

# 129. Acceptance Scenario — Removing Workspace

When user clicks:

**Remove Workspace**

Blunt Code must say clearly:

- This removes the workspace from Blunt Code.
- It does not delete project files.

If scan history deletion is a separate choice, make that explicit.

No source file should be deleted.

---

# 130. Repository Documentation Required Before Public Release

## `README.md`

Must include:

- What Blunt Code is.
- Screenshots.
- Windows requirement.
- One-command install.
- `bluntcode` launch.
- Privacy behavior.
- Supported languages.
- Analyzers.
- Offline explanation.
- Uninstall.
- Contributing.
- License.

## `SECURITY.md`

Include:

- How to report vulnerability.
- Local security model.
- Supported versions.

## `THIRD_PARTY_NOTICES.md`

Include all shipped/downloaded dependencies as required.

## `CONTRIBUTING.md`

Explain:

- Go setup.
- Node setup for frontend development.
- Dev commands.
- Tests.
- Analyzer fixture policy.
- No telemetry/cloud design rule.

---

# 131. Final Architecture Summary

The final V1 architecture should look conceptually like:

```text
+------------------------------------------------------+
|                   User's Browser                     |
|                                                      |
|  React + TypeScript UI                               |
|  Workspaces | File Tree | Progress | Reports         |
+--------------------------+---------------------------+
                           |
                           | localhost HTTP + SSE
                           |
+--------------------------v---------------------------+
|                 bluntcode.exe (Go)                   |
|                                                      |
|  API                                                  |
|  Workspace Service                                    |
|  Discovery                                            |
|  Scan Orchestrator                                    |
|  Analyzer Registry                                    |
|  Report Engine                                        |
|  Tool Manager                                         |
|  SQLite                                               |
|  Windows Native Integration                           |
+-----+-------------+-------------+-------------+------+
      |             |             |             |
      v             v             v             v
    Ruff          Biome        Semgrep       SonarQube
                                               |
                                               v
                                         SonarScanner
```

Everything is local.

The browser is only the interface.

The Go process owns the filesystem, analyzers, local processes, persistence, and report generation.

---

# 132. The Core Product Contract

A successful implementation must preserve this promise:

> **Blunt Code turns local static analysis into a one-click workflow.**
>
> The user should not need to know how Ruff, Biome, Semgrep, SonarQube, SonarScanner, Java runtimes, local ports, analyzer output formats, or report normalization are wired together.
>
> They choose a project, choose what they care about, click Analyze, and get one clear local report.

That is the product.

Anything that makes the user manage the underlying analyzer infrastructure manually is a regression against the central idea.

---

# 133. Implementation Priority

When tradeoffs appear, optimize for:

**Simple user workflow > number of features.**

**Reliable local analysis > clever architecture.**

**Clear partial results > all-or-nothing failure.**

**Privacy > analytics.**

**Deterministic tooling > AI dependency.**

**Persistent workspace behavior > stateless one-off scans.**

**Analyzer abstraction > SonarQube-specific coupling.**

---

# 134. Initial Technical Decision Record

The following decisions are considered accepted for V1 unless a concrete implementation blocker is discovered:

| Area | Decision |
|---|---|
| Product | Blunt Code |
| OS | Windows only |
| Backend | Go |
| UI | React + TypeScript + Vite |
| App shell | Local browser |
| Desktop framework | None |
| Storage | SQLite |
| Backend transport | localhost HTTP |
| Live progress | Server-Sent Events |
| Workspace selection | Native Windows folder picker via backend |
| Python analyzer | Ruff |
| JS/TS analyzer | Biome |
| Security analyzer | Semgrep |
| Broad analysis | SonarQube + SonarScanner |
| AI | None |
| Cloud | None |
| Telemetry | None |
| Export | Markdown |
| In-app report | Rich HTML UI |
| CI integration | Deferred |
| License | MIT for Blunt Code source |
| Dependency handling | Automatically managed |
| Normal scan connectivity | Offline-capable after setup |

---

# 135. Final Instruction to the Builder

Treat this document as the source of truth for the first implementation.

Do not spend early development time inventing additional product features.

Build vertical slices.

The first meaningful success is not "all abstractions compile."

The first meaningful success is:

> A Windows user can point Blunt Code at a real Python or TypeScript repository, click one button, and receive a persisted combined report without manually configuring the analyzers.

Then harden it until that experience is boringly reliable.
