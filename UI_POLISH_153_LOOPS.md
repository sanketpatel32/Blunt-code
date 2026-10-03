# Blunt Code — UI Improvement Loops 141–153

Continues `UI_POLISH_140_LOOPS.md`. That pass was correctness-first and this one
is **evidence-first**: every loop below was found by measuring the rendered app
in a real browser against a real corpus (10 workspaces, 2,899 findings, 13
scans), not by reading the source or the stylesheet. Where a loop changed a
number, the before and after are both in the table.

Verified: `tsc -b` PASS · `vitest run` 46 files / 529 tests PASS ·
`npm run audit:contrast` PASS · `scripts/verify.ps1` green.

Reproduce the evidence:

```powershell
.\bluntcode.exe scan . --profile standard      # real findings to look at
.\bluntcode.exe --no-browser --port 8787
node scripts\ui-loops\shots.cjs .playwright-cli\before
node scripts\ui-loops\verify-loops.cjs          # prints the PASS/FAIL table
```

See [`scripts/ui-loops/README.md`](scripts/ui-loops/README.md) for why these
cannot be jsdom tests, and for how to produce a before/after plate.

---

## The short version

| # | Loop | Measured before → after |
|---|---|---|
| 141 | **The severity ramp existed; three surfaces refused to use it.** | 3 distinct severity colours → 5 |
| 142 | **Five facet bars, five different denominators.** | track spread 36.7px → **0.0px** |
| 143 | **Nine accent-blue "Open" links down one column.** | 9 × accent → 9 × ink |
| 144 | **"2 findings fixed" was the loudest block on the report.** | green flood → neutral card |
| 145 | **A raw Java exception, two internal UUIDs, one duplicated path.** | 3 lines of stack → 1 sentence |
| 146 | **The board's ribbon was five saturated hues with no boundaries.** | 0 internal edges → 4 |
| 147 | **Six identical red D tiles, ranking nothing.** | 1 distinct tile → 10 distinct depths |
| 148 | **The data directory printed twice; values floated 900px from labels.** | ×2 → ×1; worst gap **~900px → 128px** |
| 149 | **Five scans, five identical `24 Aug 2026` labels.** | 5 identical → 0 identical |
| 150 | **Two calendar icons per date field.** | 2 glyphs → 1 |
| 151 | **A sparkline orphaned onto its own flex line.** | in-card, on-row |
| 152 | **27 identical blue bands for the default state.** | 27 tinted → 27 quiet |
| 153 | **A card stretched 1140px, throwing its sparkline 900px away.** | sparkline→label gap **~900px → 8px** |

---

## A. The severity ramp (141–142)

**141 · One map, five bands, and it was already in the codebase.**
`--color-sev-critical/high/medium/low/info` has been the app's ordered severity
ramp since loop 104. Three surfaces each carried their own hand-rolled JS map
instead, and each was wrong differently:

- the Insights donut mapped `critical` **and `high`** to the same token — the
  two worst bands were one colour, which is the one distinction a severity ramp
  exists to draw;
- it mapped `info` to `--color-success`. Green is this app's *good*. 608
  informational findings painted green read as "fine", which is the one thing a
  severity ramp must never say;
- it mapped `low` to `--color-accent`, the interactive brand blue, so "low
  severity" and "a thing you can click" were the same colour;
- the search facet had the mirror bug: `critical == high`, and `low` **and**
  `info` were *both* `--color-ink-faint` — the two mildest bands were literally
  indistinguishable in the one panel whose job is choosing between them.

`SEVERITY_COLOR` now lives in `lib/chartData.ts` and every surface routes
through it. `chartData.test.ts` asserts five distinct values, that no severity
wears the accent or a success hue, and that each is its own `--color-sev-*`
token — so a surface cannot quietly reintroduce any of this.

**142 · Five bars on five different scales.** The search facet computes widths
as percentages of one shared scale, and the comment says so proudly. But
`.facet-meter` was a bare flex item in a `justify-between` row, so it inherited
whatever was left after the label and the count badge. Measured live:

```
critical 118.3px   high 114.1px   medium 93.2px   low 124.0px   info 129.9px
```

Five denominators for five bars on one scale. Medium's declared 26.8% rendered
as 25px and low's 18.3% as 22.7px — a 2px gap for a 2.15× difference in
findings, purely because medium's track was the shortest. The numbers were right
and the chart was still lying, because the eye compares pixels, not percentages.
`.facet-sev-label` and `.facet-count-slot` are now fixed-width so the meter is
the only elastic thing on the row: **all five tracks measure 80.0px.**

## B. Loudness that did not match importance (143–146)

