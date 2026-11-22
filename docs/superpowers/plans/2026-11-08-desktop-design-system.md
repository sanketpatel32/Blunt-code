# Implementation plan — desktop design system (shadcn/Radix)

| Field | Value |
| ----- | ----- |
| **Status** | Implementation plan (approved design); **no product code in this document** |
| **Author** | Sanket Patel |
| **Date** | 2026-11-08 |
| **Design spec** | [`docs/superpowers/specs/2026-11-08-desktop-design-system-design.md`](../specs/2026-11-08-desktop-design-system-design.md) @ `022b600e9c79d7ac74b3ec11f79a67935771866c` |
| **Code base** | `main` @ `5f40c333e64699a499dfed9d8c62223a7a23067a` (v0.49.1) at plan authorship |
| **Execution** | Staged PRs to `main` from implementation branches; **parent decides when to start** |

---

## Goal

Finish the **existing** Radix + `web/src/components/ui/*` layer into one coherent **desktop-first** UI across Overview, Workspaces, History, Findings search, Tools, and Settings: shared list chrome, consistent states, coral/teal tokens, and **unchanged server APIs** (fleet stats, ledger **25** rows per page, search/history paging). **No new UI npm packages.** **No backend changes.**

---

## Architecture

```
tokens.css ──► tailwind.config.js ──► ui/* (primitives)
                      │
components/ui.tsx (Loading, Empty, ErrorPanel)
                      │
        ┌─────────────┼─────────────┬──────────────┐
        ▼             ▼             ▼              ▼
  Pagination    ListFooter    DataTableShell   FilterBar
  (+ tests)     (+ tests)     (+ tests)        (+ tests)
        │             │             │              │
        └─────────────┴─────────────┴──────────────┘
                              │
              DataPanel (optional shell; keeps board-* hooks)
                              │
        ┌─────────────────────┼─────────────────────┐
        ▼                     ▼                     ▼
   HomePage.tsx        WorkspacesPage.tsx    HistoryPage.tsx
   SearchPage.tsx       ToolsPage.tsx         SettingsPage.tsx
```

**Data flow (frozen):** Pages call `web/src/api.ts` helpers with `page`, `page_size`, `sort`, `order`, filters. Components **never** slice `items` for display. Overview hero reads **`api.stats`** (or equivalent loader), not ledger page 1.

**Automation hooks (preserve unless same PR updates tests + e2e):**

| Hook | Consumer |
| ---- | -------- |
| `section.board-ledger` | `web/e2e/synthetic-board-verify.mjs` |
| `section.board-activity` | Same (strict-mode isolation) |
| `li.ledger-row[data-workspace-id]` | `web/src/pages/HomePage.test.tsx` |
| Ledger footer regex `Showing {start}–{end} of {total} by risk` | Synthetic board + Vitest |

---

## Stack (reuse only)

| Layer | Path / package |
| ----- | -------------- |
| Runtime | React 19.1.1, Vite 7, TypeScript 5.8 (`web/package.json`) |
| Tokens | `web/src/tokens.css` |
| Legacy layout CSS | `web/src/styles.css` (delete page blocks as pages migrate) |
| Primitives | `web/src/components/ui/*.tsx`, `web/src/lib/utils.ts` (`cn`) |
| State patterns | `web/src/components/ui.tsx` (`Loading`, `ErrorPanel`, `Empty`) |
| Skeletons | `web/src/components/skeletons.tsx`, `web/src/components/ui/skeleton.tsx` |
| Page title chrome | `web/src/components/PageHeader.tsx` (already exists — extend usage, do not fork) |
| Icons | `lucide-react` |
| Charts / motion | `recharts`, `motion`, Kokonut copy-ins under `web/src/components/kokonutui/` |

**Forbidden:** `@tanstack/react-table`, MUI, Chakra, Ant Design, duplicate pagination implementations per page.

---

## Global constraints (copied from design spec)

1. Desktop acceptance widths: **1280×720**, **1440×900**, **1920×1080** (data pages stay full-width, not a narrow centered column).
2. Server pagination default **25** where the product already uses 25 (`LEDGER_PAGE_SIZE`, `WS_PAGE_SIZE`, Search `page_size`).
3. Fleet vs ledger: global stats drive Overview verdict; ledger is independently paged.
4. Coral = accent/focus; teal = success/low-risk; `--color-sev-*` only for severity (always with text labels).
5. `prefers-reduced-motion`: no mandatory motion for comprehension; `motion-reduce:` on new transitions; GradeTile flip respects existing reduced-motion behavior in `split-flap-text.tsx`.
6. Each mergeable PR: Vitest green, contrast audit green, no unrelated refactors.
7. **No** version bump / tag / release until the full series completes and a human runs the ship rule.

