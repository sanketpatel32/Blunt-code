# Changelog

All notable changes to Blunt Code are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.16.13] - 2026-09-01

### Added
- **Gitleaks 8.30.1 joins the managed pipeline.** The Tools page's Gitleaks row is real now: a pinned, SHA256-verified gitleaks download ships behind the same installer as Ruff and Biome, auto-installs on first standard/deep scan, and runs a directory-wide secrets sweep (`--no-git`) over the whole workspace — not just discovery-selected files — honoring the workspace's own `.gitleaks.toml` and `.gitleaksignore`. Findings land with the same shape as every analyzer (rotate-and-move-to-a-manager remediation, severity by credential class: AWS keys and private keys critical, everything else high), secret values stay redacted to a 4-character preview + length + entropy, and the scratch report holding full values is deleted as soon as it is parsed.

## [0.16.12] - 2026-09-01

### Fixed
- **Scan reports no longer mislabel the app version.** A second version constant inside the scan pipeline had drifted to 0.16.7 while the app shipped 0.16.11 — every scan report and API snapshot claimed `bluntcode_version 0.16.7`, and incremental scan reuse wrongly survived version upgrades that should have invalidated it. The version now lives in one place (`internal/build`) that the CLI, API, and scan pipeline all read; bumping `cmd/bluntcode/main.go` alone can never drift again.
- **Semgrep now self-heals stale rules.** Machines that installed before v0.16.1 carry a rules pack whose invalid YAML semgrep rejects, so every scan failed with no automatic recovery. The readiness check now compares the extracted pack's `RULES_VERSION` marker against the bundled one; a stale pack reads as not-ready and the scan's install path re-extracts the current 25-rule pack automatically — no manual `doctor --fix` needed.
- **Semgrep failures now say why.** Configuration errors surface from semgrep's JSON output instead of an empty `semgrep exited 7:`.
- **CLI usage line lists flags before the path** — `bluntcode <path> --no-browser` exited 2 because Go's flag parser stops at the first positional; the usage text now shows the working order (`bluntcode [--no-browser] [--port N] [path]`).

## [0.16.11] - 2026-09-01

### Fixed
- **Agent docs and catalog match reality.** llm.txt/llms.txt claimed a 20-rule semgrep pack (the bundled pack has 25 rules), and the analyzer catalog listed SonarQube as deep-only when the backend runs it on standard scans too. Both corrected.

## [0.16.10] - 2026-09-01

### Fixed
- **The pentest page no longer fabricates results.** Every template carried hardcoded fake run history (e.g. Log4Shell showing "fail · 2 findings · 890ms" as if it had executed) — all of it is stripped; every template now truthfully reads "not run". Hacking-test buttons are relabeled **Preview** (they never executed anything), and the page copy no longer claims ZAP/Nuclei/Burp analyzers run on deep scans — the three pentest preview rows are gone from the analyzer catalog, so the Tools page "Coming soon" section is gone too. The catalog now lists exactly the six analyzers that really run.
- **"Run pentest" became "Run deep scan" with a workspace picker.** It previously fired a deep scan at the first workspace it found with no way to choose and no navigation; now you pick the workspace in the header, the scan you started opens when it launches, and the status message says plainly that static analyzers run today and DAST is planned.

## [0.16.9] - 2026-09-01

### Removed
- **Dead tool manifest entries and phantom catalog rows are gone.** The download manifest carried six pinned-but-unreachable entries (Gitleaks, OWASP ZAP, Nuclei, OSV Scanner, Trivy, Checkov) — real URLs and checksums, but no analyzer adapter exists, the install API 404s on them, and no code path ever references them. The analyzer catalog likewise advertised seven analyzers that never run under any profile (Snyk, Dependabot, Gitleaks, OSV, Trivy, Checkov, License Scan); the Tools page "Coming soon" section, language coverage matrix, and workspace templates no longer present them. Workspace templates that pre-filled phantom analyzers now pre-fill real ones (Ruff, Biome, Semgrep, SonarQube, Secrets, TODO). The three pentest preview entries remain pending the pentest page rework.

## [0.16.8] - 2026-09-01

### Fixed
- **The Tools page no longer buries the built-in Secrets and TODO/FIXME analyzers in "Coming soon".** Both already run on every standard and deep scan, but because they're bundled in-process (nothing to install) the tools API never listed them, so the page showed them as "Not installed" placeholders with disabled install buttons. They now sit in their own "Built-in analyzers" section with a permanent Built-in status and no install actions. The Secrets catalog entry also no longer claims to run on quick scans — quick runs Ruff and Biome only.

## [0.16.7] - 2026-09-01

### Changed
- **Every page now spans the full viewport width** — the boxed content cap and centering are gone, and on wide desktops (≥90rem) the dashboard becomes a multi-column grid: Recent activity and Recent projects side by side under the full-width metric row, with trends and tool readiness in their own band below.
- **No hidden controls left: every option is visible on screen at once.** Scan profile (quick/standard/deep), the severity-trend range, search analyzer/status facets, the report's Tool/Status filters and rows-per-page window, the filter drawer's analyzer/status, and the UI language switcher (nav + Settings) are now segmented pill bars or chip groups instead of `<select>` menus. The Files & rules language filter is a scrollable chip rail keeping per-language file counts.
- **Dropdown menus became flat rows:** workspace cards show Open details / Run scan / Remove as an always-visible action row; the workspace page's "More" menu is inline ghost buttons; the report's Export popover is a flat link row.
- **Dashboard disclosures removed:** the severity breakdown card and the trends/language-coverage section render open instead of hiding behind a collapsed summary.

## [0.16.6] - 2026-09-01

### Changed
- **Workspace section nav is a horizontal bar above the content:** the left rail spent 13rem+ of every workspace page's width on three links. Overview and Files & rules now get the full content width, with the section links as pills in a slim bar at the top.
- **Latest analysis and Scan history stack on the workspace page** instead of sitting side by side — the analysis card was mostly empty and cramped the history table beside it.
- **Scan report page decluttered:** the page title no longer renders the raw state string (it reads "Completed with warnings", not "completed with warnings"); the analyzer list appears once instead of twice (failed analyzers show their error message inline under the category chip); and the flow panel says "How the analysis ran" once a scan is finished instead of "What is happening now".
- **Workspaces filters and sort share one toolbar row** — search/tag inputs on the left, sort controls on the right, wrapping intact on narrow widths.
- **The search column picker is a compact dropdown:** as a full-width bar it stretched to the findings table's own width (~1860px), wider than the page.
- **Language coverage matrix lost its duplicate count row:** the per-language analyzer count was shown under each column header *and* as a bottom "Analyzers" row with identical numbers; the header count stays.

### Removed
- The Files & rules explainer paragraph ("Choose source paths to analyze…") — static instructions that pushed the tree down.

## [0.16.5] - 2026-09-01

### Removed
- **The Workspaces page heading block:** an eyebrow reading "Workspaces" above an h1 reading "Your local projects" — directly under a nav item already called Workspaces — plus a paragraph about file rules nobody reads on a list page. The page now opens straight onto the filter and sort bar; adding a workspace still lives in the nav's + button (and the empty state's templates). An sr-only h1 keeps the accessible heading order intact.

## [0.16.4] - 2026-09-01

### Added
- **Selectable rows-per-page on the findings pagination:** the report's fixed 50-row window becomes a Per page selector (25/50/100/200 — 200 is the API's maximum page size). Switching refetches at the new window and restarts on page 1, and the choice persists in the shareable report URL as `page_size` through back/forward navigation.

### Changed
- **Scan status badges get explicit, honest tones:** the dashboard table's status badge rendered raw state text ("completed with warnings") in a neutral gray, and the workspace cards' badge mis-toned the same state as green via a string-includes guess. A shared `scanStateDisplay` mapping now drives both: completed renders green, **completed-with-warnings renders amber**, failed/cancelled red, running/queued/pending accent, unknown states a neutral outline — each with a sentence-case label ("Completed with warnings", not "completed_with_warnings") kept on one line. The activity feed and history views already toned correctly and are unchanged.
- **Per-finding "Jira ↗" link removed:** it was a non-functional stub pointing at a hardcoded example.atlassian.net URL on every finding row. The real Jira CSV export in the export menu is unaffected.

## [0.16.3] - 2026-09-01

### Changed
- **Language badges carry a per-language color dot:** the pills in the dashboard's Languages column (and everywhere else `LanguageBadges` renders) now lead with a GitHub-Linguist-style colored dot — Python blue, Docker blue, Markdown's deep blue and so on — so a workspace's language mix reads at a glance instead of as a wall of same-looking text. Unknown languages fall back to the app accent, and every dot keeps a faint rule-colored ring so it stays visible on both theme surfaces. The workspace cards' language dots switch from positional accent/success/warning colors to the same per-language colors, raw `markdown`/`text` ids get proper "Markdown"/"Text" labels, and the badges are now real `<li>` elements (the old `<Badge/>` children rendered `<div>`s inside the `<ul>`).

## [0.16.2] - 2026-09-01

### Fixed
- **Dashboard and workspace-card paths rendered the full Windows path:** a deep project path (`C:\Users\…\src\Suremed_agent\complete_prod`) consumed the whole workspace cell and pushed real content out of view. Both surfaces now show the path's last two segments behind an ellipsis (`…\Suremed_agent\complete_prod`) — paths shallow enough that an ellipsis would save nothing stay whole — with the full path still available in the hover tooltip and via a new copy button beside it that puts the complete path on the clipboard (inline check confirmation, legacy fallback for plain-http loopback). Also cleans up three a11y lint errors the 0.16.1 clarity pass shipped in the workspaces sort bar: a semantic `fieldset` instead of `role="group"`, no `aria-sort` on buttons (the `aria-pressed` + sr-only text already carry the state), and no `aria-label` on a plain `div`.

## [0.16.1] - 2026-08-31

