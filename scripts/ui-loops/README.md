# UI loop verification

The evidence behind `UI_POLISH_153_LOOPS.md`. Three scripts, all of which drive
the **running app** in Chromium — none of them read the source or the
stylesheet. That is the point: the defects in that pass (a chart whose pixels
disagreed with its own percentages, a colour map that collapsed two categories
into one, a chart fragment orphaned by a flex line) are invisible in a diff and
in the cascade. You have to measure the rendered box.

## Why this is not a unit test

`web/src/**/*.test.tsx` runs in jsdom, which loads no CSS — so it cannot see a
track that is 7px too short, a wash that renders transparent, or a label 900px
from its value. Those need a layout engine. Hence Playwright against a live
server with a real corpus behind it.

The one thing this suite *cannot* do is invent data. A severity chart with no
severities has nothing to get wrong, so seed it first:

```powershell
.\bluntcode.exe scan . --profile standard
.\bluntcode.exe --no-browser --port 8787
```

## The scripts

| Script | What it does |
|---|---|
| `shots.cjs <outDir>` | Full sweep: every route, light + dark, 1440x1000. Run once per build. |
| `verify-loops.cjs` | Re-derives every before/after claim and prints PASS/FAIL. The regression gate. |
| `compare.cjs <label> <route> <x> <y> <w> <h> [theme]` | Crops a region out of the `before` and `after` captures into one side-by-side plate. |

### Producing a before/after pair

```powershell
# with the old build running
node scripts\ui-loops\shots.cjs .playwright-cli\before
# with the new build running
node scripts\ui-loops\shots.cjs .playwright-cli\after

node scripts\ui-loops\verify-loops.cjs     # must be all PASS
node scripts\ui-loops\compare.cjs L147-grade-gauge home 272 500 740 455
```

`compare.cjs` reads the **stored** captures and crops them with CSS rather than
re-screenshotting the page — an earlier version re-shot the live app for both
sides, which produced two identical "before/after" plates and proved nothing.

### Exit codes

`verify-loops.cjs` prints `PASS`/`FAIL` per claim but always exits 0, because
several claims are informational (counts, depths) rather than boolean. Read the
output; do not gate CI on its exit code. `scripts/verify.ps1` remains the gate.

## Adding a loop

1. Find it by measuring. Add the measurement to `verify-loops.cjs` first, watch
   it report the defect, then fix it and watch it pass.
2. Cover the *intent* in a unit test where one is possible — a shared colour map
   or a pure function is exactly what jsdom is good at. Reach for a browser only
   for what genuinely needs layout.
3. Note the before/after number in `UI_POLISH_153_LOOPS.md`.

Step 1 is not ceremony. Three of the thirteen loops were found by
`verify-loops.cjs` rather than by eye, and one of those exposed an earlier fix
that had silently done nothing at all.