---

## File responsibility map

| File | Responsibility after plan |
| ---- | ------------------------- |
| `web/src/components/Pagination.tsx` | Previous/Next; `hasPrev`/`hasNext` or `page`/`pageCount`; `aria-disabled` on disabled buttons; optional `navLabel` |
| `web/src/components/Pagination.test.tsx` | Keyboard/a11y/disabled contract |
| `web/src/components/ListFooter.tsx` | Range line + `Pagination` + optional trailing actions slot |
| `web/src/components/ListFooter.test.tsx` | Range formatting, live region optional |
| `web/src/components/DataTableShell.tsx` | Wraps `ui/table` scroll shell + right-edge fade (ports `.table-wrap` behavior) |
| `web/src/components/DataTableShell.test.tsx` | Renders children; `stickyFirstColumn` class hook |
| `web/src/components/FilterBar.tsx` | Layout for filters + meta status line (`role="status"`) |
| `web/src/components/FilterBar.test.tsx` | Clear button, Escape on inputs |
| `web/src/components/DataPanel.tsx` | `header` / `children` / `footer` slots; maps to `board-panel` classes when `variant="board"` |
| `web/src/pages/HomePage.tsx` | Ledger footer → `ListFooter`; keep `board-ledger`, `ledger-row`, `data-workspace-id` |
| `web/src/pages/WorkspacesPage.tsx` | Toolbar → `FilterBar`; table → `DataTableShell`; footer → `ListFooter` |
| `web/src/pages/HistoryPage.tsx` | Date filter styling; pagination → `ListFooter`; replace legacy `.button secondary` nav |
| `web/src/pages/SearchPage.tsx` | Findings footer nav → `ListFooter` (`has_next` authoritative) |
| `web/src/pages/ToolsPage.tsx` | Table shell + `Button` variants only |
| `web/src/pages/SettingsPage.tsx` | `Tabs`/`Card`/`Separator` alignment |
| `web/src/components/GradeTile.tsx` | Verify `flip` gated on reduced motion (no new motion deps) |
| `web/src/styles.css` | Remove `Page · Workspaces`, `Scan history`, `Search`, `Tools` blocks when unused (grep-verified) |

---

## Public interfaces (TypeScript)

Implement exactly these exports (adjust only with spec amendment + test updates).

```ts
// web/src/components/Pagination.tsx
export type PaginationProps = {
  /** Accessible name for the <nav>, e.g. "Workspace assessments pagination" */
  'aria-label': string;
  onPrevious: () => void;
  onNext: () => void;
  hasPrevious: boolean;
  hasNext: boolean;
  busy?: boolean;
  /** When set, renders "Page {current} of {total}" between buttons (activity/ledger may hide) */
  currentPage?: number;
  pageCount?: number;
  className?: string;
  size?: 'sm' | 'default';
  variant?: 'outline' | 'ghost'; // match Home ledger (ghost) vs Workspaces (outline)
};

// web/src/components/ListFooter.tsx
export type ListFooterProps = {
  rangeText: React.ReactNode;
  'aria-label'?: string;
  pagination?: PaginationProps | null;
  actions?: React.ReactNode;
  className?: string;
  /** When true, adds polite aria-live on range (Search already uses <output>) */
  liveRange?: boolean;
};

// web/src/components/DataTableShell.tsx
export type DataTableShellProps = {
  children: React.ReactNode;
  className?: string;
  stickyFirstColumn?: boolean; // applies ws-table sticky rules via data attribute + token classes
  minWidth?: string; // default min-w for horizontal scroll, e.g. "56rem" workspaces
};

// web/src/components/FilterBar.tsx
export type FilterBarProps = {
  filters: React.ReactNode;
  meta?: React.ReactNode; // count line, role="status"
  onClear?: () => void;
  clearLabel?: string;
  className?: string;
};

// web/src/components/DataPanel.tsx
export type DataPanelProps = {
  id?: string;
  titleId: string;
  title: React.ReactNode;
  headerActions?: React.ReactNode;
  footer?: React.ReactNode;
  className?: string;
  panelClassName?: string; // e.g. "board-panel board-ledger"
  children: React.ReactNode;
};
```

---

## Verification matrix

