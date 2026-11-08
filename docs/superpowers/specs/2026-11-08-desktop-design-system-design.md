# Desktop design system — shadcn/Radix consolidation

| Field | Value |
| ----- | ----- |
| **Status** | Approved for documentation; **implementation plan written** (see plans doc); code not started |
| **Author** | Sanket Patel |
| **Date** | 2026-11-08 |
| **Base revision** | `main` @ `5f40c333e64699a499dfed9d8c62223a7a23067a` (release **v0.49.1**) |
| **User approval** | Consistent shadcn/Radix desktop redesign; preserve coral/teal and server pagination |
| **Document type** | Design specification only — **no product code changes are implied by this file** |

This specification defines how Blunt Code’s **desktop web UI** will be brought onto a single, accessible component system built from **existing** Radix primitives and the hand-maintained `web/src/components/ui/*` layer (shadcn-style patterns). It does **not** describe work already shipped and does **not** claim any visual redesign has occurred.

---

## 1. Purpose and scope

### 1.1 Purpose

Reduce visual and interaction inconsistency across core desktop surfaces by **finishing** the current Radix/shadcn stack—not by adding parallel UI libraries. Operators should get:

- Predictable **table, filter, and pagination chrome** on data-heavy pages.
- **One primary action** per page header where possible; fewer competing hero cards and duplicate explanatory copy.
- Shared **loading, empty, error, and retry** treatment.
- Keyboard-safe **dialogs, menus, and focus** behavior (aligned with existing a11y tests).
- **Coral/teal** brand direction with **severity and risk semantics** that do not rely on color alone.

### 1.2 In scope (pages)

| Route / page | Primary file(s) |
| ------------ | ----------------- |
| Overview (home) | `web/src/pages/HomePage.tsx` |
| Workspaces list | `web/src/pages/WorkspacesPage.tsx` |
| Scan history (global + workspace embed) | `web/src/pages/HistoryPage.tsx`, embed in `WorkspaceDetailPage.tsx` |
| Findings search | `web/src/pages/SearchPage.tsx` |
| Analyzers / tools | `web/src/pages/ToolsPage.tsx` |
| Settings | `web/src/pages/SettingsPage.tsx` |
| Shared chrome | `AppShell`, `DesignTopbar`, nav rail, `CommandPalette` |

**Report, scan, workspace detail, and pentest** pages adopt shared primitives only where they already import `ui/*` or duplicate table/pagination patterns; they are not the first migration wave unless a change is required for shared component APIs.

### 1.3 Out of scope

- **Backend or API** changes (pagination parameters, sort keys, filters, `/api/v1/stats`, demo seed).
- **TanStack Table**, MUI, Chakra, Ant Design, or other new table/UI frameworks.
- **Tablet-first** or mobile redesign; responsive rules may remain but **acceptance viewports are desktop widths** (see §8).
- **Native Windows** managed-analyzer execution or real scan validation (see §12).
- Version bumps, tags, releases, or changelog entries as part of design work.

---

## 2. Baseline stack (do not duplicate dependencies)

The following are **already in production** (`web/package.json` @ v0.49.1). Implementation must **reuse** them; adding overlapping libraries is forbidden unless this spec is amended.

| Layer | Technology |
| ----- | ---------- |
| Runtime | React **19.1.1**, Vite **7**, TypeScript **5.8** |
| Styling | Tailwind **3.4** + `web/src/tokens.css` + `web/src/styles.css` |
| Primitives | Radix (accordion, dialog, dropdown, popover, select, tabs, tooltip) |
| UI kit | `web/src/components/ui/*` (Button, Card, Input, Table, Dialog, Select, Tabs, Skeleton, Badge, Combobox, …) |
| Utilities | `class-variance-authority`, `clsx`, `tailwind-merge` (`lib/utils.ts`) |
| Icons | `lucide-react` |
| Charts | `recharts` (lazy route chunks — preserve chunk boundaries) |
| Motion | `motion` + Kokonut copy-ins (`switch-button`, `split-flap-text`) — **scope reduced**, not expanded |
| Tables in app | **Semantic HTML tables** and lists with server-driven page indices — **not** a headless table library |

