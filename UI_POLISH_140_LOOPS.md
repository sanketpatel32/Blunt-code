# Blunt Code — UI Improvement Loops 101–140

Continues `UI_POLISH_100_LOOPS.md`. This pass is **correctness-first**: instead of
nudging values, it audits for tokens that were defined but never used, rules
declared twice, chrome offsets that disagreed with each other, and controls that
were unreachable or unlabelled. Every loop is a defect removed, not a taste tweak.

Verified: `npm run build` PASS · `vitest run` 31 files / 259 tests PASS.

---

## A. Token foundations (101–112)

| # | Loop |
|---|---|
| 101 | Added `--color-success-ink` / `--color-warning-ink` / `--color-danger-ink`. `ui/button.tsx` referenced `var(--color-danger-ink)` but the token **did not exist**, so every destructive button rendered its label in inherited ink instead of the intended inverse. |
| 102 | Added `--nav-h: 3.5rem`. The sticky chrome used three different nav heights (`3.5rem`, `3.75rem`, `5rem`) — `.main`, `.workspace-context`, `.findings-section .section-head` and `.search-sidebar` all sat at slightly different offsets from the real nav. |
| 103 | Added `--color-on-accent`; replaced 14 hardcoded `white` foregrounds on accent/semantic fills so they invert correctly in dark mode. |
| 104 | Added `--color-sev-critical/high/medium/low/info`. The severity ramp was `color-mix(… 62%, white)` inline in five places — a lighten-only mix that goes the wrong way on a dark canvas. |
| 105 | Added `--tap-min: 2.75rem` (44px). |
| 106 | `.dialog-backdrop` and `.filter-drawer-backdrop` hardcoded `oklch(16% 0.01 268 / .38)` while `--color-dialog-backdrop` existed and was **never referenced anywhere**. Both now use the token. |
| 107 | `analyzerCatalog.ts` mapped 11 categories to 11 raw hex values. Remapped to `--color-cat-*`, extending the ramp with `cat-9..cat-13` so no two categories share a hue. |
| 108 | `THEME_COLORS` was documented as "rough sRGB of `--color-paper`" but the mobile chrome sits against the *nav*, which paints `--color-surface`. Recomputed: `#ffffff` / `#11141a`; `index.html` corrected from the warm `#fbf9f7` / `#1a1c20` to match the cool token palette. |
| 109 | Added `--shadow-inset-critical/high/medium` for the row severity edge. |
| 110 | Added `--page-gutter`; the page inline padding was overridden per-breakpoint in three places. |
| 111 | Added `--color-skeleton` / `--color-skeleton-sheen`; the shimmer gradient was hand-written three times. |
| 112 | Added `--color-scrollbar-thumb` / `--color-scrollbar-track` and applied to `html`, `.table-wrap`, dialogs and the palette list. |

## B. Stylesheet hygiene (113–120)

| # | Loop |
|---|---|
| 113 | **The toast stack was declared twice** — an early "premium" block and a later "shadcn refinement" block with conflicting widths (28rem vs 24rem), padding, icon size and exit behaviour. Merged into one block keeping the wider stack, the kind-coloured left rail, the real `toast-out` animation and the pause-on-hover lifetime bar. |
| 114 | Two competing `@keyframes toast-in` with different travel distances, plus `toast-life` vs `toast-lifetime`. Unified to one `toast-in` / `toast-out` / `toast-lifetime` triple. |
| 115 | Removed the empty rule `.button:not(:disabled):hover { }`. |
| 116 | `scrollbar-gutter: stable` was declared twice on `html`. |
| 117 | **`.tree-toolbar` was declared twice** — `{gap: var(--space-xs); flex-wrap: wrap}` at line ~372 and `{justify-content: flex-end; min-height: 1.6rem}` at ~670. The later one silently won, so the wrap intent was dead. Merged. |
| 118 | Removed `will-change` from six static elements (`.workspace-hero-bento`, `.workspace-next`, `.summary-card`, `.workspace-section-card`, `.tree-panel`, `.rule-editor`, `.progress-fill`). They never animate; each was pinning a compositor layer for nothing. Kept it only where a transform/opacity/height animation actually runs. |
| 119 | Removed `backdrop-filter: blur(8px)` from `.filters`, which paints an **opaque** `--color-surface-muted` — a full-screen backdrop pass for zero visual gain. |
| 120 | The five severity rules were duplicated across `.severity-bar`, `.severity-legend` and `.trend-chart` (15 rules → 1 ramp). Side effect: `high` is now visually distinct from `critical` in `.severity-dots` instead of both being `--color-danger`. |

## C. Accessibility (121–128)