### Changed
- **Navigation reduced to three primary destinations plus a "More" menu:** Home, Workspaces and Search stay as direct links; Tools, Pentest, Rules, Settings and About move into a "More" disclosure that also carries Close app (which styles.css used to hide below 63rem entirely). Eight equal-weight links made every navigation a scan-and-choose, and half of them are set-once destinations. The menu closes on outside click, Escape, and after picking a link, and its label ships localized in all six UI languages.
- **Dashboard rebuilt around one metric row and the activity feed:** the separate StatsOverview block is gone (it repeated numbers the summary row already shows), a severity breakdown folds behind a disclosure, and the per-card inline animation styles moved to reduced-motion-aware CSS.
- **Workspace page hero split into identify / act zones:** the hero card is now title-and-status only, the scan profile + "Run scan" pair becomes the single primary action with every other action grouped beside it, the lower sections switch from accordion to tabs, and the completion confetti is skipped for reduced-motion users.
- **Search column picker collapsed by default:** hiding columns is a taste call most people never make, so the picker is now a "Columns" disclosure showing how many are hidden, with human labels instead of internal state keys.
- **Workspaces filter bar simplified:** the dead "Filters" button and the tag dropdown that duplicated the tag text box are gone, and the result count appears only while a filter is active.

### Fixed
- **Scan page right panel pushed off-screen after a scan completed:** `.scan-page` is a grid with no explicit columns, so its single implicit `auto` track sized itself to the widest child's content — once the terminal report (with its wide findings table) rendered, the whole page grew ~450px past the container, and `overflow-x: clip` on html/body cut the right side off with no scrollbar. The "Results so far" panel was unreadable without zooming out. The page track is now `minmax(0, 1fr)` so it never exceeds the container; the findings table keeps scrolling inside its own `.table-wrap`.
- **Semgrep returned zero findings on every scan:** the bundled local rulepack was invalid twice over — unquoted `__html: ` scalars made the YAML unparsable, and the `react-dangerous-html` / `javascript-url-href` rules used `<... X ... />`, which is not valid semgrep pattern grammar. Either failure makes semgrep reject the whole config file, silently disabling all 25 rules. Patterns rewritten to valid grammar (verified with `semgrep --validate` and behavior-tested against live JSX), pack version bumped to 3.1.2, `doctor --fix` re-extracts it, and the rulepack tests now reject both the `": "` plain-scalar YAML class and the `<... ... />` grammar class so this cannot ship silently again. Also deduplicated a repeated `postMessage` pattern.
- **Incremental scans never invalidated across Blunt Code upgrades:** `internal/scans` carried its own hardcoded version constant stuck at `0.5.0` while releases reached 0.16.x, so the incremental-identity cache key never changed between builds and reports mislabeled the version. Now kept in lockstep with the `cmd/bluntcode/main.go` version const (with a comment saying so).
- **SonarQube findings dropped with "compute task timed out" on first analysis:** the compute-engine wait had a hardcoded 5-minute deadline, which a first analysis after server boot (fresh DB, plugin init, ES indexing) regularly exceeds on thousand-file workspaces. Raised to 20 minutes; the scan's `--timeout` context still bounds the overall wait.

## [0.16.0] - 2026-08-29

### Added
- **`npm run audit:contrast`** — WCAG AA regression guard (`web/scripts/contrast-audit.mjs`). Converts `oklch` to linear sRGB and computes contrast ratios for every pairing that actually ships, naming the CSS rule that creates each one, and exits non-zero on any result below 4.5:1 so it can run in CI. All 10 shipped pairings pass.
- **Layout + semantic tokens:** `--nav-h`, `--page-gutter` (with responsive overrides at 68/56/38rem), `--tap-min`, `--color-on-accent`, `--color-sev-critical|high|medium|low|info`, `--color-success-ink|warning-ink|danger-ink`, `--color-success-strong`, `--color-skeleton`, `--color-skeleton-sheen`, `--color-scrollbar-thumb|track`, `--shadow-inset-critical|high|medium`, and `--color-cat-9..13` so the 11 analyzer categories never share a hue.
- **Settings → General language row** — a full-width select so the language switcher stays reachable below 768px, where the nav control is width-gated.

### Changed
- **Severity ramp consolidated (loop 120):** 15 duplicated rules collapsed into one ordered `--sev` scale shared by `.severity-bar`, `.severity-legend`, `.severity-stack`, `.trend-chart` and `.severity-dots`; row severity edges now use `--shadow-inset-*` tokens.
- **Chrome offsets unified (loop 102):** three disagreeing nav-height magic numbers (`3.5rem`/`3.75rem`/`5rem`) replaced by a single `--nav-h`, consumed by `.app-nav`, `.workspace-context`, `.findings-section .section-head` and `.search-sidebar`.
- **Sticky footer (loop 131):** `.app-frame` is now a `100dvh` flex column with `.main { flex: 1 1 auto }`, replacing a `calc(100vh - 5rem)` guess that floated the footer mid-screen on short pages.
- **Scroll affordance without JS (loop 133):** `.table-wrap` uses four background layers — two `background-attachment: local` masks over two `scroll` radial shadows — so an edge shadow appears only when there is real overflow.
- **Stylesheet hygiene (loops 113–119):** removed a duplicate toast block that shadowed `@keyframes toast-in`, merged a dead duplicate `.tree-toolbar`, deleted an empty `:hover` rule, dropped `backdrop-filter` from `.filters`, and stripped `will-change` from 7 static elements.
- **Typography & layout (loops 129–134):** `text-wrap: pretty/balance` on prose and headings, `tabular-nums` on metric lists, `[id] { scroll-margin-top }` so anchors clear the sticky nav, `hyphens: auto` + `overflow-wrap: anywhere` for long paths, and print rules with `@page { margin: 14mm }`.
- **Accessibility (loops 123–128):** 44px hit pads under `(pointer: coarse)`, a `prefers-contrast: more` block that promotes hairlines to full rules — restated inside `:root[data-theme='dark']` so higher specificity cannot bypass it — and custom `:focus-visible` rings for row-like controls.
- `--color-*-strong` tokens now invert direction per theme: light goes darker, dark goes brighter, because "strong" means maximum contrast against that theme's ink.

### Fixed
- **Four WCAG AA failures found by measurement (loop 141):** scan-flow checkmark on a filled success circle 4.22:1 → **5.27:1**; dark 10px unread-count badge 3.23:1 → **5.77:1**; light `--color-ink-faint` on paper 4.47:1 → **4.66:1** (was failing by 0.03); dark `--color-ink-ghost` 3.93:1 → **4.64:1**.
- **Undefined `--color-danger-ink`** — referenced by `ui/button.tsx` but never defined, so destructive button labels silently fell back to inherited ink. Added the three semantic ink tokens.
- **Dark mode was unreachable below 640px** — the only theme toggle in the app carried `hidden sm:`. Now always visible.
- **Footer showed `v0.7` while the app shipped 0.15.0** — version is injected at build time from `package.json` via Vite `define` (`__APP_VERSION__`).
- **Invalid `aria-expanded` on `<tr>`** — unsupported by `role="row"`; replaced with an `aria-label` on the row plus screen-reader-only expanded-state text.
- **`.skeleton-chart` clipped ~18rem of grid content** — fixed `height: 9.5rem` changed to `min-height`.
- **`THEME_COLORS` and the `theme-color` meta used warm hexes** (`#fbf9f7`/`#1a1c20`) that never matched the cool oklch tokens; recomputed to `#ffffff`/`#11141a`.
- Toast live region had no accessible name, so screen readers announced bare text with no context.
- 17 hardcoded `white` foregrounds on coloured fills and 11 raw hex values in `analyzerCatalog.ts` replaced with tokens.

## [0.15.0] - 2026-08-28

### Changed
- **Workspace Overview premium:** hero bento `radius-card shadow-card` radial dot `::before` opacity .04 + `::after` gradient 2px, h1 2.5rem -0.05em 700 + copy button, LanguageBadges dots, Next step 20rem `shadow-accent` Sparkles Run + View report ghost, toolbar `workspace-toolbar` Files/History ghost + Export + More dropdown, summary 2x3 grid PremiumSummaryCard 6 with ShieldAlert/BarChart3 spark minis + stagger 40ms, donut 180 + pill legend, `workspace-section-card` `will-change`.
- **Files & rules editorial:** header hero eyebrow + `workspace-root` chip + toolbar bento Reset/Save, layout `1.2fr 0.8fr gap lg` cards `radius-card shadow-card`, tree-panel header Source tree + count badge tabular-nums, search `file-search-wrap` Search icon + kbd `/` + `file-lang-select`, toolbar Collapse + `tree-summary-bar`, tree-row hover 4% accent stagger 20ms + `tree-hit` pill + children border + `tree-item` animation, rule-editor `Overrides` eyebrow + dashed empty + segmented Include/Exclude + `rule-add` dashed Plus.
- **Scan history timeline:** filter bar bento mono 0.68rem uppercase Calendar inputs `radius-button` glow + Clear ghost, table outer `history-table-wrap` `radius-card overflow-hidden` sticky header widest 0.08em, rows hover 5% accent is-expanded, timeline vertical line `tbody::before` + dot `history-date::before` accent when expanded, band header sticky count pill `::after`, severity bar 14px pill `padding 2px gap 2px`, detail bento `radius-md p-4` 2-col meta + pills left 3px accent, pagination bento `Previous<svg>Next` (no space, textContent exact) tabular-nums.

### Fixed
- History pagination `textContent === \"Next\"` — removed space before SVG (`>Previous` / `Next<svg`) for strict equality, `FilesPage` Save button icon removed to keep `textContent === \"Save selection\"`, 259 tests green.


## [0.14.0] - 2026-08-28