There is **no** `components.json` today; the kit is maintained in-repo. Implementation may add a manifest for documentation purposes but must not fork styling away from `tokens.css`.

---

## 3. Design principles

1. **Desktop-first density**: ~25 data rows visible on a **1440×900** viewport for primary lists (Workspaces, History, ledger) without vertical page scroll for the table body alone.
2. **Server truth**: Lists display **exactly** the items returned for the current `page` / `page_size`; no client-side truncation (e.g. no `slice(0, 12)` on paged API results).
3. **Fleet vs ledger**: Overview **hero / verdict / stats** use **aggregated** `GET /api/v1/stats` (and related globals), not a single page of workspaces. Ledger pagination is independent and labeled.
4. **Semantic color**: Coral = primary action / accent / focus; teal = success, fixed, low-risk, “good news.” Severity colors (`--color-sev-*`) are **only** for finding/scan severity—not decorative washes.
5. **Reduced motion**: Respect `prefers-reduced-motion: reduce` and existing `animations.css` killswitch; decorative motion (SplitFlap, chart entry) must not block comprehension.
6. **Incremental migration**: Each page lands with tests green; legacy CSS removed in the same PR when a rule has zero references.

---

## 4. Visual system — tokens and semantics

### 4.1 Authoritative tokens

**Source of truth:** `web/src/tokens.css` (“Playful instrument — Coral + teal”).

| Role | Token(s) | Usage |
| ---- | -------- | ----- |
| Primary accent | `--color-accent`, `--color-accent-strong`, focus ring `--color-focus` | Primary buttons, active nav, focus outlines, pressed states |
| Success / teal | `--color-success`, `--color-success-text`, `--color-success-soft` | Passed scans, low severity emphasis in *positive* context, “fixed” affordances |
| Ink / surfaces | `--color-ink*`, `--color-surface*`, `--color-paper` | Typography and cards |
| Danger / warning | `--color-danger*`, `--color-warning*` | Failed scans, destructive actions—not primary marketing coral |
| Severity | `--color-sev-critical` … `--color-sev-low` | Finding counts, pills, table cells; always paired with **text labels** (existing a11y pattern) |

Tailwind theme in `web/tailwind.config.js` maps `accent`, `destructive`, `ring`, etc., to these variables. New UI must use **token-backed** classes, not hard-coded hex.

### 4.2 Non-color cues

Retain and extend existing patterns (documented in `docs/accessibility.md`):

- Severity dots with `aria-hidden` + visible severity name.
- Grade tiles with letter + state class, not color alone.
- `prefers-contrast: more` adjustments where already defined in CSS.

### 4.3 Typography and spacing

- UI body: **14px** (`--text-sm` / Tailwind `text-sm`).
- Page titles: **~24–32px** display (`--font-display`).
- Table numerics: **tabular-nums**.
- Spacing: use `--space-*` tokens; shared table padding target **`--table-pad: 14px`** (already on `ui/table.tsx` wrapper).

---

## 5. Reusable component boundaries

Implementation introduces **no new npm UI packages**. It **does** add or harden **application-level** compositions under `web/src/components/` (names are normative for implementation planning).

### 5.1 Core primitives (existing — extend in place)