| Command | Where | Expected success | Role |
| ------- | ----- | ---------------- | ---- |
| `cd web && npm test` | Any OS | `Tests … passed` (count ≥ 583 at series start) | **Gate** every PR |
| `cd web && npm run build` | Any OS | `tsc` + Vite build exit 0 | **Gate** every PR |
| `cd web && npm run audit:contrast` | Any OS | Script exit 0, no pair below 4.5:1 | **Gate** every PR |
| `cd web && npx vitest run src/components/Pagination.test.tsx src/components/ListFooter.test.tsx` | Any OS | All listed files pass | Task 1 |
| `cd web && npx vitest run src/pages/HomePage.test.tsx -t "pages the ledger"` | Any OS | 1 passed | Task 2 |
| `cd web && npm run verify:synthetic-board` | Host with **running** `bluntcode --no-browser` + `BLUNTCODE_DEMO_SEED=1`, isolated DB | Assertions pass; PNGs unless `SKIP_SHOTS=1` | Evidence PR (Task 2+) |
| `cd web && BASE=… node e2e/visual-scale.mjs` | Same server contract | PNGs under `/opt/cursor/artifacts/bluntcode-scale/` | Optional evidence |
| `powershell -File scripts\verify.ps1` | **Windows** supported toolchain | Full ship gate | **Pre-release only** (not per UI task) |

**Harness truth (do not overstate):**

- **Vitest + build + contrast** validate UI logic, API contracts, and pagination DOM **without** a live server on Linux.
- **Playwright scripts** (`synthetic-board-verify.mjs`, `visual-scale.mjs`) need a loopback Blunt Code API with demo seed and SQLite `large` preset; they exercise **synthetic** data only.
- **Linux agents** may be unable to **start** the full desktop binary if managed-analyzer bootstrap fails; that limits **live** Playwright, not unit tests. When the server **does** start (typical on Windows, or Linux with isolated `LOCALAPPDATA` if supported in your env), run the same scripts — do **not** claim “all UI work requires Windows.”
- **Native Windows analyzer scans** remain **unverified** in Linux CI and are **not** a gate for this redesign series.

---

## Task sequence and acceptance gates

Execute in order. Each task ends with **commit + push**; run the **task gate** commands before the next task.

| Gate | After task | Criteria |
| ---- | ----------- | -------- |
| **G1** | Task 1 | New component tests pass; full `npm test` + `audit:contrast` |
| **G2** | Task 2 | `HomePage.test.tsx` ledger tests pass; e2e ledger regex unchanged |
| **G3** | Task 3 | `WorkspacesPage.test.tsx` + History tests pass |
| **G4** | Task 4 | `SearchPage.test.tsx` paging tests pass |
| **G5** | Task 5 | Tools/Settings smoke paths pass; no new deps in `package.json` |
| **G6** | Task 6 | `styles.css` shrinks; grep shows no dead selectors; full test + contrast |
| **G7** | Series | Design spec §12 checklist satisfied; optional Playwright evidence attached |

---

### Task 1 — Shared `Pagination` + `ListFooter` (TDD)

**Depends on:** nothing.

**RED — create failing tests**

1. Add `web/src/components/Pagination.test.tsx`:
   - Renders `<nav aria-label="Test pagination">` with buttons named **Previous** and **Next**.
   - When `hasPrevious: false`, Previous has `disabled` and `aria-disabled="true"`.
   - Clicking Next calls `onNext` once when `hasNext: true`.
2. Add `web/src/components/ListFooter.test.tsx`:
   - Renders `rangeText` and passes pagination props through to `Pagination`.
   - `tabular-nums` class on footer wrapper.

Run:

```bash
cd web && npx vitest run src/components/Pagination.test.tsx src/components/ListFooter.test.tsx
```

**Expected before GREEN:** failures `Cannot find module './Pagination'` or assertion failures.

**GREEN — implement**

1. Create `web/src/components/Pagination.tsx` using `Button` from `ui/button.tsx` (`variant` prop per interface).
2. Create `web/src/components/ListFooter.tsx` with default footer layout matching `board-panel-foot flex-wrap gap-2` spacing (Tailwind + tokens).
3. Import new components by path from pages (there is no `web/src/components/index.ts`; primitives export via `web/src/components/ui/index.ts` only).

**REFACTOR:** None beyond ensuring no duplicate button markup.

**Gate G1:**

```bash
cd web && npx vitest run src/components/Pagination.test.tsx src/components/ListFooter.test.tsx
cd web && npm test && npm run audit:contrast
```

**Commit message:** `feat(web): add shared Pagination and ListFooter components`

---

### Task 2 — Overview ledger footer (behavior-neutral swap)

**Depends on:** Task 1.

**RED**

