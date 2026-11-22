# CSS retained selectors (desktop design system, PR #11)

This is not a wholesale stylesheet rewrite. Only rules provably superseded by shared
components were removed; everything below stays because it still carries page-specific
layout, legacy DOM contracts, or harness/visual-regression hooks.

## Removed in this wave

| Selector | Reason |
| --- | --- |
| `.history-filter-clear` | History clear control now rides on shared `FilterBar` via `.ws-filter-clear` / `.text-button` (same as Workspaces). |

## Retained (intentional)

| Selector / block | Reason |
| --- | --- |
| `.toolbar-row`, `.toolbar-filters`, `.toolbar-meta` | Base layout for `FilterBar` and pages that have not migrated yet; Tailwind utilities in components layer on top. |
| `.history-filter-field`, `.history-filter-input`, `.history-filter-input[data-empty]` | Date inputs stay History-specific (native picker styling, empty-state dashes). |
| `.history-table-wrap`, `.history-band-row`, `.history-pagination` | Timeline bands, compare workflow, and pagination footer spacing not covered by `DataTableShell` alone. |
| `.board-ledger .ledger-scroll` | **Bounded viewport** for spec desktop density: all 25 server rows remain in the DOM; vertical scroll exposes the rest without slicing or skipping. Footer pagination stays outside the scroll region. |
| `.ledger-row`, `.ledger-list`, grade/score tokens | Playwright `synthetic-board-verify.mjs` and `data-workspace-id` ledger contract. |
| `.feed-filter-btn`, `.feed-list`, `.board-activity` | Activity filter chips and feed density; `DataPanel` wraps structure but not these visual rules. |
| `.ws-filter-clear`, `.ws-count`, `.ws-pagination` | Workspaces list toolbar after `FilterBar` migration. |
| `.search-*` toolbar / query-builder blocks | Search query builder sidebar is unchanged; only the **results** toolbar uses `FilterBar`. |

## Density note (ledger)

At 1440×900 the overview cannot fit 25 ledger rows plus verdict hero, metrics, and activity
without excessive page height. Spec intent is met by **compact row rhythm** (`min-height` on
`.ledger-row`) plus **bounded scrolling** on `.ledger-scroll`, not by reducing the number of
rendered rows or hiding rows from assistive tech.
