# Accessibility evidence and conformance statement

This document is the IMP-16 deliverable: it records what accessibility evidence actually exists, so the conformance claim cites that evidence instead of treating a contrast check as the whole assessment.

## Conformance statement (authoritative)

Blunt Code's web UI is **built toward WCAG 2.2 AA** with the following evidence-backed scope:

- **Contrast (automated):** `npm run audit:contrast` (`web/scripts/contrast-audit.mjs`) measures every shipped design-token text/background pairing in both themes and fails below 4.5:1. Run locally; there is no CI in this repository.
- **Keyboard operation, focus management, dialog behavior, semantic labels, live-region structure (automated, jsdom):** enumerated below — 40+ component and page tests assert these properties directly.
- **Screen-reader experience (structural evidence only):** roles, names, and live regions are asserted in tests; no real screen-reader pass (NVDA/Narrator) is automated. See the manual checklist.
- **Zoom/reflow and localized narrow layouts (manual):** not automatable in jsdom; see the manual checklist. No conformance claim is made for reflow until that checklist has been executed and recorded.

**What is NOT claimed:** full WCAG 2.2 AA conformance. The claim is limited to the categories above, each tied to named evidence. Any broader statement (for example in marketing docs) should link here.

## Automated evidence inventory

All tests are in `web/src` (`npx vitest run`).

| Concern | Evidence |
|---|---|
| Skip link + main landmark | `src/App.navigation.test.tsx` — skip link is the first link in the app, targets `#main-content`; `<main id="main-content" tabIndex={-1}>` |
| Keyboard journey through the core flow | `src/App.keyboard-journey.test.tsx` — workspace → start scan → incomplete result (badge, failure reason, engine tally) → finding row via ArrowDown/Enter → source pane → Escape → back out, no focus trap |
| Dialog semantics: modal role, labelling, initial focus, Tab wrap trap, Escape, focus restore, backdrop close, busy guard | `src/components/dialogs.test.tsx` (add-workspace, confirmation, suppress, source-preview dialogs; shared `hooks/useDialogA11y`) |
| Command palette keyboard model | `src/components/CommandPalette.test.tsx` — arrows + `aria-activedescendant`, live empty message, focus in/out |
| Global shortcuts suspend while a dialog is open; ignored while typing; SR modifier combos excluded | `src/App.shortcuts.test.tsx`, `src/lib/shortcuts.test.ts` |
| Findings rows keyboard-walkable; pane Escape | `src/pages/report/ReportView.test.tsx` (ArrowDown focus, Enter opens pane, Escape closes) |
| Live regions for loading/errors/counts | `src/App.test.tsx` (screen-reader loading label), `src/components/toasts.test.tsx` (role=status/alert, Escape dismiss, focus pauses countdown), plus per-page `role="status"` assertions (history, files, about, report foot) |
| Charts carry text alternatives | `src/components/SeverityTrendChart.test.tsx` — SR summary, keyboard-focus tooltip, tooltip quiet for SR |
| Non-color severity/status cues | Findings rows and filter chips pair a colored dot (`aria-hidden`) with the text label; `StatsOverview`/`HomePage` severity bars are `role="img"` with computed labels plus a text legend; scan states always render as visible text (`scanStateDisplay` labels); `prefers-contrast: more` adds severity row edges (`web/src/css/styles.css`) |
| Reduced motion | Global kill switch in `web/src/css/animations.css` + `src/hooks/useCountUp.test.tsx` (count-up jumps under reduced motion) |
| Router edge behavior | `src/lib/router.test.ts` (all shapes, aliases, hostile ids stay inert data); `src/App.navigation.test.tsx` — popstate back/forward, unknown scan id renders the error panel (never executes the id), hostile `?q=` renders as an inert input value |
| Locale behavior | `src/lib/i18n.test.tsx` — six locales ship labels/names; no orphan keys outside English; translated-missing → English → raw-key fallback; switching updates `<html lang>` and persists; no dictionary key shadows internal severity/state identifiers |

Known structural notes (not defects, recorded for accuracy):

- Dates and numbers use the browser's locale (`Intl` with `undefined` locale in `web/src/lib/format.ts`), not the app language selected in the nav dropdown. Switching UI language does not retarget formatting.
- `HistoryPage` renders scan states via raw state text (`completed_with_warnings` → `completed with warnings`) rather than `scanStateDisplay`; still a visible text cue, but inconsistent labeling with the rest of the UI.
- Critical and high severities share the danger color in search pills; the text label distinguishes them.

## Manual checklist (run before releases that touch UI chrome)

Record results with the release evidence (`docs/release-evidence.md`).

1. **Zoom/reflow:** at 200% and 400% browser zoom, and at 320 px viewport width, confirm no horizontal scrolling of interactive controls and no overlap that hides controls (WCAG 1.4.4 / 1.4.10).
2. **Localized narrow layouts:** switch the nav language to Deutsch and 日本語 (longest shipped strings) and repeat the narrow-viewport pass over nav, workspace cards, and the findings toolbar.
3. **Screen reader:** with NVDA or Windows Narrator, walk the keyboard journey from `src/App.keyboard-journey.test.tsx`: reach a workspace, start a scan, understand the incomplete-result messaging, inspect a finding, return. Confirm announcements (not silence) at each step and no trap.
4. **Windows high-contrast mode:** confirm severity/status remain distinguishable (text labels, edges).

## Documentation alignment (IMP-16 item 5)

- `docs/ARCHITECTURE_AND_FEATURES.md` states "Built to WCAG 2.2 AA standards with automated contrast validation" — the contrast half is backed by `audit:contrast`; the broader sentence should link to this document's narrowed statement. (That file is owned by a parallel work stream; the one-line sync is intentionally left to it.)
- The interactive CLI manual is kept in lock-step with the parser by `cmd/bluntcode/help_test.go` `TestScanUsageAndManualDocumentEveryScanFlag`: every flag the scan FlagSet accepts must appear in both the usage line and the manual's scan section. This test caught and now prevents the drift where `--save-baseline`, `--gate-analyzer`, `--gate-category`, `--watch-poll`, `--watch-quiet`, `--github-cap`, `--json`, and `--timeout` were missing from the condensed docs.