1. In `Pagination.test.tsx`, add case: `variant="ghost"` applies ghost buttons (class or variant prop).
2. Re-run `HomePage.test.tsx` ledger tests — they must still pass **before** edits:

```bash
cd web && npx vitest run src/pages/HomePage.test.tsx -t "pages the ledger"
cd web && npx vitest run src/pages/HomePage.test.tsx -t "last partial ledger"
```

**GREEN — edit `HomePage.tsx`**

1. Replace footer block at `board-panel-foot` (lines ~597–632) with:

```tsx
<ListFooter
  rangeText={/* preserve exact strings: "Showing {start}–{end} of {total} by risk" */}
  pagination={{
    'aria-label': 'Workspace assessments pagination',
    hasPrevious: ledgerHasPrev,
    hasNext: ledgerHasNext,
    onPrevious: () => setLedgerPage((p) => Math.max(1, p - 1)),
    onNext: () => setLedgerPage((p) => p + 1),
    busy: workspaces.loading,
    currentPage: ledgerPage,
    pageCount: ledgerMaxPage,
    variant: 'ghost',
  }}
  actions={/* existing "All workspaces" Button */}
/>
```

2. Keep `section.board-ledger`, `ol.ledger-list`, `li.ledger-row`, `data-workspace-id` untouched.
3. Do **not** change `LEDGER_PAGE_SIZE`, `api.workspacesPage` args, or stats loaders.

**REFACTOR:** Remove duplicate `tabular-nums` spans if fully covered by `ListFooter`.

**Gate G2:**

```bash
cd web && npx vitest run src/pages/HomePage.test.tsx
cd web && npm test && npm run build
```

Optional (if server running):

```bash
cd web && BASE=http://127.0.0.1:9876 npm run verify:synthetic-board
```

Expect: `page 1: want 25 ledger rows`, no strict-mode collision; `Showing 1–25 of … by risk`.

**Commit message:** `refactor(web): use ListFooter on Overview ledger`

---

### Task 3 — Workspaces + History

**Depends on:** Task 1.

**RED**

```bash
cd web && npx vitest run src/pages/WorkspacesPage.test.tsx
cd web && npx vitest run src/pages/HistoryPage.test.tsx
```

Record pass baseline.

**GREEN — `WorkspacesPage.tsx`**

1. Wrap toolbar `toolbar-row ws-toolbar` filters in `<FilterBar filters={…} meta={countLine} onClear={…} />` using existing `Input` components (keep `aria-label`s).
2. Replace `<div className="table-wrap ws-table-wrap">` with `<DataTableShell className="ws-table-wrap" stickyFirstColumn minWidth="56rem">` wrapping the same `<table className="table-dense ws-table rtable">` (keep sort headers and `WorkspaceRow`).
3. Replace footer `Previous`/`Next` block (~166–174) with `ListFooter` + `Pagination` (`variant="outline"`, `hasNext: page < pageCount`, `hasPrevious: page > 1`).

**GREEN — `HistoryPage.tsx`**

1. Replace `history-pagination` nav buttons (legacy `button secondary` + inline SVG) with `ListFooter` + `Pagination`; preserve `aria-label="Scan history pagination"` and **Showing X to Y of Z** text in `rangeText`.
2. Migrate date input classes: add Tailwind/token classes mirroring `.history-filter-input` / `[data-empty]`; do not change filter API params.

**REFACTOR — CSS (same PR)**

1. `rg 'ws-table-wrap|history-pagination' web/src` — must only reference migrated markup or shell classes.
2. Delete obsolete rules in `styles.css` sections `Page · Workspaces list` and `Page · Scan history` **only** for selectors with zero matches.

**Gate G3:**

```bash
cd web && npx vitest run src/pages/WorkspacesPage.test.tsx src/pages/HistoryPage.test.tsx
cd web && npm test && npm run audit:contrast
```

**Commit message:** `refactor(web): shared list chrome on Workspaces and History`

---

### Task 4 — Findings search

**Depends on:** Task 1.

**RED:**

```bash
cd web && npx vitest run src/pages/SearchPage.test.tsx -t "page_size"
```

Tests already assert `page_size=25` and `has_next` behavior.

**GREEN — `SearchPage.tsx`**

1. Replace `<nav className="findings-pagination" …>` block (~1049–1075) with `ListFooter` where:
   - `pagination.hasNext` derives from `state.data?.has_next === true` (not legacy flags).
   - `pagination.hasPrevious` from `page > 1`.
   - Preserve `<output aria-live="polite">` via `liveRange` or embedded in `rangeText` if range stays in toolbar only (keep **single** total display per existing comment).