| Component | Path | Responsibility |
| --------- | ---- | -------------- |
| Button | `ui/button.tsx` | Variants: default (primary), outline, secondary, ghost, destructive; sizes `sm` / default; `disabled` + `aria-disabled` for pagination |
| Input | `ui/input.tsx` | Text and search fields; error state via `aria-invalid` |
| Select / Combobox | `ui/select.tsx`, `ui/combobox.tsx` | Filter dropdowns with keyboard typeahead |
| Table suite | `ui/table.tsx` | Scroll shell, sticky header option, row hover, optional `data-[cv=auto]` for content-visibility |
| Card | `ui/card.tsx` | Panel shells; error/empty cards use existing `ErrorPanel` + `Empty` patterns in `ui.tsx` |
| Dialog / Sheet | `ui/dialog.tsx`, `ui/sheet.tsx` | Modal tasks; must use `useDialogA11y` / Radix focus trap |
| DropdownMenu | `ui/dropdown-menu.tsx` | Row actions, overflow menus |
| Skeleton | `ui/skeleton.tsx`, `skeletons.tsx` | Page-level and table skeletons |
| Pagination | **New:** `components/Pagination.tsx` (or `ui/pagination.tsx`) | Previous/Next + optional page indicator; **single implementation** shared by all server-paged lists |

### 5.2 Application compositions (new or consolidated)

| Component | Responsibility | Must not |
| --------- | -------------- | -------- |
| `DataPanel` | `board-panel`-equivalent: header (title + optional actions), body slot, footer slot | Fetch data |
| `DataTableShell` | Wraps `Table` + horizontal fade edges (`.table-wrap` behavior), optional sticky first column | Implement sorting without API params |
| `FilterBar` | Horizontal cluster: `Input`, `Select`, date filter, clear control | Replace server-side filter semantics |
| `PageHeader` | Title, one-line description, **one primary** CTA + secondary actions | Duplicate command palette trigger (rail owns Ctrl+K) |
| `ListFooter` | Composes `Pagination` + `Showing X–Y of N` with tabular-nums | Compute totals client-side from partial arrays |

### 5.3 Legacy CSS retirement

When a page migrates to the compositions above, delete the corresponding **page-scoped** blocks in `styles.css` (e.g. `.page-workspaces .ws-table …`) if and only if:

1. No other selector references the rule (repo-wide search).
2. Visual regression tests and contrast audit still pass.

Legacy class names **may remain temporarily** on DOM nodes as migration hooks (`board-ledger`, `ledger-row`) until tests are updated; see §11.

### 5.4 Kokonut / motion scope

| Asset | Current use | After redesign |
| ----- | ----------- | -------------- |
| `switch-button` | Theme toggle in shell | Keep; ensure focus ring uses `--color-focus` |
| `split-flap-text` | Grade tile animation | Keep **only** when `prefers-reduced-motion: no-preference`; static letter otherwise |
| `motion` elsewhere | Charts, micro-interactions | No new full-page motion; prefer CSS transitions under 200ms |

---

## 6. Page specifications

### 6.1 Overview (`HomePage`)

**Goals:** Clear fleet story (stats-driven), ledger as **paged workspace assessments**, activity feed separate.

| Area | Behavior (unchanged data) | UI target |
| ---- | ------------------------- | --------- |
| Hero / verdict | Uses global stats + worst-workspace highlight fetch (`page_size: 1`, sort findings desc) | `PageHeader` + single primary CTA to scan or workspaces; reduce duplicate stat cards if two panels show the same total |
| Metrics strip | Stats overview component | `Card` grid or compact metric row; teal only for positive deltas |
| Ledger | `api.workspacesPage({ page, page_size: 25, sort, order })` | `section.board-ledger` **retained** or `DataPanel` with same `aria-labelledby`; rows as `ol.ledger-list` > `li.ledger-row` **or** table rows with **preserved** `data-workspace-id` |
| Ledger footer | Range text + Prev/Next | `ListFooter` + shared `Pagination`; `has_next` and `total` from API; `LEDGER_PAGE_SIZE = 25` constant |
| Activity | Separate panel with its own paging if present | Footer controls **scoped** inside `.board-activity` only (Playwright strict-scope contract) |
| Loading | Partial board load | `Loading` / `SkeletonTable` inside panels, not full-page blank |
| Error | Stats OK but workspaces fail | Existing copy; `ErrorPanel` with retry on ledger block only |
| Empty fleet | No workspaces | `Empty` with add-workspace action |

**Forbidden regressions:**

- Rendering fewer than `items.length` workspace rows when the API returned them.
- Using workspace page 1 only to compute fleet-wide verdict.