### Changed
- **Workspace Files premium:** bento `radius-card shadow-card` `file-layout 1fr 20rem gap lg`, tree-panel elevated `Source tree` eyebrow + count badge tabular-nums, search `file-search-wrap` Search icon + kbd `/` focus ring, language select `file-lang-select`, toolbar gap xs `Collapse all` ghost, `tree-row` 2.6rem hover 5% accent, toggle 1.5rem chevron, `tree-lang-badge` 10px muted, `tree-hit` pill accent-soft, children left border faint + `height 200ms` `will-change` (reduced-motion disables), `tree-item` stagger 20ms + `tree-hit` pill, RuleEditor card `rule-editor-head` eyebrow + empty dashed + `rule-add` dashed ghost Plus.
- **Workspace History premium:** filter bar bento `history-filter-bar` mono 0.68rem uppercase + Calendar date inputs `radius-button` focus glow + Clear ghost, table outer `history-table-wrap` `radius-card overflow-hidden` sticky header mono widest 0.08em, rows hover 5% accent `is-expanded` 50%, band header `::after` pill count (keeps textContent for tests) + `history.css` spring rotate 90deg, `FindingsCell` bar 4px pill tabular-nums, detail bento `radius-md p-4` 2-col meta + pills `border-left 3px accent`, pagination `tabular-nums` + `Page X of Y` mono.


## [0.13.0] - 2026-08-28

### Changed
- **Analysis story premium — shape + dynamism:** hero bento `radius-card shadow-card` gradient top `var(--color-accent-gradient)` 2px, h1 tight -0.04em 700, subtitle mono 0.75rem faint `tabular-nums` elapsed, `scan-progress-card` 4px bento shimmer `linear-gradient 90deg transparent/var(--color-accent)/transparent` sweep 1.6s spring `ease-spring will-change width`, terminal done `success`/`failed` danger, `ConfettiStub` 12 dots burst stagger 35ms `520ms ease-out-quart`, `LiveAnalyzerStrip` grouped per `analyzerMeta` category `borderLeftColor categoryColor` 3px + label, pills stagger 40ms `scan-fadeInUp` `will-change`, `ScanStageList` `stage-enter` stagger 40ms `flow-now` spinner `flow-marker-spring` current pulse shadow `done ✓ white on success` `failed danger`, `scan-side` elevated card `scan-side-head accent-soft` metric `2xl tabular-nums`, `progress-layout` gap lg bento, `live-category` colored left border per category, `will-change` + `prefers-reduced-motion` disables.


## [0.12.0] - 2026-08-28

### Changed
- **Workspace cards premium:** top accent bar 3px `var(--color-accent)`, shield icon `ShieldCheck` `accent-soft`, name 18px 700 -0.02em, path mono 12px faint, languages as colored dots `ws-lang-dots` + `LanguageBadges` sr-only, analysis metric 28px tabular 700 -0.03em, footer single primary `Run scan` accent `shadow-accent` + ghost `Open details`, dropdown `Remove`, grid `24rem` gap lg, card `rule-faint` `radius-card` `shadow-card` hover tint.
- **Workspace detail hero:** eyebrow Workspace h1 tight -0.04em, path mono + `LanguageBadges`, right `workspace-next` 18rem accent top 2px, profile select + `Run scan →` large `shadow-accent`, toolbar single line Files/History ghost + Export Markdown outline + `More` dropdown Settings/Prune/Remove, summary-grid 5 bento `clamp 1.75-2.25rem`, tabs Overview/Analysis/History `workspace-tab-panel`, analyzer-group cards `categoryColor` left border.

### Fixed
- Workspace page density: action-row soup 6 secondary → single toolbar, sidebar active `surface-muted` not ink, summary hover `accent 14%`, no lift translateY.


## [0.11.1] - 2026-08-28

### Fixed
- **Vendor chunk circular** `vendor-3pAXeipQ.js:9 useLayoutEffect` — `vite.config.ts:17` manualChunks now single `vendor` (was split `vendor-react/radix/icons/ui` causing circular `vendor→vendor-react`), main `213.97 kB` + `vendor 367.46 kB` split `chunk-charts/pentest/editor/autofix/compliance` clean, no error.

### Added
- **Logo v2 Shield Chevron:** `web/public/bluntcode-mark.svg:1` + `cmd/bluntcode/static/bluntcode-mark.svg:1` redesigned from chevron `>` + line to shield outline (`M16 5.2 L23.6 9 … C23.6 21 20.2 24.5 16 26.8`) + check `M11.8 15.9 L15.2 19.1 L20.6 12.1` + dot `r1.7 #3B82F6`, `rx 8` + subtle depth, works at 16px favicon → 512px store, `AppShell.tsx:24` inline SVG tokenized `fill var(--color-brand-mark)` + `stroke var(--color-paper) 0.14/0.96` + `fill var(--color-brand-accent)` dot for dark/light paper contrast, `public/logo-showcase.html:1` 6 variants (Shield/Chisel/Brackets/B/Flow/Line) + recommendation.

### Changed
- **Logo availability:** `web/index.html:6` favicon `href /bluntcode-mark.svg` now serves new shield from `public/` + `cmd/bluntcode/static` Go embed, both kept in sync.


## [0.11.0] - 2026-08-28

### Added
- **Advanced Data Grid:** column pinning (sticky left + shadow), column resizing (drag + keyboard Arrow, persisted `bluntcode.colWidths` 80-600), row height var `--row-height`, row expansion (`aria-expanded`, remediation + related findings), bulk bar (Suppress/Export CSV/Copy) + toolbar selectedCount/badge/column-visibility (`DataTable.tsx`, `table-toolbar.tsx`, `ReportView.tsx`).
- **Hacking Suite expansion:** 22 templates SQLi/XSS/SSRF/XXE/IDOR/auth bypass/traversal/CMDi/open redirect/CSRF/upload/JWT/GraphQL/pollution/SSTI with owasp A01-A10 cwe payload remediation (`hackingTests.ts`), 4th tab “Hacking Suite” grouped by OWASP search severity toggle sheet, stats card (`PentestSuite.tsx`, `PentestPage.tsx`, `analyzerCatalog.ts` hacking `#be123c`).
- **Languages 42 + Compliance:** 8 more `Haskell/Clojure/Erlang/F#/Lua/Zig/OCaml/Perl` + Functional family, `secrets/todo` cover all 42, semgrep expanded, `ComplianceMatrix.tsx` OWASP Top10 + CWE Top25 table progress click filters report, `LanguageCoverage.tsx` 42 + `FilesPage.tsx` languageIcon + `LanguageDistributionDonut` drill.
- **Animations v2 + Filters v2:** `animations.css` slideInLeft/Right scaleIn spring fadeInUp stagger skeleton toast dialog tab bar, `QueryBuilder.tsx` grouping parentheses drag GripVertical duplicate clear a11y, `SavedViews.tsx` rename duplicate export/import pin star, `NotificationsCenter.tsx` tabs All/Unread/Scans groupByDay markRead per item clear sound toggle, `FilterDrawer.tsx` backdrop blur slideInRight 200ms.

### Changed
- Data grid bulk/tool bar polish, hacking suite categorized, compliance matrix integrated below DependencyGraph.


## [0.10.0] - 2026-08-28