**143 · Nine blue links.** This is the *third* parallel class for "the repeated
per-row link". v0.32 caught `.row-action`, v0.36 caught `.text-button`, and this
one was `.search-row-open` — which is why the same defect survived two sweeps
that each fixed one of the others. Measured: 9 links, all `oklch(0.5 0.24 268)`,
full accent, stacked down the last column of a 25-row result set. An action
repeated once per row cannot also be the page's emphasis, because by the ninth
row it is furniture. Now `oklch(0.42 0.013 268)`, lifting to accent on hover.

**144 · The green flood.** The report's "What changed" card was a full-bleed
`--color-success-soft` panel with a saturated 3px rail and a 24px green numeral —
roughly 200px of bright green reporting **2 fixed findings**, sitting directly
under a red hero that says 2,899 findings and directly above an amber
"Incomplete analysis" block. Three saturated banners in a row, and the smallest
of the three facts was the brightest.

Loudness is a budget. This follows the rule the severity rows already use
(loops 120/133): the semantic lives in the rail and the ink, not in a flood.
`oklch(1 0 0)` background, `--color-success` rail, `--color-success-text` count
(5.51:1 rather than a 4.79:1 fill used as a beacon). Still unmistakably good
news; it just stopped shouting over the findings.

**145 · A stack trace in the UI.** The report rendered analyzer failures
verbatim. A real SonarQube failure reached the screen as:

> sonarqube: SonarQube compute task FAILED: Fail to process issues of component
> `'bluntcode:e02dcf05-e6c9-3a0b-aec0-09bd5c6c0831:web/src/lib/hackingTests.ts'`
> (Visit of Component `{key=bluntcode:e02dcf05-…:web/src/lib/hackingTests.ts,type=FILE}` failed)

Two internal UUIDs, a Java object dump, and the file path stated twice — all of
it pushing the one actionable sentence across four lines. Nobody debugging this
has the component key; it is not in their repo. Now:

> SonarQube: SonarQube compute task FAILED: Fail to process issues of component
> `'web/src/lib/hackingTests.ts'`.

`humanizeAnalyzerError` strips the keys and the dump, promotes a raw analyzer id
to its display name, and falls back to the untouched text if stripping would
leave nothing — so it can never render a blank chip. The original stays on
`title`; nothing is hidden.

**146 · The ribbon had no edges.** The board's severity ribbon is a 20px,
full-saturation, five-hue bar running the full 1140px of the card — by a wide
margin the loudest object on the landing page, encoding the *least* actionable
thing there: how 5,740 findings distribute across five bands. Two changes:

- a 2px inset separator in the surface colour, so each band reads as a discrete
  quantity. A stacked bar whose neighbours share a hue family is genuinely
  ambiguous about where one band ends;
- `--color-sev-low` and `--color-sev-info` are two different blues at nearly the
  same lightness, so side by side they read as one ambiguous 38%-wide slab. Both
  step to `opacity: .62`, and the ribbon now reads as ordered: hot left, cooling
  right.

## C. Grades that ranked nothing (147)

**147 · Six identical red squares.** The board's ledger sorts six workspaces by
weighted score — 6489, 4682, 1195, 950, 869, 14 — and then prints the same
letter on all six, because D means "50 or more" and D is open-ended. Six
identical dark-red tiles in a column: 360px of the most saturated colour on the
page, ranking nothing, immediately beside a sort order that had just done the
ranking.

`gradeDepth(score, grade)` returns how deep into its band a score sits, and the
tile renders it as a 3px gauge on its bottom edge. The gauge is painted in
`--grade-ink`, which is the whole trick: grade D is a *solid* dark-red fill so
its ink is near-white and the bar reads bright, while A/B/C are soft fills whose
ink is the dark text grade. One token, legible on all four fills, and no existing
background/ink pairing changes, so no contrast guarantee is disturbed.

Depth is per-band, so it resets at a boundary — which is correct: a C at the top
of its band must still fill more than a D at the bottom of its. The open-ended
D band is clamped against an explicit `D_TOP`, chosen as a round number well
clear of the observed corpus rather than derived from it. Live: **10 rows, 10
distinct depths** (`D:1.000 D:0.936 D:0.231 D:0.182 D:0.165 D:0.003 B:0.600
B:0.267 A:0.800 A:0.200`).

> The first attempt washed the whole tile with `--grade-edge`, which is
> `transparent` for grade D by design — so it silently did nothing on exactly
> the rows that needed it. Worth recording, because the screenshot looked
> unchanged and the obvious conclusion ("the CSS is wrong") was the wrong one.

## D. Repetition and disconnection (148)