### 6.2 Workspaces (`WorkspacesPage`)

| Area | API | UI target |
| ---- | --- | --------- |
| Toolbar | `search`, `tag`, sort query params | `FilterBar` with `Input` + tag `Select` |
| Table | `workspacesPage` with server sort | `DataTableShell`; sticky first column (name); row navigates to workspace (keyboard: focusable row or link in first cell) |
| Pagination | Server `page`, `page_size`, `has_next`, `total` | Same `Pagination` + range label as ledger |
| States | — | `SkeletonTable` (loading), `ErrorPanel` (retry), `Empty` (no workspaces / no filter matches) |

### 6.3 History (`HistoryPage` + workspace embed)

| Area | API | UI target |
| ---- | --- | --------- |
| Date filter | Client sends date filter param already supported by API | `Input type="date"` styled via tokens; `data-empty` dashed border when unset |
| Table | Paged scans | Shared table chrome; sticky header; state column uses visible text labels |
| Pagination | Server-driven | Shared `Pagination` |
| States | — | Same loading/error/empty trio |

**Note:** History state strings may continue to use raw API state text until a follow-up i18n pass; do not block redesign on labeling parity with `scanStateDisplay`.

### 6.4 Findings search (`SearchPage`)

| Area | API | UI target |
| ---- | --- | --------- |
| Query builder | `api.searchFindings` with `page`, `page_size` (25 default in tests) | `FilterBar` + existing query builder; severity/workspace filters via `Select`/`Combobox` |
| Results | Server-paged full result set; sort params unchanged | Table view and list view share `DataTableShell` / row components; **no client-side cap** on hits beyond current page |
| Pagination | `has_next`, `total` | Shared footer |
| States | Zero hits vs error vs loading | Distinct `Empty` titles; `ErrorPanel` with retry |

### 6.5 Tools (`ToolsPage`)

| Area | UI target |
| ---- | --------- |
| Analyzer catalog | `Card` grid or `DataTableShell` for dense mode; version cells monospace truncated |
| Install actions | Single primary per row or dropdown; `Button` variants |
| States | `SkeletonCards` / `SkeletonTable`, `Empty`, `ErrorPanel` |

### 6.6 Settings (`SettingsPage`)

| Area | UI target |
| ---- | --------- |
| Sections | `Tabs` or `Card` sections with `Separator` |
| Controls | `Switch` pattern via existing components; `Input` for paths/tokens |
| Danger zone | `destructive` button variant only here or in explicit confirm dialogs |

### 6.7 Global chrome

| Element | Rule |
| ------- | ---- |
| `DesignTopbar` | Breadcrumb only; **no** second command-palette button |
| Nav rail | Primary navigation; Ctrl+K palette unchanged |
| Theme toggle | Kokonut switch acceptable; focus visible |
| Toasts | Existing live regions; no redesign required beyond token alignment |

---

## 7. Interaction, accessibility, and keyboard

### 7.1 Requirements

All migrated surfaces must preserve or improve evidence listed in `docs/accessibility.md`:

- Skip link → `#main-content`.
- Dialogs: `role="dialog"`, `aria-modal`, label, focus trap, Escape, focus restore (`dialogs.test.tsx`).
- Command palette: single entry; shortcuts suspended while modal open (`App.shortcuts.test.tsx`).
- Pagination: focusable controls, disabled state not in tab order incorrectly (`aria-disabled` on buttons).
- Tables: sort controls (where present) are buttons with accessible name; row selection keys unchanged on report/search where already tested.

### 7.2 Focus and keyboard patterns

| Pattern | Spec |
| ------- | ---- |
| Focus ring | 2px `outline` / `ring` using `--color-focus`; offset per existing page rules |
| Dropdown menus | Radix `DropdownMenu`; arrow keys inside menu |
| Date filter | Native date input; calendar picker indicator opacity pattern from history CSS migrated to token classes |
| Reduced motion | `motion-reduce:` Tailwind variants on new animations |