2. Wrap results `<table>` container with `DataTableShell` where `table-wrap` is used today (table + list modes).

**Gate G4:**

```bash
cd web && npx vitest run src/pages/SearchPage.test.tsx
cd web && npm test
```

**Commit message:** `refactor(web): shared pagination on Findings search`

---

### Task 5 — Tools, Settings, global polish

**Depends on:** Tasks 1–4 (can parallel only after Task 1 if strictly shell-only).

**RED:** Run existing suites:

```bash
cd web && npx vitest run src/pages/ToolsPage.test.tsx src/pages/SettingsPage.test.tsx
```

**GREEN**

1. `ToolsPage.tsx`: wrap analyzer `tool-table` / `table-wrap` in `DataTableShell`; replace any raw `.button` with `Button` variants.
2. `SettingsPage.tsx`: group sections with `Tabs` / `Card` from `ui/*`; use `Input` + destructive `Button` in danger zone.
3. `HomePage.tsx` (copy reduction): if two panels duplicate the same fleet total from stats, demote secondary copy to one line in primary header (per spec §10.3) — **only** when `rg` shows duplicate user-visible totals and tests still pass.
4. `DesignTopbar.tsx`: confirm no palette button added.

**Gate G5:**

```bash
cd web && npm test && npm run build && npm run audit:contrast
```

**Commit message:** `refactor(web): align Tools and Settings with design system shell`

---

### Task 6 — `DataPanel`, motion, CSS retirement

**Depends on:** Tasks 2–5.

**RED:** Add `DataPanel.test.tsx` — renders `panelClassName="board-panel board-ledger"` and `titleId` wired to `aria-labelledby` on section.

**GREEN**

1. Implement `DataPanel.tsx`; optionally refactor `HomePage` ledger **wrapper only** (not row markup) to use `DataPanel` while keeping class hooks.
2. Add `web/src/components/GradeTile.test.tsx`: with `matchMedia('(prefers-reduced-motion: reduce)')` stubbed, `flip` renders static letter without animating `SplitFlapText`. Adjust `GradeTile.tsx` / `kokonutui/split-flap-text.tsx` only when the test fails.
3. Final `styles.css` deletion pass:
   - Grep each removed selector from spec sections (Workspaces, History, Search, Tools).
   - Run contrast + full vitest.

**Gate G6 + series G7:**

```bash
cd web && npm test && npm run build && npm run audit:contrast
wc -l web/src/styles.css   # record before/after in PR description
```

Check design spec §12 manually:

1. Pagination + ListFooter on Overview, Workspaces, History, Search.
2. Ledger 25 rows, `data-workspace-id`, fleet stats independent.
3. Tokens only; a11y suites unchanged strength.
4. No new UI deps.

Optional evidence PR comment: attach synthetic PNGs labeled **synthetic UI/API**.

**Commit message:** `refactor(web): DataPanel shell and retire migrated page CSS`

---

## Activity feed pagination (Overview)

Activity footer (`board-activity`, ~700–716 in `HomePage.tsx`) **must** keep its own `ListFooter`/`Pagination` instance **inside** `section.board-activity` only. Task 2 may refactor markup but **must not** lift controls to a parent that breaks Playwright `ledgerPanel()` scoping.

---

## Regression tests to watch (never weaken)

| Test file | What it guards |
| --------- | -------------- |
| `HomePage.test.tsx` | `pages the ledger…`, `26 workspaces → 25 + 1`, `fleet verdict from global stats when the workspace list fails` |
| `SearchPage.test.tsx` | `page_size=25`, `has_next` on page 2 |
| `WorkspacesPage.test.tsx` | paging math via `has_next` |
| `App.keyboard-journey.test.tsx` | End-to-end keyboard flow |
| `dialogs.test.tsx`, `CommandPalette.test.tsx` | Modal / palette |
| `web/e2e/synthetic-board-verify.mjs` | 25 rows, footer range, workspace id sets differ across pages |

---

## Commit cadence

- One commit per task minimum (Tasks 1–6).
- Normal imperative subject lines (`feat(web):`, `refactor(web):`).
- Author/committer **Sanket Patel &lt;sanpatel323@gmail.com&gt;** when shipping implementation (per repo convention).
- Rebase implementation branches onto current `main` before each PR; merge design spec/plan docs via PR #10 or a chained docs PR as directed by parent.

---

## Document history

| Date | Change |
| ---- | ------ |
| 2026-11-08 | Initial implementation plan (post design approval). |