| # | Loop |
|---|---|
| 121 | **Dark mode was unreachable on phones.** The only theme toggle in the app carried `hidden sm:`, and its label already collapses at 68rem — so the button was safe to show at every width as an icon. |
| 122 | **Language was unreachable below 640px.** Nav control now hides below `md`, and Settings gained a full-width language row so the setting is never width-gated. |
| 123 | `.copy-finding` (27px), `.toast-dismiss` (24px), `.filter-chip-remove` (19px), `.workspace-copy-btn` (26px), `.tree-toggle` (24px) were all well under the 44px minimum. Each gets a transparent `::after` hit pad under `@media (pointer: coarse)` — targets grow, visuals don't move, mouse precision is untouched. |
| 124 | Anchors and deep links landed underneath the sticky nav. Added `scroll-margin-top: calc(var(--nav-h) + var(--space-lg))`. |
| 125 | Added a `prefers-contrast: more` block: tightens the ink ramp, promotes hairlines to full rules, thickens the severity edge so severity is never carried by hue alone. Dark-mode override restated because `:root[data-theme='dark']` outranks `:root`. |
| 126 | `aria-expanded` was set on `<tr>`. A row maps to `role=row`, which does not support `aria-expanded` — the attribute was invalid and ignored. Removed; the state is now announced as sr-only text in the summary cell. |
| 127 | Rows and pills that behave like controls (`.tree-row`, `.analyzer-result`, `.workspace-context-link`, `.severity-pill`, `.quick-pill`, `.lang-pill`, `.export-item`, `.filter-chip-remove`, `.activity-row > button`) had no focus ring — the global rule only covers native `button`/`input`/`select`/`a`. |
| 128 | The expandable finding row is a `<tr>` with `onClick` + `tabIndex=0` but no accessible name, so a screen reader announced an unnamed row. Added an `aria-label` carrying severity, title, path and the expand/collapse action. |

## D. Typography & layout (129–134)

| # | Loop |
|---|---|
| 129 | `text-wrap: pretty` on running prose, `balance` on short display text (card titles, metric labels, analyzer names). |
| 130 | `hyphens: auto` + `overflow-wrap: anywhere` on fingerprint/path/code strings that used to stretch their container. |
| 131 | `.app-frame` is now a flex column with `min-height: 100dvh` and `main { flex: 1 }`. Replaces `min-height: calc(100vh - 5rem)` — a magic number that guessed the footer height and left the footer floating mid-screen on short pages. |
| 132 | `font-variant-numeric: tabular-nums` on every in-place-updating metric (elapsed time, counts, percentages, durations) so digits stop jittering as values change. |
| 133 | **CSS-only horizontal scroll affordance on `.table-wrap`.** Tables scroll sideways with no hint that content is hidden. Two `local` gradient layers mask two `scroll`-anchored shadows until there is actually overflow on that side — no JS, no ResizeObserver. |
| 134 | Empty and error states kept their desktop `3xl` padding on phones; tightened under 38rem. |

## E. Components & shell (135–140)

| # | Loop |
|---|---|
| 135 | **The footer hardcoded `v0.7` while the app shipped `0.15.0`.** Version is now injected from `package.json` via a Vite `define`, so it cannot drift again. |
| 136 | Dialogs now use `--shadow-dialog` and `--z-dialog` (both defined, previously unused — the backdrop hardcoded `z-index: 40`), plus shared scrollbar chrome and `overscroll-behavior: contain`. |
| 137 | Command palette list gets the shared scrollbar tokens and a hairline between result groups. |
| 138 | `ToastStack` had `role="status"` **and** `aria-live="polite"` (redundant) and no accessible name, so screen readers announced bare text with no context. Added `aria-label="Notifications"` and `aria-atomic="false"` so each toast is its own announcement. |
| 139 | **`.skeleton-chart` had a fixed `height: 9.5rem`** but the chart-grid variant renders three `h-24` (6rem) cards plus gaps — roughly 18rem of content in a 9.5rem box, silently overflowing. Changed to `min-height`. |
| 140 | Print: `@page` margins, `break-inside: avoid` on rows/cards, `break-after: avoid` on headings, `thead` repeats via `display: table-header-group`, and interactive-only chrome (export menu, palette, skip link, dismiss buttons) is dropped. |

---

## Files touched

```
web/index.html                       theme-color meta + boot script
web/vite.config.ts                   __APP_VERSION__ define
web/tsconfig.node.json               resolveJsonModule
web/src/env.d.ts                     (new) __APP_VERSION__ declaration
web/src/tokens.css                   ~30 new tokens, light + dark
web/src/styles.css                   dedupe, offsets, a11y, print
web/src/css/animations.css           skeleton tokens
web/src/hooks/useTheme.ts            THEME_COLORS derived from --color-surface
web/src/lib/analyzerCatalog.ts       hex → --color-cat-*
web/src/components/AppShell.tsx      mobile theme toggle, md language, footer version
web/src/components/toasts.tsx        live-region semantics
web/src/components/NotificationsCenter.tsx   danger-strong badge
web/src/components/DataTable.tsx     accent-ink
web/src/components/ui/table-toolbar.tsx      accent-ink / paper on ink hover
web/src/components/ui/calendar.tsx   accent-ink
web/src/components/LanguageCoverage.tsx      accent-ink
web/src/pages/SettingsPage.tsx       language setting row
web/src/pages/WorkspacesPage.tsx     accent-ink
web/src/pages/ToolsPage.tsx          category chip contrast
web/src/pages/report/ReportView.tsx  row aria-label, invalid aria-expanded, contrast
```