### Added
- **Loop continue — Monaco + Pentest + Languages + Perf + Query Builder:** `CodeEditor.tsx` (line numbers, syntax highlight overlay, Tab 2-spaces, Ctrl+S, error wavy, Monaco dynamic import) for `RuleStudioPage.tsx` (language select, Snippets, validateYaml), `PentestSuite.tsx` (ZAP/Nuclei/Burp tabs, combobox search, severity filter, enable toggle, sheet drawer target/auth/scope) + `pentestTemplates.ts` 12+8+4, `PentestPage.tsx` stats + Run pentest deep, expanded `analyzerCatalog.ts` 34 languages (C/C++/C#/Ruby/PHP/Rust/Swift/Kotlin/Scala/Dart/Elixir) + `LanguageCoverage.tsx` 20+ grouped + CSV export + File `languageIcon` + `LanguageDistributionDonut` drill to `FilesPage?lang=`, `VirtualizedList.tsx` windowed + `vite.config.ts` manualChunks (`vendor-react/radix/icons/ui`, `chunk-charts/pentest/editor/autofix`, main 202k) + lazy `PentestPage/RuleStudioPage` + `AnalyticsCharts` etc + memo `SeverityDistribution/AnalyzerResults` primitive deps, `QueryBuilder.tsx`/`SavedViews.tsx`/`queryBuilder.ts` advanced filter builder AND/OR + URL ↔ `FindingFilter` + `ReportView.tsx`/`SearchPage.tsx` Advanced sheet 420px.

### Changed
- Perf code-split: circular chunk warning fixed via manualChunks, Go `go vet/test` clean.


## [0.9.0] - 2026-08-27

### Added
- **AI Auto-Fix Panel:** per-finding AI fix suggestions (Before/Fix diff, copy, regenerate, confidence badge, disclaimer) in ReportView sheet drawer (`web/src/components/AutoFixPanel.tsx`, `lib/aiFix.ts`, `Sparkles` icon).
- **Analytics Charts:** 3 hand-rolled SVG charts (findings over time line-area, severity donut, language coverage bars) + `MiniSparkline` sparkline in workspace table + `chartData` helpers (`AnalyticsCharts.tsx`, `HomePage.tsx`, `WorkspaceDetailPage.tsx`).
- **Comments & Notifications:** per-finding comment thread (localStorage `bluntcode.comments.*`, avatar, relativeTime, ⌘+Enter) + bell `NotificationsCenter` dropdown with unread badge (`AppShell.tsx`, `CommentsPanel.tsx`, `notifications.ts`, push on `scan.completed`).
- **Rule Studio + Templates:** custom rule YAML editor (live preview, localStorage `bluntcode.customRules`, enable toggle) + 6 workspace templates (Python FastAPI, TS React, Go CLI, Java Spring, Pentest Lab, Full Hack Suite) with languages/analyzers chips, `Rules` route/nav, template prefill (`RuleStudioPage.tsx`, `WorkspaceTemplates.tsx`, `router.ts`).
- **Dependency Graph + Jira Export:** SVG force graph (zoom/pan/drag, tooltip, reduced-motion) below trends (`DependencyGraph.tsx`), Jira CSV export + per-finding “Jira ↗” link in ReportView ExportMenu + FindingsTable (`JiraExport.tsx`, `lib/csv.ts`).

### Changed
- Chunk 574 kB (code-split warning) — additive features, no breaking routes, all 259 tests green.


## [0.8.0] - 2026-08-27

### Added
- **50-loop major UI overhaul via subagents (Luna + Claude CLI):** shadCN primitives `select/tabs/dropdown-menu/popover/tooltip/sheet/accordion/calendar/combobox` (`web/src/components/ui/*`), `DataTable` generic (sort/facet/column-toggle/density/pagination/bulk) + `table-toolbar`, filter drawer 380px + quick filters + saved presets (`filterPresets` ↔ URL ↔ localStorage), command palette grouped navigation/filters/workspaces + recent, file-tree language filter + highlight, i18n provider 6 locales `en/es/fr/de/ja/hi` + `AppShell` language select, central `animations.css` (GPU, stagger 40ms, reduced-motion), skeletons/toasts spring, `useStagger/useReducedMotion`, scan live strip category grouping, dashboard accent-gradient bar, `analyzerCatalog` 16 types (snyk/zap/nuclei/gitleaks/osv/trivy/checkov), `LanguageCoverage` matrix, `PentestPage` + Tools pentest section, manifest stubs 6 tools, route `pentest`, i18n keys.

### Changed
- **Tables premium:** sticky header 5% hover, tabular-nums, `content-visibility`, row stagger, severity edge tint only high/critical, column toggle, dropdown actions.
- **Filters make life easy:** toolbar + chips (removable) + severity soft `accent-soft`, drawer per page remembers `bluntcode.filters.*`, URL shareable + back/forward, search debounce 200/250ms preserved for tests.
- **History/Search/Workspaces polished:** date filter hook, facet sidebar accordion sticky→sheet mobile, saved searches.

### Fixed
- Locked test regressions from overhaul: `CommandPalette` hooks order, `i18n` fallback, `HistoryPage` band headers `Today` plain, `LanguageBadges` restore `Python`, `ToolsPage` tool-table vs coming-soon split, `FilesPage/SearchPage/WorkspacesPage` debounce + native input preservation — 259 tests green.


## [0.7.2] - 2026-08-27

### Added
- **Premium UI v2 — substantial redesign:** Vibrant cobalt `56% 0.22` + visible `shadow-card` depth, soft-rect radii (`10px` button / `16px` card, pill only badges), minimal nav (no glass, hairline `faint`, active `muted 600`), breathing page `4.5rem/2xl` + bento `4-col` metrics `clamp 1.75-2.25rem`, clean tables `faint border + card shadow`, solid empty states, soft severity `accent-soft` pills, `text-wrap balance` hero `-.05` tracking, `isolation isolate` + `contain layout` perf.

### Changed
- **Design system tokens:** `tokens.css` — paper `98.6% cool`, `accent-glow 0.16`, `radius-button/card` split, `shadow-card/hover/accent` lifted, `space 3xl 4.5rem`.
- **Components:** `AppShell` tokenized brand mark + `rounded-button` + `shadow-accent` Add, `Button` `accent` primary both themes (was black), `HomePage` dashboard eyebrow + bento polish.
- **Styles:** `styles.css` — Hallmark `P5 H5 E5 S5 R5 V5` 100-loop v2, no lift `translateY`, no pill fatigue, `content-max 86rem` centered.

### Fixed
- Locked tokens: removed `#0f172a`/`#3b82f6` hex slop via `var(--color-brand-mark/accent)` — Hallmark gate 48.
- Button/shadow regression: `rounded-full` → `radius-button`, `translateY` removed per audit.


## [0.7.1] - 2026-08-27

### Added

- **Versioning ship rule:** `AGENTS.md` now mandates **bump version on every shipment** — `cmd/bluntcode/main.go` `const version`, `web/package.json` (+ `web/package-lock.json`), `scripts/package.ps1` `$Version`, `CHANGELOG.md` promotion, commit + `git tag -a vX.Y.Z` + `git push origin main` + `git push origin vX.Y.Z` + `gh release create` via `scripts/package.ps1 -Version X.Y.Z`.

### Changed

- Packaging default version bumped to 0.7.1.
- Project `AGENTS.md` created at repo root with graphify + versioning rules (global `C:\Users\sanpa\AGENTS.md` updated).

## [0.7.0] - 2026-08-26

### Added

- **shadcn clean UI — 10-loop iterative rebuild:** Tailwind 3.4 + `clsx`/`tailwind-merge`/`cva` foundation (`web/tailwind.config.js`, `postcss.config.js`, `src/lib/utils.ts:1` `cn()`), new primitives `Button`/`Card`/`Badge`/`Input`/`Table`/`Dialog`/`Skeleton`/`Separator` under `src/components/ui/*` (Radix Dialog/Slot + lucide-react) — app shell glass nav, card lift, badge tokens (`oklch` mix), table hover tint `color-mix(in oklch, var(--color-accent) 4%)`, dialogs with `Input`+`Button outline/destructive`, toasts spring + lifetime bar, skeletons shimmer `220%` — build `1869 modules`, tests `31 suites 259 pass`.
- **In-app updates:** the About page gains an **Updates** card — see the installed version, check GitHub releases for a newer one, and update without leaving the app (`GET /api/v1/update/check`, `POST /api/v1/update/apply`). Applying stages the official installer detached and stops the app so the installer can swap the binary; offline mode blocks both endpoints with `UPDATE_OFFLINE`.
- **Installer v2:** `install-latest.ps1` supports `-Version x.y.z` pins, `-Silent`, `-DesktopShortcut`, `-WhatIf` dry-run plans, 64-bit/disk pre-flight checks, honest upgrade/reinstall/downgrade labels (asks the installed exe), automatic rollback if the install swap fails, and `-WaitForCloseSeconds` for the in-app updater handoff.
- **Scan comparison endpoint:** `GET /api/v1/scans/{id}/compare?with=<scan-id>` diffs two scans with the same coverage-aware new/fixed/persistent semantics as reports (implicit previous-completed resolution when `with` is omitted, suppressed fingerprints filtered from both sides).
- **JSONL everywhere:** `--format jsonl` on headless scans and a matching `GET /api/v1/scans/{id}/findings.jsonl` download render findings as newline-delimited JSON (export ordering, derived status per row) for log pipelines and `jq`.
- **Markdown CLI format:** `--format markdown` prints the full rendered report to stdout or `--output`.
- **Suppression round-trip:** `GET /workspaces/{id}/suppressions.csv` exports dismissed fingerprints (BOM, formula-neutralized) and `POST .../suppressions/import` re-ingests that CSV with per-row accounting (`imported` / `skipped_invalid` / `duplicate`).
- **API mutation rate limiting:** state-changing requests pass a token bucket (30 capacity, 30/min refill); exhausted callers get `429 RATE_LIMITED` with `Retry-After`.
- **Doctor database self-check:** `bluntcode doctor` runs SQLite `PRAGMA quick_check` and surfaces ok/warn status (absent database is an informational skip).
- **Risk-grade distribution:** `GET /api/v1/stats` now includes `risk_grades` counting A/B/C/D across every workspace's latest completed scan.
- **Report duration chips:** analyzer runs render as success/failed chips with durations in the report header.
- **Workspace tags surface:** cards show up to three tag chips (+N overflow) and the page gains a case-insensitive *Filter by tag* input.
- **Risk badges:** workspace cards display each workspace's A–D grade when the API provides one.
- **Suppressions CSV export:** the suppressions panel downloads its rows as a BOM'd, RFC-4180-safe spreadsheet.
- **Palette indexing note:** the command palette footer reports how many workspaces its dynamic entries cover.
- **Table accessibility:** every data table gained an sr-only caption describing its contents.
- **Installer console experience:** the one-line installer now prints stepped progress (`[1/5]` … `[5/5]`: release info, download, checksum verify, install, shortcut) with a live single-line download meter (`4.2 / 11.3 MB (37%)`) that redraws in place when attached to a console and degrades to one coarse line every few MB in redirected/CI logs, plus a version-aware summary ("Installed Blunt Code 0.5.0 to …"). Output stays plain ASCII so piped `irm | iex` sessions on Windows PowerShell 5.1 decode it cleanly.
- **`g a` goes to About:** every destination in the top navigation is now reachable from the keyboard — the shortcuts help gains the `g` `a` row, and the command palette's "Go to About" entry shows the `g a` hint like its siblings.

### Changed

- **UI interaction layer:** 10-loop shadcn rebuild + prior 15 polish loops — button press/busy feel, table-row hover tint, card lift, sliding nav underline, copy-pop confirmation, toast entrance, breathing critical counts, floating empty states, command-palette active step, unified focus rings, tabular numerals, sticky glass filter heads, gliding progress fills, staggered dialog entrances, native-feeling dark elevation, shadcn glass nav + pill badges.
- Packaging default version bumped to 0.7.0.

### Removed

- **GitHub Actions workflow:** `.github/workflows/ci.yml` deleted — CI gated locally via `go vet`/`go test`/`npm test` + self-scan dogfood; no remote runner required. `.github/` removed from repo.
- **Legacy standalone installer script:** `scripts/install.ps1` (the download-and-run-a-local-copy installer superseded by `install-latest.ps1`) is gone — the `irm … | iex` release-asset one-liner is the single supported PowerShell install path. The README's execution-policy notes no longer walk users through saving and running an installer `.ps1`; they explain that the piped one-liner never touches Execution Policy at all.

### Fixed

- `.gitattributes` now forces LF for the bundled semgrep rulepack so checkouts never corrupt it (test previously failed with "must keep LF line endings").
- **Header pill no longer overflows:** at common desktop widths the *Add workspace* button spilled past the nav pill's rounded border, because the pill was hard-capped at 52rem while its contents (brand, six nav links, four action buttons) need ~65rem. The pill now hugs its content (`fit-content`, still viewport-capped with a 52rem floor), a measured slimming ladder sheds the theme label, *Close app*, and the shortcuts button as the viewport narrows, and the nav links scroll as a last-resort safety net instead of pushing buttons out of the pill.
- **Add-workspace placeholder shows real Windows paths:** the folder-path field suggested `C:\\Projects\\my-app` with doubled backslashes, because JSX attribute strings are raw text where backslashes are not escapes. It reads `C:\Projects\my-app` now.
- **Network failures explain themselves:** when a request never reaches the local server (usually because the app window was closed while a tab stayed open), toasts now say the server is unreachable and how to restart it instead of surfacing the browser's raw "Failed to fetch". A repeat of the currently visible toast refreshes it in place rather than stacking identical copies.
- **Uninstaller cleans up its shortcut:** the one-line installer creates a Start-menu `Blunt Code.lnk`, but the uninstaller left it behind as a dangling link after removing the app (the README already promised shortcut removal). It now deletes the shortcut and refuses to run while Blunt Code is open, mirroring the installer's guard. Shipped as a clobber-refresh of the v0.5.0 ZIP and installer assets.
- **Installer output survives `irm | iex` on Windows PowerShell 5.1:** GitHub serves the installer script as `application/octet-stream`, so the ellipsis in "Downloading Blunt Code…" decoded as mojibake; the message is ASCII-only now, and `package.ps1` prints the real installer filename (`install-latest.ps1`) instead of a name that never existed.
- **README execution-policy fallback** points at `install-latest.ps1` (the actual release asset) instead of an `Install-BluntCode.ps1` file that was never shipped anywhere.

## [0.6.0] - 2026-08-26

### Added

- **Global findings search:** `GET /api/v1/findings/search` queries every stored scan at once (text across message/rule/path, severity lists, analyzer, workspace scoping, suppression-aware) and a new **Search** page renders ranked results with links back to each originating report.
- **Workspace risk score:** `GET /workspaces/{id}/risk` weighs the latest completed scan (critical ×10, high ×5, medium ×2, low ×1) into an A–D grade with a trend delta against the previous scan; the workspace dashboard shows it as a card.
- **Scan deletion & retention:** `DELETE /scans/{id}` removes one terminal scan with full cascade (409 while still running), `DELETE /workspaces/{id}/scans?keep=N` prunes beyond the newest N, and a **Prune history** control on the workspace page drives it.
- **Workspace tags:** freeform lowercase labels per workspace (`GET/PUT /workspaces/{id}/tags`, migration 006) embedded in workspace JSON.
- **CLI output files:** `--format csv` (UTF-8 BOM, formula-neutralizing, identical to the API export), `--output FILE` for any document format, and `--save-baseline FILE` to capture the SARIF baseline in one step.
- **Scoped CI gates:** `--gate-analyzer semgrep,secrets` and `--gate-category security` narrow which findings feed `--fail-on`/`--max-findings`, with a stderr scope report.
- **Watch tuning:** `--watch-poll` (250ms–1m) and `--watch-quiet` (100ms–2m) override the watch loop timing with sanity checks.
- **`bluntcode help`:** stdout usage for every subcommand.
- **GitHub annotation cap:** `--github-cap 1..50` overrides how many annotations show per severity before the truncation notice (default stays 10).
- **Richer secret detection:** six new detectors — Stripe live keys, OpenAI project and classic keys, Anthropic keys, Slack app-level tokens, Azure storage account keys — with placeholder rejection and entropy floors.
- **TODO attribution:** `TODO(jane):` and `FIXME(APPS-123)` markers now count as debt and carry the owner/ticket through the finding message.

### Changed

- **Semgrep rulepack v3.0.0** grows to 25 bundled rules: `os.popen`, `tempfile.mktemp` races, `random` used for secrets, `new Function` execution, and `postMessage("*")` origin leaks. The version bump invalidates incremental-reuse caches exactly like an analyzer upgrade.
- **Shareable report URLs:** the report view syncs filters/sort/page to the address bar and hydrates from it on load plus back/forward navigation.
- **Server-paged history:** `GET /workspaces/{id}/scans?page&page_size` returns bounded windows with totals; the history page pages server-side instead of loading every scan.
- **Resilient UI plumbing:** `useLoad` ignores stale responses after dependency changes; live scan streams reconnect with exponential backoff (1s→15s) and surface the retry count.

### Fixed

- Workspace sort controls expose `aria-sort`/`aria-pressed` state, matching the report table's accessibility.
- The finding preview dialog gains a **Copy fingerprint** action for suppression tooling and bug reports.

## [0.5.0] - 2026-08-25

### Added

- **Workspace context sidebar:** the workspace detail page gains a sticky left rail (Overview / Files & rules / Scan history) with `aria-current` section tracking, collapsing under the content on small screens. The same rail now also appears on the Files and Scan history pages so workspace sub-navigation stays persistent across all three views.
- **First-run dashboard hero:** with zero workspaces and zero scan history the dashboard swaps its stats/activity sections for a single onboarding panel pointing at "Add your first workspace"; any saved workspace or past scan keeps the full dashboard.
- **Command palette (Ctrl/Cmd+K):** a type-to-filter launcher for navigation and actions — go to Home/Workspaces/Tools/Settings/About, add a workspace, toggle theme, open shortcuts help. Works from anywhere, including inside text fields and over other dialogs; combobox/listbox semantics with arrow navigation, Enter to run, Escape to close; ranked matching puts label prefixes first. Documented in the shortcuts dialog. The palette mounts only while open, so keyboard focus moves into its input on open and restores to the trigger on close.
- **Live scan progress:** running scans show a determinate progress bar once the analyzer count is known (finished ÷ total) with an indeterminate sweep before that and success/danger tone on completion; the elapsed clock ticks every second while a scan runs; live per-analyzer pills in the results panel flip from stream events without waiting for a reload.
- **Findings scannability:** the findings toolbar sticks beneath the app nav while scrolling long reports, and critical/high/medium finding rows get severity-tinted leading edges (low/info rows stay clean).
- **Toast actions and lifetime bar:** notices can carry an inline action button ("View scan"-style) that runs its callback and dismisses; a thin bar mirrors each toast's remaining auto-dismiss time and pauses with it on hover/focus.
- **History date bands and expandable rows:** scan history groups rows under Today / Yesterday / This week / Earlier band headers (missing or invalid timestamps land in Earlier), and every row has an `aria-expanded` disclosure revealing absolute start/finish times, profile, any error summary as a warning block, and per-analyzer status pills. Row disclosure buttons carry distinct accessible names including the scan's relative time. Bands are computed per page of server-paginated results, so a band header can repeat on the next page when one day spans pages.
- **Sortable workspaces grid:** Name / Last scan / Findings sort client-side via sortable-header buttons with direction arrows; never-scanned workspaces always sink to the end.
- **Tools readiness strip:** a "X of Y ready" counter with a chip per not-ready tool sits above the tools table; chips show spinner plus operation copy ("installing…", "repairing…", "updating…") for whichever action is in flight, and rows gained version badges.
- **About page at-a-glance card:** identity, inline version badge, privacy bullets with line icons, server details, and a Copy version info button with transient inline confirmation.
- **Contextual 404:** unknown paths shaped like `/workspaces/<id>` or `/scans/<id>` explain that the content may have been removed alongside the usual Home/Workspaces links.
- **Trend chart tooltips and axes:** severity trend bars are keyboard-focusable and reveal a tooltip (date, total, severity breakdown) on focus and hover, sit on a labeled baseline with first/last date ticks, and the chart carries a trend-direction summary for screen readers. Native SVG titles were removed so hover shows exactly one tooltip.
- **Suppressions search:** the workspace suppressions panel filters by reason or fingerprint fragment with debounced input, a live result count, and a no-match state distinct from the nothing-suppressed state.
- **Files page helpers:** the workspace root path renders as a breadcrumb chip under the heading, a loaded-path counter reflects what search actually covers, and Collapse-all folds every open folder without discarding already-fetched children.

### Changed

- **Settings toggles are real switches:** the two general settings use `role="switch"` controls with visible On/Off state instead of text buttons whose labels doubled as state.
- **Motion polish:** page changes fade-and-rise via a remount-scoped animation, dialogs scale in from 97% with the backdrop fading, and the nav underline lands with spring easing; everything is disabled under `prefers-reduced-motion`.
- **Stacked-dialog Escape ownership:** when one dialog is opened over another (e.g. the command palette over shortcuts help), Escape closes only the top layer — the ownership rule matches the existing focus-trap behavior.
- **Quieter assistive tech:** the files tree loaded-count no longer announces through a live region on every folder expansion (search-match counts keep theirs); stats overview numbers render in tabular numerals with screen-reader-only card descriptions and a stable skeleton-swap height.

## [0.4.0] - 2026-08-24

### Added

- **Inline `bluntcode:ignore` comments for the built-in analyzers:** false positives from the secrets and TODO trackers can now be dismissed at the source, reviewable in git. A line comment containing the case-sensitive token `bluntcode:ignore` (any comment leader — `#`, `//`, `<!-- -->`, `/* */`, `--`, `;`) suppresses same-line findings or findings on the immediately following line; it can target a single rule (`bluntcode:ignore secrets.aws-access-key-id`) or act bare, and trailing free text after `reason:` is allowed for annotation. Parsing is fail-safe — a typo'd rule id matches nothing, keeping the finding visible rather than silently suppressed — and the JSON envelope carries parsed directives rather than raw line text so secrets never leak through their own ignore comments. External tools keep their own mechanisms (`# noqa`, `// biome-ignore`, `# nosemgrep`), and the directive never affects another analyzer's findings.
- **User documentation for the 0.3.x wave:** three new guides under docs/, cross-linked from the README's new Documentation section. `docs/ci.md` covers the headless CI contract — exit codes, the `--fail-on` grammar, baselines with the SARIF round-trip (including a PowerShell 5.1 UTF-16 redirection caveat), every output format including GitHub annotation behavior and caps, `--jobs`, `--incremental`, and a complete GitHub Actions workflow mirroring the repo's own ci.yml. `docs/ignoring-findings.md` explains when to use each of the three ignore mechanisms (inline directive, workspace suppressions with restore, external-tool pragmas) plus `.bluntcodeignore` pattern forms versus per-machine exclude rules. `docs/configuration.md` documents the LOCALAPPDATA data layout, reading `bluntcode config`, the SonarQube startup timeout variable, offline-mode semantics (including that the built-in analyzers are held back offline), and what each profile runs.
- **Broad language coverage for the built-in analyzers:** discovery now classifies forty-plus file types across code (Go, Java, Kotlin, C#, C/C++, Ruby, PHP, Rust, Swift, Scala, Objective-C, Vue, Svelte), web/data (CSS, SCSS, HTML, JSON, YAML, TOML, XML, SQL, GraphQL), shell (sh, PowerShell, batch), and config/credential files (Markdown, INI, properties, `.env` including `.env.local`-style variants, Dockerfile, PEM/key blobs). The secrets detector receives every classified language — its findings finally cover `.env` files, Dockerfiles, and YAML, the places credentials actually live — and the TODO tracker covers the code and config languages where comments exist, with documented exclusions (JSON has no comments; PEM/`.env` are secrets territory). External tools are untouched: ruff, Biome, and semgrep still receive exactly their own languages, so existing fingerprints and findings stay stable. `ScanRequest.Languages` now enumerates every classified language instead of the original hard-coded trio.
- **Incremental rescans:** `bluntcode scan --incremental` — and watch mode automatically, from its second scan onward — re-runs analyzers only on files that changed since the workspace's last completed scan. Selected files are content-hashed (SHA-256) at scan time and hashes are persisted per scan (migration 005), with reuse invalidated whenever the analyzer set, an analyzer version, the profile, or the Blunt Code version changes. Findings for unchanged files are copied into the new scan with their fingerprints intact, so suppression and the new/fixed/persistent comparison keep working, and a scan note records what happened (`incremental: reused findings for N unchanged file(s), ran analyzers on M file(s)`). An equivalence test pins the core invariant: an incremental scan over an unchanged workspace produces the same fingerprints and totals as a full scan. Any failure to read hashes silently degrades to a full scan.
- **Analyzer and rule filters with real pagination in the findings UI:** the scan report view now offers a rule `<select>` populated from the report's own data (scoped to the selected analyzer, with an "All rules" default and a removable chip) that filters server-side via the `rule=` param, and the findings table switched from the legacy limit/offset window to proper `page`/`page_size` paging at a fixed 50-row window — Previous/Next with boundary disables, a "Showing X–Y of Z / Page N of M" status announced politely, `has_next` treated as authoritative, and any filter or sort change resetting to page 1. The CSV export mirrors the active filters without being clipped to the visible page.
- **`bluntcode config`:** a read-only effective-configuration report for troubleshooting — version, every resolved path (data dir, database, tools, reports, logs, temp) with exists/missing markers, the `BLUNTCODE_SONAR_STARTUP_TIMEOUT` override in effect (with an invalid-value note matching the adapter's acceptance rules), persisted app settings when the database is readable (or a clear "app is running" note when it isn't — the command never takes the data-dir lock and never creates anything), and the installed version per managed tool. `--json` emits a stable fixed-key shape; unknown arguments are usage errors (exit 2).
- **SARIF export from the CLI:** `bluntcode scan --format sarif` writes the exact SARIF 2.1.0 document the API's report download serves, straight to stdout — byte-for-byte the same serialization (pinned by a test against the route's encoder invocation). This unlocks the CI baseline round-trip without a server (`bluntcode scan --format sarif > baseline.sarif`, then `--baseline baseline.sarif` on later runs) and feeds GitHub code-scanning uploads directly. In `--watch` mode each rescan emits one complete newline-separated SARIF document, mirroring `--format json`.
- **Dashboard overview cards:** the home page now leads with a global overview fed by `/api/v1/stats` — Workspaces, Scans (with completed/running split and a live pulse when scans are in flight), Findings with a severity mini-bar and counted legend (latest-completed-per-workspace semantics, so the number matches what the dashboard shows), Suppressions, and Tools readiness (omitted when unwired). Loads with the standard skeleton, degrades to a quiet inline retry on error without blocking the rest of the dashboard, and renders zeros on a fresh install.
- **GitHub Actions CI:** the repository now runs `.github/workflows/ci.yml` on pushes to main and pull requests — `go vet`, `go build`, the full Go test suite, the web test suite and build, then a self-scan dogfood gate: the freshly built exe scans `web/` with the quick profile under `--fail-on high+`, using a workspace-isolated `LOCALAPPDATA` so CI state never leaks, with the managed analyzer tools cached by manifest hash and the generated markdown report uploaded as an artifact when the gate trips.

### Changed

- **Dogfood self-scan cleanup:** pointing the broadened built-in analyzers at this repository's own source surfaced 305 findings (34 high) — all intentional (detector fixtures, prose mentions of the tracked markers, documentation examples). Fixture files and feature-documentation prose are now excluded via a committed `.bluntcodeignore`; the remaining self-referential lines carry inline `bluntcode:ignore` directives (including invisible HTML comments in the markdown docs). Self-scan reports 0 high findings (158 low-baseline), the CI gate (`--fail-on high+`) passes, and an incremental rescan of the unchanged workspace completes in 92 ms.

## [0.3.0] - 2026-08-24

### Added

- **Watch mode:** `bluntcode scan --watch` keeps the command alive — it scans once immediately, then polls the workspace every 2 seconds and rescans automatically once changes settle (a 1.5-second quiet window debounces save bursts; a change during a running scan coalesces into exactly one queued rescan). Each rescan prints a short `rescan: N file(s) changed` header on stderr, so `--format json` stdout stays pure newline-separated documents; scan failures and gate failures print per scan but never stop the loop, and Ctrl+C keeps its existing graceful/forced (exit 130) semantics. `--watch` composes with the gate, baseline, and jobs flags and is rejected with `--format github` (annotations make no sense in a loop).
- **Built-in TODO/FIXME comment tracker:** a new `todo` analyzer joins the built-in set (no tool to download; standard and deep profiles). It flags whole-word, case-sensitive `TODO`, `FIXME`, `HACK`, `XXX`, and `BUG` markers (`FIXME`/`BUG` at medium severity, the rest low, category maintainability) with the trailing comment text as the message — rune-safe truncation at 200 characters, control bytes scrubbed. Word-boundary and follower heuristics keep `TODOS`, `BUGFIX`, `TODO_LIST`, and lowercase prose from firing, and it shares the built-in hygiene guards (1 MiB/file read cap, binary skip, per-file and per-run finding caps with truncation notes).
- **Global stats endpoint:** `GET /api/v1/stats` gives dashboards a one-call overview — workspace count, scan totals split completed/running, a findings severity rollup computed with the dashboard's own semantics (latest completed scan per workspace, not an all-history sum), suppression count, managed-tool readiness, and an RFC3339 timestamp. It is two aggregate SQL queries with no N+1, severity counts are a fixed struct (never a map), and an empty database returns all zeros rather than nulls.
- **GitHub Actions annotations export:** `bluntcode scan --format github` emits the workflow-command protocol on stdout, so scans running inside GitHub Actions produce inline annotations on files and PRs. Severity maps to annotation level (critical/high → `error`, medium → `warning`, low/info → `notice`), escaping follows the actions/toolkit rules (percent-encoding for `,`/`:`/`%` and newlines in properties, control bytes scrubbed before encoding, forward-slash paths), and the stream respects GitHub's 10-annotations-per-type-per-step cap — overflowing types are truncated severity-first with a final notice carrying the full counts. Annotations go to stdout while gate/baseline summaries stay on stderr, so the format composes cleanly with `--fail-on` in CI.
- **`bluntcode doctor --fix`:** doctor can now repair the mechanical problems it finds instead of just reporting them: stale or missing semgrep local rules are re-extracted from the bundled pack (with version verification identical to the tools service's), interrupted-install leftovers are cleaned up (`.new` staging files, partial downloads, abandoned SonarQube scanner scratch dirs, stale `.previous` backups only when the final version exists), and missing data-directory subdirectories are recreated with correct permissions. Two new report-only checks list orphaned tool version directories (manual cleanup — deleting is unsafe without an install journal) and summarize applied repairs. Repairs are idempotent, and the fix pass acquires the same data-dir lock as the app — if Blunt Code is running, every repair is skipped with a loud notice instead of corrupting state. Plain `doctor` and `doctor --json` output is byte-identical to before; `--fix --json` adds per-check repair fields.
- **Richer findings filtering, sorting, and pagination on the API:** `GET /api/v1/scans/{id}/findings` gains `rule` (exact, case-insensitive), `path_prefix` (anchored, slash/case-normalized, LIKE-wildcard-safe), comma-list values for the existing `severity` and `status` params, an extended sort whitelist (`path|line|severity|rule|analyzer|status` with a `-` prefix for descending and severity sorted in domain order, not alphabetical), and `page`/`page_size` pagination (1..200) with `page`/`page_size`/`has_next` metadata. The findings.csv export accepts the same filters. With none of the new params, requests compile to the same SQL and return a byte-equivalent envelope to before — the existing UI contract is pinned by a regression test.
- **Built-in committed-secrets detector:** a new `secrets` analyzer joins ruff/biome/semgrep/sonarqube that runs entirely in-process — nothing to download, so it participates in every standard or deep scan (the quick profile stays ruff+biome). It detects AWS access key ids, GitHub/Slack/Google tokens, private-key blocks, JWTs, and credential-bearing connection URIs (severity high, category security), plus generic high-entropy assignments to `password`/`secret`/`token`/`api_key`-style keys (severity medium) with placeholder and entropy filtering to stay precise. Messages never echo the secret (first 4 characters plus length only). Hygiene guards: at most 1 MiB read per file, binary files skipped, 50 findings per file and 5,000 per scan with explicit truncation notes. It receives every language the discovery layer classifies today (Python, JS/TS family).
- **Interactive standalone HTML report:** the one-file HTML export now groups findings into collapsible per-file sections and adds a client-side filter toolbar — severity chips with live counts, an analyzer dropdown built from the runs in the report, a debounced search over rule/message/path, a "showing X of Y" counter, and clear-filters. The whole thing stays self-contained (≈4 KB inline vanilla JS, zero external resources, works offline), printing shows all findings with the filter UI hidden, output remains byte-stable for identical input, and every dynamic string is HTML-escaped by the Go renderer before embedding — hostile-corpus tests prove `<script>`/quote/control-byte payloads in finding text cannot execute.
- **Suppression management in the web UI:** the fingerprint-based suppression workflow (API routes shipped earlier) is now fully usable from the browser. Findings rows offer "Suppress…" behind an accessible dialog (optional reason with a 500-character limit and live counter, focus trap, Esc to close, helper copy explaining that suppression hides the finding from future scans, reports, and the CI gate), suppressed findings offer "Restore", and the status filter gained a "Suppressed" option. The workspace detail page gains a Suppressions panel listing every dismissal with its reason, date, and a Restore action — with skeleton loading, empty state, and quiet inline errors that never block the page. Thirteen new frontend tests (165 total, all passing).
- **Severity trends over scan history (API + UI):** `GET /api/v1/workspaces/{id}/trends?limit=N` (default 20, max 100, validated like the other limit params) returns the workspace's completed scans oldest→newest, each point carrying the persisted per-severity counts (critical…info), total, profile, state, and `finished_at` (falling back to `started_at`). The workspace detail page renders these as a dependency-free, hand-rolled SVG stacked bar chart — fixed severity stacking order, native tooltips per bar with date and per-severity counts, a screen-reader figure description plus summarizing `aria-label`, zero-finding scans kept visible as baseline markers, and colors derived from the existing severity tokens so light and dark themes both work. Loading uses the existing skeleton pattern, errors are quiet inline messages that never block the page, and the section stays hidden until the first completed scan exists.
- **Bounded analyzer parallelism (`--jobs`):** `bluntcode scan --jobs <N>` (positive integer; invalid values are usage errors, exit 2) runs at most N analyzer pipelines concurrently via a semaphore. Analyzers previously executed strictly sequentially in registry order; the default remains exactly that sequential model (event ordering, cancellation checkpoints, and panic handling preserved verbatim), while `--jobs 2` on a four-analyzer setup overlaps tool runtimes — meaningful because SonarQube's long compute-engine waits currently serialize behind ruff/biome/semgrep. Under a bound, queued analyzers abort promptly on scan cancellation, in-flight runs keep their per-analyzer timeout contexts, and a panicking worker fails the scan instead of crashing it; tests verify the concurrency cap, identical persisted runs/findings versus the default path, and cancellation while queued.
- **Fingerprint-based finding suppression ("ignore this finding forever"):** findings can be dismissed as acknowledged/wontfix by fingerprint, per workspace, via three new API routes — `POST /api/v1/workspaces/{id}/suppressions` (body `{"fingerprint","reason"}`; fingerprint is the finding's 64-character sha256 hex, reason optional up to 500 characters), `DELETE /api/v1/workspaces/{id}/suppressions/{fingerprint}` (404 when the fingerprint was never suppressed), and `GET /api/v1/workspaces/{id}/suppressions` for the UI. Suppressions are keyed by workspace + finding fingerprint (the same identity scans compare across runs) in a new `suppressed_findings` table and apply from the next scan — suppressing mid-scan is safe. Future scans still store matching findings, but they are excluded from the scan record's `total_findings` and severity counts, from every report and export (Markdown, HTML, SARIF, CSV, JSON — filtered at model-build time so all renderers inherit it), from the new/fixed/persistent comparison (a dismissed fingerprint is never falsely reported as fixed), and from the `--fail-on`/`--max-findings` CI gate. The findings list keeps suppressed rows visible with a new `suppressed` status (selectable via `?status=suppressed`), and removing a suppression restores normal counting on the next scan.
- **Per-project `.bluntcodeignore` file:** workspaces can commit a `.bluntcodeignore` at their root and every scan (CLI or web UI) plus the file-tree browser honors it — no re-entering excludes in the UI on each machine. The format is gitignore-inspired and deliberately small: one pattern per line, `#` comments and blank lines ignored, LF or CRLF accepted, backslashes normalized to `/`, capped at 1,000 patterns / 64 KiB, and `!` negation lines are skipped (counted for a log line). Patterns merge additively with the workspace's saved exclude rules using the exact same matcher (basename, `dir/**` prefix, `**/name` suffix, case-insensitive), the merged list is recorded in the scan snapshot's exclusions, and a broken or oversized ignore file logs-and-continues — it can never fail a scan.
- **JSON report export:** a machine-readable, versioned report document (`"schema": "bluntcode/scan-report"`, schemaVersion 1) joins Markdown/HTML/SARIF/CSV. It carries workspace and scan metadata (id, profile, state, timestamps), file counts (candidate/selected/skipped), severity totals, per-analyzer run summaries (state, findings, duration, error), the full findings array (every exported field including fingerprint and comparison-derived new/persistent status), analyzer metrics, and the new/fixed/persistent comparison. Output is byte-stable for identical input (findings sorted by path/line/rule, struct-based severity counts, lists never `null`), 2-space indented with LF endings, and scrubbed of control bytes like the other formats. Surfaces as `bluntcode scan --format json` (full document on stdout; the existing `--json` summary flag is unchanged) and as a `GET /api/v1/scans/{id}/findings.json` download that mirrors the findings.csv route.
- **React auto-domain for Biome:** React workspaces now get Biome's React domain rules without any setup. Detection runs at plan time: `react`/`react-dom` in the root (or `packages/*` / `apps/*`) package.json dependencies; failing that, an import scan of the first 50 selected `.jsx`/`.tsx` files for `react`/`react-dom`/`next`; failing that, `.jsx`/`.tsx` presence when no package.json exists at all. When detected (and the workspace does not ship its own `biome.json`/`biome.jsonc`), the adapter injects a config enabling `linter.domains.react: "recommended"` for the pinned Biome 2.5.6, written once to a content-hashed stable path. Non-React workspaces produce byte-identical command lines as before, and any config-write failure degrades to the previous config-less run. Verified end-to-end: `useHookAtTopLevel` and `useExhaustiveDependencies` now fire on React code that previously passed silently (this repo's own `web/` included).
- **Baseline diff mode for CI gates:** `bluntcode scan --baseline <scan-id-or-path>` accepts either a previous scan ID from the same workspace or a path to a SARIF 2.1.0 file and loads its finding fingerprints as the baseline. With gate flags active (`--fail-on`, `--max-findings`), only findings whose fingerprints are **not** in the baseline count toward the gate, so teams can adopt the CI gate without drowning in pre-existing debt; a one-line stderr summary reports how many known findings were excluded and how many are new. The SARIF export now embeds each finding's fingerprint as a `partialFingerprints` entry (`bluntcode/v1` key) and a new reader makes Blunt Code SARIF files round-trip as baselines; foreign SARIF logs without fingerprints still load via deterministic sorted-key fallbacks. Unknown scan IDs and unreadable or invalid SARIF paths are usage errors (exit 2) before the scan starts.
- **Expanded bundled semgrep rulepack (2 → 20 rules):** the local rulepack maintained beside the semgrep adapter now covers Python and JS/TS security: command injection (`subprocess` with `shell=True`, `os.system`, `child_process.exec` on dynamic input, `process.env` fed to exec), unsafe deserialization (`pickle.loads`, `yaml.load` without a safe loader), SQL built by f-string/format/`%`/concatenation/template literals, hardcoded secrets, weak hashes (MD5/SHA-1), disabled TLS verification (`verify=False`), Flask `debug=True`, unfiltered `tarfile.extractall`, dynamic `eval`/`exec`/`new Function`, React `dangerouslySetInnerHTML` with dynamic HTML, and `javascript:` URLs. Injection rules require non-literal involvement to stay precise, all rule ids are namespaced and stable (fingerprints depend on them), and tests validate the pack's schema, uniqueness, LF endings, and a minimum rule count so it cannot silently shrink. The tools installer now extracts this adapter-maintained pack as the single source of truth (rules version bumped to 2.0.0 so existing installations refresh).
- **Configurable, adaptive SonarQube startup wait:** the `BLUNTCODE_SONAR_STARTUP_TIMEOUT` environment variable (a Go duration like `45s` or `5m`; unparseable, zero, or negative values warn and fall back) now overrides the default 10-minute budget for waiting on the managed SonarQube server. The health poll starts immediately (a server that is already UP returns on the first check), polls every second, and logs progress on status transitions and every ~20 seconds with elapsed time, remaining budget, and the last observed status, so a cold boot no longer looks like a hang. Context cancellation aborts the wait promptly instead of burning the remaining budget.
- **CI severity gate for headless scans:** `bluntcode scan` gained `--fail-on <severities>` (comma-separated, case-insensitive, with `high+`-style "and above" shorthand) and `--max-findings <N>` flags that turn a completed scan into a build gate. When either flag is given, the scan exits 1 — with a one-line `fail:` explanation on stderr — if any unresolved finding sits at or above the fail-on threshold or the total exceeds the maximum; otherwise it exits 0. Gate counts exclude resolved (fixed) findings, scans that fail, cancel, or time out keep their existing exit codes (1, or 130 after Ctrl+C), and invalid gate values are usage errors (exit 2) before any scan starts. Without the flags the exit-code contract is unchanged.

### Fixed

- **Hardening batch from the 2026-08-24 QA review:** graceful scan shutdown on Ctrl+C (running analyzers are cancelled instead of being abandoned mid-write), event-bus history pruning so long-lived servers no longer grow per-scan history without bound, Biome source-position lookup capped at 2 MB files, ruff exit codes derived from merged findings instead of always reporting failure, a nil-scan-started guard in report writing, verified-download temp files cleaned up on checksum mismatch, and a fingerprint edge case for root-relative paths.
- **API validation and security headers:** workspace-rule patterns now reject directory traversal, absolute paths, and drive letters; duplicate workspace creation returns the existing workspace instead of a 500; CSV export surfaces write errors; scans list and previous-scan lookups order deterministically (`finished_at, id`) so pagination no longer shuffles; the API sends `X-Content-Type-Options`, `X-Frame-Options: DENY`, `Referrer-Policy`, and a `Content-Security-Policy`; the file-tree endpoint refuses symlinked directories that resolve outside the workspace; SARIF artifact URIs drop parent-traversal paths; user exclude patterns match case-insensitively and support the `**/name` basename form.
- **Per-language file routing for analyzers:** scan orchestration now hands every analyzer only the selected files whose language it actually supports (ruff receives `.py`/`.pyi`, Biome receives `.js`/`.jsx`/`.mjs`/`.cjs`/`.ts`/`.tsx`/`.mts`/`.cts`). Previously a mixed-language workspace passed the entire file list to every analyzer, which dogfooding on this repository exposed dramatically: ruff parsed a committed minified JavaScript bundle as Python and failed the scan after generating 8 MB of syntax-error JSON. The ruff and Biome adapters additionally filter defensively, so no caller can hand them files they cannot lint, and an empty post-filter selection skips the analyzer instead of invoking a bare `ruff check` that would scan its working directory.
- **SonarQube findings were silently dropped:** the compute-engine task id was parsed only from a debug-only `ceTaskUrl=` scanner line, so the normal INFO output matched nothing, the wait-for-processing step was skipped, and issues were fetched before the server had processed the report — every scan reported zero SonarQube findings. The parser now also reads the default scanner output (verified end-to-end: 440 findings on a 692-file project that previously showed 0). Related repairs: the managed-server startup window grew from 3 to 10 minutes because cold boots on consumer hardware legitimately exceed 3 minutes, the issues query now walks all result pages instead of silently capping at 500, and component paths are stripped of their `<project-key>:` prefix so reports show clean workspace-relative paths.
- **Ruff file batching for large workspaces:** scanning a 692-file Python project failed with `fork/exec ... The filename or extension is too long` because the ruff adapter passed every file in a single process invocation, overflowing Windows' 32,767-character command-line limit. Ruff now splits file arguments into batches (like Biome and semgrep always did) and merges the per-batch JSON outputs into one result.
- **Frontend lint debt cleared by dogfooding:** a self-scan surfaced 79 findings in non-test UI source, all fixed: explicit `type` on every action button (43), honest `useEffect`/`useCallback` dependency lists in the scan stream, theme, file-tree, and history hooks (7, including a restructured OS-theme listener and a page-reset trigger keyed to the newest scan), stable React list keys that no longer derive from array indices (skeleton placeholders, file-rule rows, scan event stream), and accessibility repairs (labelled SVG toast icons, a semantic list for language badges, a real `fieldset` for severity filters, list-based filter chips, `aria-hidden` removed from the focusable `/` search hint, non-null assertions replaced with real guards in App routing and the workspace detail view).

## [0.2.1] - 2026-08-22

A batch of 10 QA loops: fuzzing, hostile-payload round-trips, a live-browser
audit, a security pass, lifecycle/concurrency stress tests, and scale
benchmarks. Every fix below ships with a regression test.

### Security

- **CSV formula injection (High):** findings text is untrusted (it derives from scanned code). Cells starting with `=`, `+`, `-`, `@`, tab, or CR are now prefixed with `'` in `findings.csv`, neutralizing spreadsheet formula execution on export-open.
- **SARIF control-byte leak:** DEL (0x7f) was written raw and terminal escape sequences could smuggle into rule ids, messages, and help URIs; all analyzer-derived SARIF strings now pass the same control scrubber as the other formats.
- **Markdown and terminal hardening:** C0/DEL control characters are stripped from report tables and from CLI stdout/stderr output (ANSI poisoning, forged log lines).

### Fixed

- Eight list endpoints returned `null` instead of `[]` when empty (scans, findings, rules, path-overrides, fixed, report findings/metrics/warnings) — fixed at the repository source.
- Frontend crashes on hostile data: invalid date strings crashed every date-rendering page (RangeError); dashboard summaries rendered `NaN`; findings pagination showed "0–NaN of undefined"; `null` tool/tree/rules bodies threw; report payloads without a `scan` object crashed the report view.
- Scan lifecycle: an analyzer panic killed the whole app instead of failing the scan; a database failure during completion left the workspace permanently 409-locked; cancelling during the final analyzer reported `completed` instead of `cancelled`; orphaned Windows process trees could hang a scan forever (now killed with `taskkill /T` plus a wait deadline); concurrent tool installs raced on file locks; cancelling one scan killed the shared SonarQube server out from under another.
- CLI: flags now parse in any position relative to the path argument; the second Ctrl+C always exits with cleanup; NTFS junction and case-variant paths no longer create duplicate workspace rows (fixed at the normalization root for both the CLI and the API); the instance-lock failure path no longer leaks the log-file handle.
- Live-browser audit: wide tables no longer pan the whole page sideways on mobile; the skip-to-content link now actually moves focus; four color tokens were below WCAG AA contrast and were adjusted in both themes.

### Performance

- New `findings(scan_id, fingerprint)` index: the findings page at 50k findings went from 215ms to 2ms; the CSV export of 52k rows went from an effective hang to 868ms; the fixed-findings listing at 20k rows went from 9.1s to 125ms.
- Workspace lists no longer run a full-history query per workspace (51 queries → 2 at 50 workspaces).
- Frontend: `Intl.DateTimeFormat` instances hoisted to module level (~45× faster date rendering); hostile finding messages and long paths are line-clamped instead of stretching the table.

## [0.2.0] - 2026-08-22

A batch of 25 improvement loops focused on automation, exports, and UI polish.

### Added

- **Headless CLI scans:** `bluntcode scan <path>` runs one full scan without the browser or server, with `--profile quick|standard|deep`, `--json` (machine-readable summary for CI), `--timeout`, and `--quiet` flags. Progress streams to stderr and the summary prints to stdout. Exit codes are CI-friendly: `0` when the scan completes (warnings included), `1` when it fails, is cancelled, or times out, `2` for usage errors, and `130` after Ctrl+C.
- **SARIF export:** `GET /api/v1/scans/{id}/report.sarif` serves a SARIF 2.1.0 log with deduplicated rules, severity-to-level mapping, and workspace-relative artifact locations — compatible with VS Code SARIF viewers and GitHub code scanning.
- **HTML report export:** `GET /api/v1/scans/{id}/report.html` serves a standalone, self-contained HTML document (summary strip, analyzer and findings tables, print styles, no external assets).
- **CSV export:** `GET /api/v1/scans/{id}/findings.csv` exports findings honoring the current filters (category, path, query, severity, status), with a UTF-8 BOM so Excel renders it correctly.
- **Global scans feed:** `GET /api/v1/scans` returns recent scans across all workspaces plus a dashboard summary (per-workspace severity totals from each workspace's latest completed scan, scan counts, and active scan count).
- **Fixed-findings listing:** `GET /api/v1/scans/{id}/fixed` lists findings fixed since the previous completed scan, coverage-aware (only analyzers that succeeded in both scans are compared).
- **Open-folder endpoint:** `POST /api/v1/system/open-folder` opens Blunt Code's own `data`/`reports`/`logs`/`tools` folders in Windows Explorer. The request carries a fixed enum — never a caller-supplied path — and the server launches `explorer.exe` directly with no shell.
- **Dark mode:** theme toggle with OS-preference sync, explicit choice locking, and a no-flash bootstrap script (persisted in `localStorage` under `bluntcode-theme`).
- **Home dashboard:** activity summary cards (active scans, critical+high findings, totals, weekly scans), a recent-scans feed with state/profile/severity dots, and a quick-scan action.
- **Report visualizations:** stacked severity distribution bar with legend and per-analyzer severity mini-bars sorted by count; print styles hide app chrome, keep severity colors, and pin the light palette when printing from dark mode.
- **"What changed" panel:** lists findings fixed since the previous scan (top 10 with severity, rule, and location) once a scan reaches a terminal state.
- **Export menu:** Markdown / HTML / SARIF / CSV from the report view, with CSV honoring the active filters and sort.
- **Scan history upgrades:** per-scan severity mini-bars with tooltip breakdown, profile badges, warning/danger hints on the newest row, and per-scan Markdown export.
- **Keyboard shortcuts:** `g` then `h`/`w`/`t`/`s` navigation sequences, `n` for add workspace, `/` to focus search, `?` for a help dialog; suppressed while typing or when a dialog is open.
- **File tree search:** recursive search across loaded folders with auto-expansion, match counts, substring highlighting, per-node loading spinners, retryable load failures, and `/` to focus.
- **Toast notifications:** stacked toasts (bottom-right, max 4, auto-dismiss with pause on hover/focus, error codes as badges) replacing the single-slot notice banner.
- **Loading skeletons:** layout-aware table/cards/lines skeletons with screen-reader labels, plus Installing/Repairing/Updating busy feedback on tool rows.
- **Empty states & all-clear:** icon medallions on every empty state and a celebratory all-clear panel for completed zero-finding scans.
- **UI robustness & accessibility:** error boundary with crash details and route-keyed recovery, a real 404 page, About page in nav, skip-to-content link, dialog focus traps with `aria-modal` and backdrop close, relative timestamps with absolute tooltips, copy finding/location to clipboard, arrow-key row navigation, and `scope=col` table headers.
- **Settings:** buttons to open Blunt Code's data, reports, logs, and tools folders in Explorer.

### Changed

- **Deeper deep profile:** the deep scan profile now runs Ruff with an extended rule selection (`--select=E,W,F,B,SIM,C4,RET,ARG,PLR`), covering style, bugbear, simplification, comprehension, return, argument, and refactor rules. Quick and standard profiles are unchanged. Diagnostic logs now record the executed analyzer command line.
- **Shared findings filter pipeline:** the JSON and CSV findings endpoints are served by one query builder, so filters behave identically across both.
- **Frontend modularized:** the `App.tsx` monolith was split into `pages/`, `components/`, `hooks/`, and `lib/` modules; the web test suite grew from 16 to 124 cases.
- **Findings table:** columns (severity, file, tool, status) are now sortable server-side with `aria-sort`, severity pills are clickable filters, text filters debounce (300 ms) with removable filter chips.

### Fixed

- Unknown URLs now render a friendly 404 page instead of silently showing Home.
- Printing a report from dark mode no longer produces a dark-themed printout; the light palette is pinned for print.
- Theme no longer flashes on load: an inline bootstrap script applies the stored theme before first paint.