**148 · The same path, twice, and values adrift.** Settings printed
`C:\Users\sanpa\AppData\Local\BluntCode` as a read-only "Data directory" row
under Privacy *and* again 40px below under Data folders — so the page opened
with the same 34-character string twice and read as though they were two
different directories. It is one directory, and the section that *acts* on it is
the right place to name it; a row that only reports a value is not worth its own
heading. Measured: **×2 → ×1.**

Separately, `.setting` was `justify-content: space-between` across 1140px, which
put "Default scan profile" at x=270 and "Workspace managed" at x=1170 — a 900px
gulf, with the description ending near x=600 and nothing joining it to the word
it describes. A grid with a bounded value column keeps the pair within reading
distance at any width and lines every value up at the same x. **Worst gap across
all eight rows: ~900px → 128px.**

## E. Indistinguishable rows and doubled affordances (149–150)

**149 · Five scans, one label.** `relativeTime` degrades to a bare date past a
week, and the absolute stamp beside it was gated to `age < 7 days` — so every
older row got exactly one label, the date. Live: five consecutive scans all
rendered `24 Aug 2026`, on five rows whose entire purpose is to be told apart.
Two had different finding counts and no visible way to say which was which.

The rule inverts past the 7-day mark: once the label stops being relative it is
no longer distinguishing anything on its own, so the clock is added exactly then.
Minute precision turned out not to be enough — the first pass still produced two
identical labels (`04:32` twice, `04:29` twice), because scans seconds apart are
routine for `--watch` and for a CI rerun. If the stamp's only job is to
disambiguate, minute resolution is not finishing the job, so the disambiguating
stamp carries seconds. Recent rows keep minute precision, because "3 minutes ago"
is already self-ordering and a seconds column there would be noise.

**150 · Four calendar icons for two controls.** Every date field carried the
app's own calendar SVG on the left *and* the browser's
`::-webkit-calendar-picker-indicator` on the right. The hand-drawn one is gone;
the native indicator is styled into the design system and becomes the only
affordance, keeping native date entry, keyboard support and locale formatting.
Both fields also start empty, and `dd-mm-yyyy` gave no hint that "empty" means
"no bound" — so an inactive filter read as a broken control. `data-empty` now
dashes and fades the field until a date lands, and the explanation rides on
`title`, deliberately **not** on the accessible name.

## F. Layout that broke its own contents (151, 153)

**151 + 153 · A sparkline, twice.** The workspace verdict strip is a wrapping
flex row, and the strip holds three cards while the risk card is wide — so the
third card always wraps onto a line of its own. Two things then went wrong:

- `min-width: 12rem` could not hold icon + number + label + a 64px sparkline, so
  the svg wrapped onto a line *of its own* and `margin-left: auto` fired,
  parking a 64×20 chart alone at the far right of an empty second line. It read
  as a broken render.
- `flex: 1 1 auto` then stretched that same card across the full 1140px, so even
  once the sparkline was inline its `margin-left: auto` threw it to the far
  right edge — **~900px from its own label**.

Fixes: the label became the shrinkable element (`flex: 1 1 auto; min-width: 0;
text-overflow: ellipsis`) so the card degrades by shortening a word rather than
orphaning its picture, the sparkline is `flex: none` so it cannot wrap or
squash, `min-width` rose to 13.5rem, and the cards gained `max-width: 30rem`
which only bites when a card is alone on a wrapped line. Measured: **sparkline
→ label gap 8px**, and the strip is a clean three-up row.

## G. The default state shouting (152)

**152 · 27 identical blue bands.** `.tree-row` tinted `:checked`, so a freshly
opened workspace — where every path is included, which is the default and
carries no information — painted all 27 rows the same pale accent. 27 identical
blue bands read as "27 things are special here", and the two rows you actually
excluded looked exactly like the 25 you did not. **The most likely state of this
panel was the least legible.**

A background wash is only good for one thing: marking the exception. Inverting
the selector makes the panel answer "what did I change?" at a glance.
`:not(:checked)` picks up indeterminate directories too, which is exactly the
half-included row you want to notice. Measured: default rows now
`rgba(0, 0, 0, 0)`.

---

## Why these and not more

Every prior pass in this repo's history was found by *reading* — the rendered
page top to bottom, or the stylesheet. That finds repetition, stale copy and
over-claiming. It does not find a chart whose pixels disagree with its own
percentages, a chart fragment orphaned by a flex line, or a colour map that
collapses two categories into one.

Every loop above has a number attached, and `scripts/ui-loops/verify-loops.cjs`
re-derives all of them against the running app. Three of these were found *by*
that script rather than by eye: the five track widths, the `transparent` grade
edge, and the remaining same-minute date collision — the last of which means the
first fix for it was not actually a fix.