## Bonus — Loop 141: contrast regressions the audit caught after the fact

Running the `contrast-audit` tool against the *shipped* pairings (not the exploratory sweep) surfaced four real WCAG failures that loops 101–140 did not cover. All four are fixed. Every measurement below is computed, not estimated.

| Defect | Where | Before | After |
|---|---|---|---|
| Success fill + success ink | `.scan-flow li.done .flow-marker` ✓ glyph, `0.6875rem` bold on a filled circle — so AA-normal applies, not the 3:1 icon rule | `oklch(56% 0.15 152)` → **4.22:1, fail** | new `--color-success-strong: oklch(50% 0.16 152)` → **5.27:1** |
| Dark danger badge + on-accent | `NotificationsCenter` unread count, **10px text** | `oklch(52% 0.21 24)` → **3.23:1, fail** | dark `--color-danger-strong: oklch(66% 0.22 24)` → **5.77:1** |
| Light `ink-faint` on paper | every metadata line in the app | `oklch(56% …)` → **4.47:1, fail by 0.03** | `oklch(55% …)` → **4.66:1** |
| Dark `ink-ghost` on dark paper | dimmest labels | `oklch(54% …)` → **3.93:1, fail** | `oklch(58% …)` → **4.64:1** |

Two notes on judgement calls:

- **`--color-danger-strong` inverts direction per theme, and that is intentional.** "Strong" means *maximum contrast against its ink*. In light mode the ink is near-white, so strong goes **darker** (46%). In dark mode the ink is near-black, so strong goes **brighter** (66%). Darkening the dark-mode ink does not work — I swept it and the ratio floors at 3.41 because the background luminance is too low, so the background had to move.
- **Light `ink-ghost` (3.11:1) was deliberately left failing.** Reaching 4.5:1 needs roughly 53%, which is darker than `ink-faint` and would collapse two steps of the ramp into one. It is decorative-only; the token now says so in a comment so nobody "fixes" it later.

Also corrected: the comment on light `--color-danger-strong` claimed `--color-danger` reaches only ~3.5:1 against white. It actually measures **4.79:1**. The token is still worth having — the badge it serves is 10px — but the stated reason was wrong, so it now gives the real number.

`scripts/contrast-audit.mjs` was rewritten from an exploratory sweep into a **regression guard**: it now lists only pairings that actually ship, names the CSS rule that creates each one, and **exits non-zero** on any AA failure. Wired up as `npm run audit:contrast`. All 10 pairings pass.

```
LIGHT accent bg + accent-ink          (.button.primary, .segmented-btn.active)  4.80  PASS
LIGHT accent-strong bg + accent-ink   (.button.primary:hover)               6.33  PASS
LIGHT danger bg + danger-ink          (.button.danger:hover)                4.65  PASS
LIGHT danger-strong bg + on-accent    (NotificationsCenter unread badge)    7.18  PASS
LIGHT success-strong bg + success-ink (.scan-flow li.done .flow-marker)     5.27  PASS
LIGHT warning bg + warning-ink        (badge fill)                          6.63  PASS
DARK accent bg + on-accent            (.button.primary, .segmented-btn.active)  5.73  PASS
DARK danger bg + danger-ink           (.button.danger:hover)                5.39  PASS
DARK danger-strong bg + on-accent     (NotificationsCenter unread badge)    5.77  PASS
DARK success-strong bg + success-ink  (.scan-flow li.done .flow-marker)     4.65  PASS
```

The one-off `scripts/tune.mjs` sweep was deleted; `contrast-audit.mjs` is kept because it is a permanent guard.

## Previously listed as follow-ups — all since resolved

These were in the follow-up list after loops 101–140 and are now **done**, so the list is not carried forward:

- **Google Fonts is no longer `@import`-ed from `tokens.css`.** It moved to a non-blocking `<link>` in `index.html`: `media="print"` keeps it off the critical path and an `onload` handler promotes it to `all` once it arrives, with a `<noscript>` fallback and `preconnect` hints for both `fonts.googleapis.com` and `fonts.gstatic.com`. The old arrangement forced two serial round trips (`styles.css` → `tokens.css` → Google) before any of the cascade could apply. **Offline behaviour is unchanged** — in the packaged desktop binary the request fails and the system-font fallbacks in `--font-body` / `--font-mono` carry the UI, exactly as before. `tokens.css` now carries a comment saying where the fonts come from so nobody re-adds the import.
- `--text-micro` is **removed**. It duplicated `--text-2xs` at `0.6875rem` and had zero usages in `src/`. A note at the definition of `--text-2xs` records this.
- `--color-cat-7` is **not a duplicate literal** — it is a deliberate alias (`var(--color-accent-strong)`). The categorical ramp is indexed positionally by chart code, so `cat-1..13` has to stay a complete series even where a slot shares a value. Left as is on purpose.

## Still open

- **Version bump / release.** `cmd/bluntcode/main.go`, `web/package.json`, and `scripts/package.ps1` all still say `0.15.0`, and `CHANGELOG.md` has not been promoted. Required by the `AGENTS.md` ship rule before this is released. Not done — a release publishes to GitHub, which is an external action.
