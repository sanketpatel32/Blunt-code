// Verifies each loop's specific claim against the live app. Numbers, not vibes.
const PW = 'C:/Users/sanpa/AppData/Roaming/npm/node_modules/@playwright/mcp/node_modules/playwright';
const { chromium } = require(PW);
const EXE = 'C:/Users/sanpa/AppData/Local/ms-playwright/chromium-1243/chrome-win64/chrome.exe';
const BASE = 'http://127.0.0.1:8787';
const WS = 'e02dcf05-e6c9-3a0b-aec0-09bd5c6c0831';
const SCAN = 'fbbfebf7-cd3f-f751-640f-f567ad6cfd5c';
const ok = (c) => (c ? 'PASS' : 'FAIL');

(async () => {
  const b = await chromium.launch({ executablePath: EXE });
  const c = await b.newContext({ viewport: { width: 1440, height: 1000 } });
  const p = await c.newPage();
  const go = async (r) => { await p.goto(BASE + r, { waitUntil: 'networkidle', timeout: 45000 }).catch(()=>{}); await p.waitForTimeout(2200); };

  // L141/L142 — severity colours + one shared facet track
  await go('/search');
  const s = await p.evaluate(() => {
    const dots = [...document.querySelectorAll('.severity-pills .severity-dot')].map((d) => ({
      sev: d.closest('button').querySelector('.capitalize').textContent,
      bg: getComputedStyle(d).backgroundColor,
    }));
    const tracks = [...document.querySelectorAll('.severity-pills .facet-meter')].map((t) => +t.getBoundingClientRect().width.toFixed(1));
    return { dots, tracks };
  });
  const distinct = new Set(s.dots.map((d) => d.bg)).size;
  console.log(`L141 severity dots distinct colours : ${distinct}/5  ${ok(distinct === 5)}`);
  console.log(`       ${s.dots.map((d) => `${d.sev}=${d.bg}`).join('  ')}`);
  const anyGreen = s.dots.some((d) => /rgb\(0?[0-9]?[0-9], 1[0-9][0-9]/.test(d.bg) === false && d.bg.includes('152'));
  console.log(`L141 no green severity               : ${ok(!anyGreen)}`);
  const trackSpread = Math.max(...s.tracks) - Math.min(...s.tracks);
  console.log(`L142 facet tracks = ${s.tracks.join('/')} (spread ${trackSpread.toFixed(1)}px) ${ok(trackSpread < 1)}`);

  // L143 — quiet per-row links
  const links = await p.evaluate(() => {
    const m = {};
    for (const a of document.querySelectorAll('.search-row-open')) {
      const c = getComputedStyle(a).color;
      m[c] = (m[c] || 0) + 1;
    }
    return m;
  });
  const linkColors = Object.entries(links);
  console.log(`L143 search "Open" link colours      : ${linkColors.map(([k, v]) => `${k} x${v}`).join('  ')}`);
  console.log(`       (accent would be oklch(0.5 0.24 268))`);

  // L145 — no raw UUID on the report
  await go(`/scans/${SCAN}`);
  const warn = await p.evaluate(() => {
    const el = document.querySelector('.inline-warning');
    return el ? el.textContent : '';
  });
  console.log(`L145 report warning has no UUID      : ${ok(!/[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}/i.test(warn))}`);
  console.log(`       "${warn.replace(/\s+/g, ' ').slice(0, 150)}"`);

  // L144 — what-changed is not a green flood
  const wc = await p.evaluate(() => {
    const el = document.querySelector('.what-changed');
    if (!el) return null;
    const cs = getComputedStyle(el);
    return { bg: cs.backgroundColor, borderLeft: cs.borderLeftColor };
  });
  console.log(`L144 what-changed bg=${wc?.bg} rail=${wc?.borderLeft} ${ok(wc && !wc.bg.includes('152'))}`);

  // L147 — grade tiles differ  /  L146 — ribbon segments are separated
  await go('/');
  const ribbon = await p.evaluate(() => {
    const segs = [...document.querySelectorAll('.verdict-bar i')];
    return segs.map((s) => {
      const cs = getComputedStyle(s);
      return { w: Math.round(s.getBoundingClientRect().width), shadow: cs.boxShadow !== 'none', op: cs.opacity };
    });
  });
  const separated = ribbon.filter((r) => r.shadow).length;
  console.log(`L146 ribbon segments separated       : ${ok(separated === ribbon.length - 1)} (${separated}/${ribbon.length - 1} internal edges, widths ${ribbon.map((r) => r.w).join('/')}px)`);
  console.log(`       low/info opacity ${ribbon.filter((r) => r.op !== '1').map((r) => r.op).join('/')} ${ok(ribbon.filter((r) => r.op !== '1').length === 2)}`);

  const grades = await p.evaluate(() => [...document.querySelectorAll('.ledger-grade')].map((g) => ({
    grade: g.textContent.trim(),
    depth: g.style.getPropertyValue('--grade-depth') || '(none)',
  })));
  const dVals = grades.filter((g) => g.depth !== '(none)').map((g) => Number(g.depth));
  console.log(`L147 ledger grade tiles              : ${grades.length} rows, ${new Set(dVals).size} distinct depths ${ok(new Set(dVals).size > 1)}`);
  console.log(`       ${grades.map((g) => `${g.grade}:${g.depth}`).join('  ')}`);

  // L148 — path stated once
  await go('/settings');
  const paths = await p.evaluate(() => {
    const found = {};
    const walk = document.createTreeWalker(document.body, NodeFilter.SHOW_TEXT);
    let n;
    while ((n = walk.nextNode())) for (const v of (n.textContent.match(/[A-Za-z]:\\[^\s"']+/g) || [])) found[v] = (found[v] || 0) + 1;
    return found;
  });
  const counts = Object.values(paths);
  console.log(`L148 settings path occurrences       : ${JSON.stringify(paths)} ${ok(counts.every((n) => n === 1))}`);
  const gap = await p.evaluate(() => {
    const rows = [...document.querySelectorAll('.setting')];
    const out = [];
    for (const row of rows) {
      const h3 = row.querySelector('h3');
      const p = row.querySelector('p');
      if (!h3) continue;
      // The value is the row's own last element child.
      const value = row.lastElementChild;
      if (!value || value === h3.parentElement) continue;
      const descRight = p ? p.getBoundingClientRect().right : h3.getBoundingClientRect().right;
      out.push({
        label: h3.textContent,
        gap: Math.round(value.getBoundingClientRect().left - descRight),
      });
    }
    return out;
  });
  const worst = gap.reduce((m, g) => Math.max(m, g.gap), 0);
  console.log(`L148 description->value gap          : ${JSON.stringify(gap)}`);
  console.log(`       worst ${worst}px ${ok(worst < 700)}`);

  // L149/L150 — history
  await go(`/workspaces/${WS}/scans`);
  const hist = await p.evaluate(() => {
    const rows = [...document.querySelectorAll('tbody tr:not(.history-band-row):not(.history-detail-row)')];
    const labels = rows.map((r) => {
      const rel = r.querySelector('.history-relative')?.textContent || '';
      const abs = r.querySelector('.history-abs')?.textContent || '';
      return rel + (abs ? ' / ' + abs : '');
    });
    const dupes = labels.filter((l, i) => labels.indexOf(l) !== i);
    const inputs = [...document.querySelectorAll('.history-filter-input')];
    return {
      labels, dupes,
      svgsPerField: inputs.map((i) => i.parentElement.querySelectorAll('svg').length),
      emptyFlags: inputs.map((i) => i.hasAttribute('data-empty')),
      dashed: inputs.map((i) => getComputedStyle(i).borderTopStyle),
    };
  });
  console.log(`L149 identical date labels           : ${hist.dupes.length} ${ok(hist.dupes.length === 0)}`);
  for (const l of hist.labels) console.log(`       ${l}`);
  console.log(`L150 hand-drawn svgs per date field  : ${JSON.stringify(hist.svgsPerField)} ${ok(hist.svgsPerField.every((n) => n === 0))}`);
  console.log(`L150 empty fields marked + dashed    : ${JSON.stringify(hist.emptyFlags)}/${JSON.stringify(hist.dashed)} ${ok(hist.emptyFlags.every(Boolean))}`);

  // L151 / L153 — sparkline inline, and near its own label
  await go(`/workspaces/${WS}`);
  const spark = await p.evaluate(() => [...document.querySelectorAll('.premium-card')].map((card) => {
    const sp = card.querySelector('.premium-sparkline');
    if (!sp) return null;
    const cr = card.getBoundingClientRect(), sr = sp.getBoundingClientRect();
    const label = card.querySelector('.premium-card-label');
    if (!label) return null;
    const lr = label.getBoundingClientRect();
    return {
      label: label.textContent,
      sameRow: sr.top >= cr.top - 2 && sr.bottom <= cr.bottom + 2,
      gapFromLabel: Math.round(sr.left - lr.right),
      cardW: Math.round(cr.width),
    };
  }).filter(Boolean));
  console.log(`L151 sparkline inline in its card    : ${spark.length ? ok(spark.every((s) => s.sameRow)) : 'no sparks'} (${spark.length} cards)`);
  console.log(`L153 sparkline->label gap            : ${spark.map((s) => `${s.label}=${s.gapFromLabel}px`).join('  ')} ${ok(spark.every((s) => s.gapFromLabel < 200))}`);
  console.log(`       card widths ${spark.map((s) => s.cardW).join('/')}px`);

  // L152 — tree rows: default quiet, exception marked
  await go(`/workspaces/${WS}/files`);
  const tree = await p.evaluate(() => {
    const rows = [...document.querySelectorAll('.tree-row')];
    const checked = rows.filter((r) => r.querySelector('input[type=checkbox]:checked'));
    const unchecked = rows.filter((r) => r.querySelector('input[type=checkbox]:not(:checked)'));
    return {
      total: rows.length, checked: checked.length, unchecked: unchecked.length,
      checkedBg: checked[0] ? getComputedStyle(checked[0]).backgroundColor : null,
      uncheckedBg: unchecked[0] ? getComputedStyle(unchecked[0]).backgroundColor : null,
    };
  });
  console.log(`L152 tree rows ${tree.total} (${tree.checked} in / ${tree.unchecked} out)`);
  console.log(`       included bg=${tree.checkedBg}   excluded bg=${tree.uncheckedBg}`);
  console.log(`       default rows are quiet         : ${ok(!tree.checkedBg || tree.checkedBg === 'rgba(0, 0, 0, 0)')}`);

  await b.close();
})();
