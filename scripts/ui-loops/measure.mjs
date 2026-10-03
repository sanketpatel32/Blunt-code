// Measure the rendered app. Every number here comes from a live layout box, not
// from reading the cascade — the defects below are invisible in the source.
//
//   node scripts/ui-loops/measure.mjs [baseUrl]
import { createRequire } from 'node:module';
const { chromium } = createRequire(import.meta.url)('C:/Users/sanpa/AppData/Roaming/npm/node_modules/@playwright/mcp/node_modules/playwright');

const BASE = process.argv[2] || 'http://127.0.0.1:8787';
const EXE = 'C:/Users/sanpa/AppData/Local/ms-playwright/chromium-1243/chrome-win64/chrome.exe';
const WS = 'e02dcf05-e6c9-3a0b-aec0-09bd5c6c0831';

const rows = [];
const say = (gate, claim, actual, pass) =>
  rows.push(`${pass === undefined ? 'INFO' : pass ? 'PASS' : 'FAIL'}  ${gate.padEnd(30)} ${claim.padEnd(52)} ${actual}`);

const browser = await chromium.launch({ executablePath: EXE });

for (const theme of ['light', 'dark']) {
  const ctx = await browser.newContext({
    viewport: { width: 1440, height: 1000 },
    colorScheme: theme,
  });
  await ctx.addInitScript((t) => {
    try { localStorage.setItem('bluntcode-theme', JSON.stringify(t)); } catch {}
  }, theme);
  const page = await ctx.newPage();

  // ---- D1 · the hero number is jammed into running text ------------------
  // The original gate asked whether the number's PARENT held prose too, which
  // is wrong: after the fix the parent is `.verdict-hero-figure`, which
  // correctly holds number + unit and nothing else. What has to be true is
  // that the number and the SCOPE SENTENCE do not share a line — that is the
  // collision, and it is measured from geometry, not from nesting.
  await page.goto(`${BASE}/`, { waitUntil: 'networkidle' });
  await page.waitForTimeout(1500);
  const hero = await page.evaluate(() => {
    const big = [...document.querySelectorAll('*')].find((el) => {
      const t = el.textContent?.trim() ?? '';
      return /^\d{3,}$/.test(t) && parseFloat(getComputedStyle(el).fontSize) > 48;
    });
    if (!big) return null;
    const ctx = document.querySelector('.verdict-hero-ctx');
    const numBox = big.getBoundingClientRect();
    const ctxBox = ctx?.getBoundingClientRect();
    // Do their boxes overlap vertically? If the caption shares the number's
    // line, its top lands inside the number's box.
    const sharesLine = ctxBox ? ctxBox.top < numBox.bottom : null;
    return {
      text: big.textContent.trim(),
      fontSize: getComputedStyle(big).fontSize,
      ctxText: ctx?.textContent.trim().slice(0, 60) ?? '(none)',
      sharesLine,
      label: document.querySelector('.verdict-hero')?.getAttribute('aria-label') ?? '(none)',
    };
  });
  if (hero) {
    say('D1 hero-number', 'hero number and its scope caption share a line', `${hero.text} (${hero.fontSize}) vs "${hero.ctxText}"`, hero.sharesLine === false);
    say('D1 hero-number', 'hero accessible name is one clean sentence', hero.label, /^\d+ findings? across .+ workspaces?\.$/.test(hero.label));
  }

  // ---- D2 · duplicate footer --------------------------------------------
  const footers = await page.locator('footer.app-footer').count();
  say('D2 duplicate-footer', 'footer.app-footer elements in the DOM', `${footers} (expected 1)`, footers === 1);

  // ---- D3 · eyebrow template tell ---------------------------------------
  const eyebrows = await page.evaluate(() =>
    [...document.querySelectorAll('.eyebrow')]
      .map((el) => el.textContent.trim())
      .filter(Boolean));
  say('D3 eyebrow-tell', 'decorative uppercase labels above titles', `${eyebrows.length} on home: ${eyebrows.slice(0, 6).join(' | ')}`, undefined);

  // ---- D4 · h1 scale consistency across routes ---------------------------
  const scales = [];
  for (const [route, label] of [['/', 'home'], ['/workspaces', 'workspaces'], ['/scans/fbbfebf7-cd3f-f751-640f-f567ad6cfd5c', 'report'], [`/workspaces/${WS}`, 'workspace']]) {
    await page.goto(BASE + route, { waitUntil: 'networkidle' });
    await page.waitForTimeout(900);
    const h1 = await page.evaluate(() => {
      const el = document.querySelector('h1');
      if (!el) return null;
      return { size: getComputedStyle(el).fontSize, len: el.textContent.trim().length, text: el.textContent.trim() };
    });
    if (h1) scales.push(`${label}=${h1.size}/${h1.len}ch`);
  }
  const distinct = new Set(scales.map((s) => s.split('=')[1].split('/')[0]));
  say('D4 h1-scale', 'distinct h1 font-sizes across 4 routes', `${scales.join(' ')} -> ${distinct.size} sizes`, distinct.size === 1);

  // ---- D5 · rail real estate --------------------------------------------
  // 15% is the gate: below it the rail starts truncating real labels ("Pentest &
  // Security" is the longest at ~118px), and above ~16% it is withholding
  // width the data tables need. 15.5rem measured 17.2%.
  const rail = await page.evaluate(() => {
    const el = document.querySelector('.app-rail');
    if (!el) return null;
    const r = el.getBoundingClientRect();
    const truncated = [...document.querySelectorAll('.rail-link-label')]
      .filter((l) => l.scrollWidth > l.clientWidth + 1).length;
    return { w: Math.round(r.width), vw: window.innerWidth, truncated };
  });
  if (rail) {
    say('D5 rail-width', 'rail share of a 1440px viewport', `${rail.w}px = ${((rail.w / rail.vw) * 100).toFixed(1)}%`, rail.w / rail.vw <= 0.155);
    say('D5 rail-width', 'rail labels truncated by the narrower rail', `${rail.truncated} truncated`, rail.truncated === 0);
  }

  // ---- D6 · files page density ------------------------------------------
  await page.goto(`${BASE}/workspaces/${WS}/files`, { waitUntil: 'networkidle' });
  await page.waitForTimeout(1200);
  const files = await page.evaluate(() => {
    const rowsEls = [...document.querySelectorAll('.tree-row')];
    const scroll = document.querySelector('.tree-scroll');
    const side = document.querySelector('.rule-editor');
    const h = rowsEls[0]?.getBoundingClientRect().height ?? 0;
    // Does the tree need its own scrolling? `scrollHeight > clientHeight` on a
    // scroller is the honest test — panel height alone cannot tell you whether
    // content is clipped.
    const clipped = scroll ? scroll.scrollHeight > scroll.clientHeight + 2 : null;
    return {
      rowCount: rowsEls.length,
      rowH: Math.round(h),
      clipped,
      sideH: Math.round(side?.getBoundingClientRect().height ?? 0),
    };
  });
  say('D6 files-density', 'tree row height (was 44px)', `${files.rowCount} rows @ ${files.rowH}px`, files.rowH <= 34);
  say('D6 files-density', 'whole loaded tree visible without scrolling', files.clipped === false ? 'no inner scroll' : `INNER SCROLL (${files.clipped})`, files.clipped === false);

  // ---- D7 · report filter chrome ----------------------------------------
  // NOTE: the first version of this gate reported "22 chips in a 39px band",
  // and the write-up turned that into "3 rows / ~150px of filter chrome". That
  // was wrong — the selector grabbed the first `.chip-group` fieldset, one
  // 39px row, not the toolbar. It now measures the whole toolbar and counts
  // how many rows the chips actually wrap onto.
  await page.goto(`${BASE}/scans/fbbfebf7-cd3f-f751-640f-f567ad6cfd5c`, { waitUntil: 'networkidle' });
  await page.waitForTimeout(1500);
  const filters = await page.evaluate(() => {
    const toolbar = document.querySelector('.analysis-toolbar');
    const chips = [...document.querySelectorAll('.analysis-toolbar .chip')];
    const groups = [...document.querySelectorAll('.analysis-toolbar .filter-group')];
    // Rows the chips occupy: distinct rounded `top` values, per group.
    const rowsPerGroup = groups.map((g) =>
      new Set([...g.querySelectorAll('.chip')].map((c) => Math.round(c.getBoundingClientRect().top))).size);
    return {
      chips: chips.length,
      toolbarH: Math.round(toolbar?.getBoundingClientRect().height ?? 0),
      rowsPerGroup,
      groups: groups.length,
      disclosures: document.querySelectorAll('.analysis-toolbar .chip-more').length,
    };
  });
  say('D7 filter-chrome', 'chips per group / rows each wraps to', `${filters.chips} chips in ${filters.groups} groups, rows per group [${filters.rowsPerGroup}], ${filters.disclosures} disclosure(s)`);
  say('D7 filter-chrome', 'whole toolbar height (search + filters)', `${filters.toolbarH}px`, filters.toolbarH <= 130);

  // ---- D8 · panel monotony: how many distinct surface treatments? ---------
  await page.goto(`${BASE}/`, { waitUntil: 'networkidle' });
  await page.waitForTimeout(1200);
  const skin = await page.evaluate(() => {
    const panels = [...document.querySelectorAll('main section, main .summary-card, main > div > div')].filter((el) => {
      const cs = getComputedStyle(el);
      return cs.borderTopWidth !== '0px' || cs.boxShadow !== 'none';
    });
    const sigs = new Set(panels.map((el) => {
      const cs = getComputedStyle(el);
      return `${cs.borderTopWidth}|${cs.borderTopColor}|${cs.borderTopLeftRadius}|${cs.boxShadow.split(',')[0]}`;
    }));
    return { panels: panels.length, treatments: sigs.size };
  });
  say('D8 panel-monotony', 'panels sharing one border+radius+shadow recipe', `${skin.treatments} distinct treatments across ${skin.panels} bordered panels`, skin.treatments >= 2);

  // ---- D9 · touch-target floor on the rail ------------------------------
  const targets = await page.evaluate(() =>
    [...document.querySelectorAll('.rail-link, .rail-search, .icon-button')]
      .map((el) => ({ t: el.textContent.trim().slice(0, 18) || el.getAttribute('aria-label') || '?', h: Math.round(el.getBoundingClientRect().height) }))
      .filter((x) => x.h < 32));
  say('D9 tap-target', 'rail controls under 32px tall', `${targets.length}: ${targets.slice(0, 5).map((x) => `${x.t}@${x.h}`).join(', ')}`, targets.length === 0);

  await ctx.close();
}

await browser.close();
console.log(`\nBlunt Code · rendered measurement — ${BASE} @1440x1000\n`);
for (const r of rows) console.log(r);