### 7.3 Live regions

- Loading: `role="status"` + `aria-busy` on page skeleton (`Loading` in `ui.tsx`).
- Errors: `role="alert"` on `ErrorPanel`.
- Pagination changes: optional polite `aria-live` on footer range (if added, test in Vitest).

---

## 8. Layout — desktop viewports

Acceptance layout review uses **width × height**:

| Viewport | Intent |
| -------- | ------ |
| **1280×720** | Minimum supported desktop; horizontal scroll inside `DataTableShell` allowed; sticky first column required on Workspaces |
| **1440×900** | Primary design target; ~25 table rows visible without clipping pagination footer |
| **1920×1080** | Extra horizontal space; content max-width may grow but **no centered narrow column** on data pages |

**Non-goal:** Re-layout for `< 1000px` beyond not breaking existing collapse rules.

Grid / hero on Overview may use two columns at 1440+; ledger and activity stack or sit side-by-side per existing `board-*` layout, with **one** visual hierarchy: verdict → metrics → ledger → activity.

---

## 9. Data and error flows (frozen contracts)

### 9.1 API surfaces (unchanged)

| Client helper | Endpoint semantics |
| ------------- | ------------------ |
| `api.workspacesPage` | `page`, `page_size`, `sort`, `order`, `search`, `tag` |
| `api.stats` / global stats loader | Fleet aggregates for Overview |
| `api.searchFindings` | Full-text search paging, facets |
| Workspace history paging | `page`, `page_size` capped per API |
| Findings report paging | Server `page` / `page_size`; no client merge of pages |

### 9.2 Overview ledger constants

| Constant | Value |
| -------- | ----- |
| `LEDGER_PAGE_SIZE` | **25** |
| `LEDGER_STORAGE_KEY` | Persists current page in `localStorage`; clamp page when `total` shrinks |

### 9.3 Error handling

| Layer | Behavior |
| ----- | -------- |
| `useLoad` / fetch | Surfaces `CODE: message` strings |
| `ErrorPanel` | Friendly text for known codes; **no retry** on `*_NOT_FOUND` |
| Partial failure | Overview: stats can succeed while ledger fails — show inline error without falsifying fleet counts |

### 9.4 DOM contracts for automation

Unless explicitly versioned in the same release as test updates:

| Selector / attribute | Purpose |
| -------------------- | ------- |
| `section.board-ledger` | Playwright scope for ledger pagination (`web/e2e/synthetic-board-verify.mjs`) |
| `section.board-activity` | Separate scope for activity Next control |
| `li.ledger-row[data-workspace-id]` | Ledger row identity in Vitest (`HomePage.test.tsx`) |
| `data-workspace-id` on row | Must match API `workspace.id` for every rendered ledger row |

If ledger markup moves to `<table>`, tests must assert the same **per-row workspace id** attribute on the clickable row element.

---

## 10. Incremental migration strategy

### 10.1 Incremental migration units

Work lands in **small, mergeable PRs** to `main`. Each unit below lists its **exit criterion**; **sequence is not fixed in this spec** and will be chosen during a separate implementation planning step after spec approval.

| Unit | Deliverable | Exit criterion |
| ---- | ----------- | -------------- |
| Shared chrome | `Pagination` + `ListFooter` + Vitest coverage for disabled/next/prev/range text | Component used by at least one existing page without behavior change |
| Overview ledger | Ledger panel + footer on shared chrome | `HomePage` pagination tests and synthetic board verify pass |
| Workspaces + History | Tables on `DataTableShell` | Server sort/filter unchanged; sticky first column preserved |
| Search + Tools + Settings | Filters and footers aligned to shared chrome | Search paging tests and contrast audit pass |
| CSS retirement | Remove migrated `styles.css` page blocks | No orphaned selectors; visual checks at 1280/1440/1920 where harness runs |

### 10.2 Parallel styling rules

During migration, **do not** duplicate button/table styles in new CSS. Prefer:

1. Tailwind utilities on `ui/*` components.
2. Token variables for one-off layout.
3. Delete legacy `.button`, `.table-wrap` duplicates when call sites are gone.

### 10.3 Copy and card reduction

When two panels repeat the same fleet total or scan count, **merge or demote** the secondary panel to a single line in the primary panel header. Do not remove information required by tests or by `stats` API fields.

---

## 11. Testing, regression control, and evidence

### 11.1 Automated (required on each implementation PR)

| Command | Purpose |
| ------- | ------- |
| `cd web && npm test` | Vitest unit/integration (583+ tests baseline at v0.49.1) |
| `npm run audit:contrast` | Token pair contrast in light/dark |
| `npm run verify:synthetic-board` | Ledger vs activity pagination scopes (Playwright + synthetic API) |
| `node e2e/visual-scale.mjs` | 1280 / 1440 / 1920 viewport captures when harness available |

**Targeted suites:** `HomePage.test.tsx` (ledger pages, 25 rows, `data-workspace-id` uniqueness across pages), `SearchPage.test.tsx` (`page_size=25`, `has_next`), `App.test.tsx` / navigation / keyboard journey, `dialogs.test.tsx`, `CommandPalette.test.tsx`.

### 11.2 Regression risks called out

| Risk | Mitigation |
| ---- | ---------- |
| Client slice of paged data | Code review + test asserting row count === `items.length` |
| Fleet stats from page 1 only | Tests stub `/stats` separately from workspaces list |
| Playwright strict mode collision | Pagination buttons live only inside named `section` containers |
| Sticky column hover mismatch | Visual check at 1280 with horizontal scroll |
| Bundle size | No new table library; monitor Vite chunk sizes for `recharts`/`motion` |

### 11.3 Visual evidence policy

| Environment | Claim |
| ----------- | ----- |
| Synthetic API + SQLite large dataset + Playwright | **Allowed** for screenshots in PR evidence; label **“synthetic UI/API”** |
| Native Windows managed tools / real Sonar scans | **Explicitly unverified** in Linux CI/agent environments; not a gate for UI redesign PRs |

---

## 12. Acceptance criteria (design complete → implementation ready)

Implementation may begin only after this spec is merged or approved via review. An implementation series is **done** when **all** of the following hold:

1. **Shared components:** `Pagination` + `ListFooter` used on Overview ledger, Workspaces, History, and Search.
2. **Server pagination:** Default page size **25** where the product already uses 25; footer shows correct **X–Y of N** and respects `has_next`.
3. **Overview:** Fleet hero uses **global stats**; ledger shows up to **25** rows per page with stable `data-workspace-id`; activity pagination is DOM-isolated from ledger.
4. **Tokens:** New UI uses coral/teal and severity tokens per §4; contrast audit passes.
5. **States:** Every in-scope page uses shared loading, empty, error, and retry patterns from `ui.tsx` / `ui/*`.
6. **A11y:** Existing automated a11y tests pass without weakening assertions; dialogs/menus retain keyboard models.
7. **Motion:** SplitFlap and decorative animations honor reduced motion.
8. **Dependencies:** No new UI framework packages; Radix/Tailwind/Recharts/lucide/motion remain the stack.
9. **CSS:** Page-specific blocks removed for migrated pages (measurable diff in `styles.css`).
10. **Evidence:** Vitest green; synthetic board verify green when run; viewport captures at 1280/1440/1920 where harness is available.

---

## 13. Document history

| Date | Change |
| ---- | ------ |
| 2026-11-08 | Initial approved design spec (Approach A). No implementation. |

---

## 14. Related documents

- [`docs/superpowers/plans/2026-11-08-desktop-design-system.md`](../plans/2026-11-08-desktop-design-system.md) — implementation plan (tasks, gates, tests)
- `docs/accessibility.md` — conformance evidence inventory
- `AGENTS.md` — graphify / release rules (implementation phase)
- `CONTRIBUTING.md` — demo seed and synthetic harness notes
