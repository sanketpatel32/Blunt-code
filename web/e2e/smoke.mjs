// End-to-end smoke checks against a RUNNING Blunt Code (UI + API).
//
//   node e2e/smoke.mjs                         # BASE defaults to http://127.0.0.1:5173
//   BASE=http://127.0.0.1:8787 node e2e/smoke.mjs
//   SMOKE_PROJECT=/path/to/a/project node e2e/smoke.mjs
//
// Needs `playwright` (+ a Chromium) and `@axe-core/playwright` resolvable from
// here; they are not app dependencies. The run registers a throwaway
// workspace pointing at SMOKE_PROJECT (a tiny generated project by default),
// scans it through the UI, exercises the main flows, and removes the
// workspace again. Exit code 1 on any failed check.
import { mkdirSync, writeFileSync, mkdtempSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { chromium } from 'playwright';
import AxeBuilder from '@axe-core/playwright';

const BASE = process.env.BASE ?? 'http://127.0.0.1:5173';
const results = [];
let failed = 0;
async function check(name, fn) {
  const started = Date.now();
  try { await fn(); results.push(`PASS  ${name} (${Date.now() - started}ms)`); }
  catch (error) { failed++; results.push(`FAIL  ${name}: ${String(error?.message ?? error).split('\n')[0].slice(0, 300)}`); }
}
function assert(cond, message) { if (!cond) throw new Error(message); }

function makeProject() {
  if (process.env.SMOKE_PROJECT) return process.env.SMOKE_PROJECT;
  const dir = mkdtempSync(join(tmpdir(), 'bluntcode-smoke-'));
  mkdirSync(join(dir, 'src'));
  writeFileSync(join(dir, 'src', 'app.js'), "const key = 'AKIA' + 'IOSFODNN7EXAMPLE';\n// TODO: remove eval\nmodule.exports = (x) => eval(x);\n");
  writeFileSync(join(dir, 'src', 'util.py'), 'import os\n\ndef run(cmd):\n    # FIXME shell injection\n    os.system(cmd)\n');
  writeFileSync(join(dir, 'package.json'), JSON.stringify({ name: 'smoke-app', version: '1.0.0', license: 'MIT' }, null, 2));
  return dir;
}

const browser = await chromium.launch();
const context = await browser.newContext({ viewport: { width: 1280, height: 800 } });
const page = await context.newPage();
const consoleErrors = [];
page.on('console', (m) => { if (m.type() === 'error') consoleErrors.push(`${page.url()} :: ${m.text()}`); });
page.on('pageerror', (e) => consoleErrors.push(`${page.url()} :: ${e}`));

async function visit(path) {
  await page.goto(BASE + path, { waitUntil: 'networkidle' });
  await page.waitForTimeout(300);
  const crashed = await page.getByText(/Something went wrong/i).count();
  assert(!crashed, `error boundary on ${path}`);
  await page.locator('h1').first().waitFor({ timeout: 8000 });
}
const api = async (method, path, body) => {
  const res = await page.request.fetch(BASE + '/api/v1' + path, { method, data: body });
  assert(res.ok(), `${method} ${path} -> ${res.status()}`);
  return res.status() === 204 ? null : res.json();
};

const projectDir = makeProject();
let wsId = null;
let scanId = null;

// 1 · Add a workspace through the UI.
await check('add workspace (UI dialog)', async () => {
  await visit('/workspaces');
  await page.getByRole('button', { name: /^Add workspace$/ }).first().click();
  const dialog = page.getByRole('dialog');
  await dialog.getByLabel(/Folder path/).fill(projectDir);
  await dialog.getByLabel(/Workspace name/).fill('smoke-app');
  await dialog.getByRole('button', { name: /^Add workspace$/ }).click();
  await page.waitForURL(/\/workspaces\/[0-9a-f-]{36}/, { timeout: 15000 });
  wsId = page.url().match(/workspaces\/([0-9a-f-]{36})/)[1];
  await page.getByRole('heading', { name: 'smoke-app' }).waitFor();
});

// 2 · Run a scan through the UI and read the results.
await check('run scan from workspace page → results', async () => {
  assert(wsId, 'no workspace');
  await visit(`/workspaces/${wsId}`);
  await page.getByRole('button', { name: /^Run scan$/ }).first().click();
  const dialog = page.getByRole('dialog');
  const start = dialog.getByRole('button', { name: /^Run .*scan$/i });
  await start.waitFor();
  await page.waitForFunction(() => { const b = [...document.querySelectorAll('[role=dialog] button')].find((x) => /^Run .*scan$/i.test(x.textContent.trim())); return b && !b.disabled; }, null, { timeout: 15000 });
  await start.click();
  await page.waitForURL(/\/scans\/[0-9a-f-]{36}/, { timeout: 15000 });
  scanId = page.url().match(/scans\/([0-9a-f-]{36})/)[1];
  for (let i = 0; i < 90; i++) {
    const scan = await api('GET', `/scans/${scanId}`);
    if (['completed', 'completed_with_warnings', 'failed', 'cancelled', 'interrupted'].includes(scan.state)) break;
    await page.waitForTimeout(1000);
  }
  await page.reload({ waitUntil: 'networkidle' });
  await page.locator('.scan-grade').waitFor({ timeout: 10000 });
  const rows = await page.locator('table tbody tr').count();
  assert(rows > 0, 'no finding rows rendered');
});

// 3 · Findings filters on the report.
await check('findings filters narrow the list', async () => {
  assert(scanId, 'no scan');
  await visit(`/scans/${scanId}`);
  const count = async () => Number((await page.locator('text=/^\\d+ findings?$/').first().textContent()).match(/\d+/)[0]);
  const before = await count();
  const chips = page.locator('.filter-group button, .filter-group [role=checkbox]');
  assert(await chips.count() > 0, 'no filter chips');
  await page.getByPlaceholder(/Search message, rule, or file/).fill('zzz-no-such-finding');
  await page.waitForTimeout(600);
  const after = await count();
  assert(after < before || before === 0, `search filter did not narrow (${before} → ${after})`);
  await page.getByPlaceholder(/Search message, rule, or file/).fill('');
});

// 4 · History lists the scan and opens its report.
await check('history → open report', async () => {
  await visit(`/workspaces/${wsId}/scans`);
  await page.getByRole('button', { name: 'Open report' }).first().click();
  await page.waitForURL(/\/scans\//);
});

// 5 · Tools filter tabs.
await check('tools page + filter', async () => {
  await visit('/tools');
  assert(await page.locator('table tbody tr').count() > 0, 'no tools rows');
  await page.getByRole('button', { name: /^Ready/ }).first().click();
  await page.waitForTimeout(300);
});

// 6 · Rules editor validates a draft.
await check('hidden features: /rules is a graceful 404 and nothing links to it', async () => {
  await visit('/rules');
  await page.getByText(/Page not found|not found/i).first().waitFor();
  await visit('/');
  assert(!(await page.locator('a[href="/rules"]').count()), 'a link to /rules is still on screen');
});
await check('hidden features: no ZAP/Nuclei/Burp tabs or Execute Check on the pentest page', async () => {
  await visit(`/workspaces/${wsId}/pentest`);
  const tabs = await page.getByRole('tab').allInnerTexts();
  assert(!tabs.some((t) => /ZAP|Nuclei|Burp/.test(t)), `tabs: ${tabs.join(' | ')}`);
  assert(!(await page.getByRole('button', { name: /Execute Check/ }).count()), 'Execute Check is still shown');
  await visit(`/workspaces/${wsId}`);
  assert(!(await page.getByRole('tab', { name: 'Dependencies' }).count()), 'Dependencies tab is still shown');
});

// 7 · Settings: language switch and back.
await check('settings: language ES then EN', async () => {
  await visit('/settings');
  await page.getByRole('button', { name: /^ES$/ }).first().click();
  await page.waitForTimeout(400);
  const lang = await page.evaluate(() => document.documentElement.lang);
  assert(lang.startsWith('es'), `html lang is ${lang}`);
  await page.getByRole('button', { name: /^EN$/ }).first().click();
  await page.waitForTimeout(300);
});

// 8 · Dark mode via the theme switch, persisted across reload.
await check('theme switch → dark, persists', async () => {
  await visit('/');
  await page.locator('.theme-toggle').first().click();
  await page.waitForTimeout(700);
  assert(await page.evaluate(() => document.documentElement.dataset.theme) === 'dark', 'not dark after toggle');
  await page.reload({ waitUntil: 'networkidle' });
  assert(await page.evaluate(() => document.documentElement.dataset.theme) === 'dark', 'dark not persisted');
});

await check('axe: 0 WCAG A/AA violations in dark (home, report)', async () => {
  for (const path of ['/', `/scans/${scanId}`]) {
    await visit(path);
    await page.waitForTimeout(500);
    const { violations } = await new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa']).analyze();
    assert(!violations.length, `${path}: ${violations.map((v) => `${v.id}×${v.nodes.length}`).join(', ')}`);
  }
  await page.locator('.theme-toggle').first().click();
  await page.waitForTimeout(700);
});

// 9 · Command palette navigates.
await check('command palette (Ctrl+K) → Tools', async () => {
  await visit('/');
  await page.keyboard.press('Control+k');
  const input = page.getByRole('dialog').locator('input').first();
  await input.waitFor();
  await input.fill('tools');
  await page.waitForTimeout(200);
  await page.keyboard.press('Enter');
  await page.waitForURL(/\/tools/, { timeout: 5000 });
});

// 10 · Pentest page runs its suite.
await check('pentest page: Run Pentest Scan', async () => {
  await visit(`/workspaces/${wsId}/pentest`);
  await page.getByRole('button', { name: /Run Pentest Scan/ }).first().click();
  const confirm = page.getByRole('dialog').getByRole('button', { name: /Run Pentest Scan/ });
  if (await confirm.count()) await confirm.click();
  await page.waitForTimeout(1500);
  const crashed = await page.getByText(/Something went wrong/i).count();
  assert(!crashed, 'pentest crashed');
});

// 11 · Unknown route.
await check('404 page', async () => {
  await page.goto(BASE + '/definitely/not/a/page', { waitUntil: 'networkidle' });
  await page.getByText('Page not found').first().waitFor();
});

// 12 · Every route renders, light, desktop + phone.
const routes = ['/', '/workspaces', `/workspaces/${wsId}`, `/workspaces/${wsId}/files`, `/workspaces/${wsId}/scans`, `/workspaces/${wsId}/pentest`, `/scans/${scanId}`, '/search', '/tools', '/settings', '/about', '/cli'];
// `html, body { overflow-x: clip }` hides document scrollWidth, so sideways
// overflow is measured by element right edges instead.
async function overflowingElements() {
  return page.evaluate(() => {
    const limit = window.innerWidth + 1;
    const out = [];
    for (const el of document.querySelectorAll('main *, header *')) {
      const r = el.getBoundingClientRect();
      if (r.width === 0 || r.height === 0 || r.right <= limit) continue;
      let clipped = false;
      for (let p = el.parentElement; p && p !== document.body; p = p.parentElement) {
        const cs = getComputedStyle(p);
        if (/(auto|scroll|hidden|clip)/.test(cs.overflowX) && p.getBoundingClientRect().right <= limit) { clipped = true; break; }
      }
      if (!clipped && getComputedStyle(el).position !== 'fixed') out.push(`${el.tagName.toLowerCase()}.${String(el.className).split(' ')[0]}→${Math.round(r.right)}`);
      if (out.length > 4) break;
    }
    return out;
  });
}
const VIEWPORTS = [{ name: 'wide', width: 1440, height: 900 }, { name: 'laptop', width: 1280, height: 800 }, { name: 'tablet', width: 820, height: 1180 }, { name: 'phone', width: 390, height: 844 }];
for (const viewport of VIEWPORTS) {
  await check(`every route renders at ${viewport.name} ${viewport.width}px with no element past the right edge`, async () => {
    await page.setViewportSize({ width: viewport.width, height: viewport.height });
    const bad = [];
    for (const route of routes) {
      await visit(route);
      const over = await overflowingElements();
      if (over.length) bad.push(`${route}: ${over.join(', ')}`);
    }
    assert(!bad.length, bad.slice(0, 3).join(' | '));
  });
}

await check('tablet: icon rail shows, labels hidden, nav links still named', async () => {
  await page.setViewportSize({ width: 820, height: 1180 });
  await visit('/workspaces');
  const rail = page.locator('aside.app-rail');
  const box = await rail.boundingBox();
  assert(box && box.width > 40 && box.width < 120, `rail width ${box?.width}`);
  const link = rail.locator('nav[aria-label="Main navigation"] a').first();
  const name = (await link.getAttribute('aria-label')) || (await link.getAttribute('title')) || (await link.innerText());
  assert(name && name.trim().length > 0, 'icon-only nav link has no accessible name');
  await rail.locator('nav[aria-label="Main navigation"] a', { hasText: /Tools/ }).or(rail.locator('a[aria-label="Tools"], a[title="Tools"]')).first().click();
  await page.waitForURL(/\/tools$/, { timeout: 5000 });
});

await check('phone: topbar controls are 40px tap targets, bell present; page buttons ≥24px (WCAG 2.5.8)', async () => {
  await page.setViewportSize({ width: 390, height: 844 });
  await visit('/');
  const tiny = await page.locator('main button:visible, main a.button:visible').evaluateAll((els) => els.map((el) => { const r = el.getBoundingClientRect(); return { name: el.getAttribute('aria-label') || el.textContent.trim().slice(0, 30), w: Math.round(r.width), h: Math.round(r.height) }; }).filter((s) => s.w < 24 || s.h < 24));
  assert(!tiny.length, `buttons under 24px: ${JSON.stringify(tiny.slice(0, 4))}`);
  const options = page.locator('main [aria-label="Scan options"]:visible');
  if (await options.count()) { const b = await options.first().boundingBox(); assert(b && b.width >= 40 && b.height >= 40, `Scan options ${b?.width}x${b?.height}`); }
  const sizes = await page.locator('header.app-topbar button:visible').evaluateAll((els) => els.map((el) => { const r = el.getBoundingClientRect(); return { name: el.getAttribute('aria-label') || el.textContent.trim(), w: Math.round(r.width), h: Math.round(r.height) }; }));
  assert(sizes.length >= 3, `only ${sizes.length} topbar controls`);
  const small = sizes.filter((s) => s.w < 40 || s.h < 40);
  assert(!small.length, `small targets: ${JSON.stringify(small)}`);
  assert(sizes.some((s) => /notification/i.test(s.name)), `no notifications bell in ${sizes.map((s) => s.name).join(',')}`);
});

// 13 · Phone tables become cards.
await check('phone: workspaces table renders as cards', async () => {
  await page.setViewportSize({ width: 390, height: 844 });
  await visit('/workspaces');
  const display = await page.locator('.rtable tbody tr').first().evaluate((el) => getComputedStyle(el).display);
  assert(display === 'block', `row display is ${display}`);
  const scrolls = await page.evaluate(() => document.documentElement.scrollWidth > window.innerWidth + 1);
  assert(!scrolls, 'page scrolls sideways');
});

async function assertCards(selector, label) {
  const row = page.locator(`${selector} tbody tr`).first();
  await row.waitFor({ timeout: 8000 });
  const display = await row.evaluate((el) => getComputedStyle(el).display);
  assert(display === 'block', `${label}: row display is ${display}`);
  const box = await row.boundingBox();
  assert(box && box.x >= 0 && box.x + box.width <= 391, `${label}: card overflows the viewport (${JSON.stringify(box)})`);
  const hasText = await row.evaluate((el) => el.innerText.trim().length > 0);
  assert(hasText, `${label}: card is empty`);
  const scrolls = await page.evaluate(() => document.documentElement.scrollWidth > window.innerWidth + 1);
  assert(!scrolls, `${label}: page scrolls sideways`);
}
await check('phone: search results render as cards', async () => {
  await page.setViewportSize({ width: 390, height: 844 });
  await visit('/search?q=a');
  const tableToggle = page.getByRole('button', { name: /table view/i });
  if (await tableToggle.count()) await tableToggle.first().click();
  await assertCards('table.search-results', 'search');
});
await check('phone: scan findings render as cards with labelled fields', async () => {
  await page.setViewportSize({ width: 390, height: 844 });
  await visit(`/scans/${scanId}`);
  await assertCards('.findings-table table', 'findings');
  const labels = await page.locator('.findings-table tbody tr').first().locator('td[data-label]').evaluateAll((tds) => tds.map((td) => td.dataset.label));
  assert(['File', 'Tool', 'Status'].every((l) => labels.includes(l)), `labels: ${labels.join(',')}`);
});
await check('phone: workspace profile picker keeps its options on one row', async () => {
  await page.setViewportSize({ width: 390, height: 844 });
  await visit(`/workspaces/${wsId}`);
  const tops = await page.locator('.profile-picker.segmented > button').evaluateAll((bs) => bs.map((b) => Math.round(b.getBoundingClientRect().top)));
  assert(tops.length === 4, `found ${tops.length} profile options`);
  assert(new Set(tops).size === 1, `options wrap across rows: ${tops.join(',')}`);
  const pressed = page.locator('.profile-picker.segmented > button', { hasText: 'deep' });
  await pressed.click();
  assert((await pressed.getAttribute('aria-pressed')) === 'true', 'tapping "deep" did not select it');
});

// Deep-audit regressions (fix/deep-audit).
await check('add workspace: missing folder shows a plain-language inline error', async () => {
  await page.setViewportSize({ width: 1280, height: 800 });
  await visit('/workspaces');
  const mark = consoleErrors.length;
  await page.getByRole('button', { name: /^Add workspace$/ }).first().click();
  const dialog = page.getByRole('dialog');
  await dialog.getByLabel(/Folder path/).fill(join(tmpdir(), 'bluntcode-smoke-does-not-exist'));
  await dialog.getByLabel(/Workspace name/).fill('nope');
  await dialog.getByRole('button', { name: /^Add workspace$/ }).click();
  await dialog.getByText(/does not exist/i).first().waitFor({ timeout: 5000 });
  const text = await dialog.innerText();
  assert(!/stat |no such file|lstat|errno/i.test(text), 'raw Go error leaked into the dialog');
  await page.keyboard.press('Escape');
  // The browser logs the deliberate 400 as a resource error; it is the point of this check.
  consoleErrors.splice(mark, consoleErrors.length - mark, ...consoleErrors.slice(mark).filter((e) => !/status of 400/.test(e)));
});
await check('phone: finding detail opens as a sheet in view, not below the list', async () => {
  await page.setViewportSize({ width: 390, height: 844 });
  await visit(`/scans/${scanId}`);
  await page.locator('.findings-table tbody tr').first().click();
  const pane = page.locator('aside.source-pane');
  await pane.waitFor({ timeout: 5000 });
  const box = await pane.boundingBox();
  assert(box && box.y < 844 && box.y + 40 > 0, `sheet top at ${box?.y}px, outside the viewport`);
  assert(box.x >= -1 && box.x + box.width <= 391, `sheet overflows horizontally (${box.x}+${box.width})`);
  await page.keyboard.press('Escape');
});
await check('suppression lowers the scan counts, unsuppress restores them', async () => {
  const before = await api('GET', `/scans/${scanId}`);
  const { items } = await api('GET', `/scans/${scanId}/findings?limit=25`);
  const target = items.find((f) => f.status !== 'suppressed') ?? items[0];
  await api('POST', `/workspaces/${wsId}/suppressions`, { fingerprint: target.fingerprint, reason: 'smoke: accepted risk' });
  const after = await api('GET', `/scans/${scanId}`);
  assert(after.total_findings === before.total_findings - 1, `total ${before.total_findings} -> ${after.total_findings}`);
  await api('DELETE', `/workspaces/${wsId}/suppressions/${target.fingerprint}`);
  const restored = await api('GET', `/scans/${scanId}`);
  assert(restored.total_findings === before.total_findings, `restore ${restored.total_findings} != ${before.total_findings}`);
});
await check('search defaults to each workspace\'s latest scan (no duplicates)', async () => {
  const latest = await api('GET', '/findings/search?limit=100');
  const all = await api('GET', '/findings/search?limit=100&scope=all');
  const keys = (latest.items ?? []).map((f) => `${f.workspace_id}|${f.fingerprint}`);
  assert(new Set(keys).size === keys.length, 'duplicate findings across scans of one workspace');
  assert((all.total ?? all.items?.length ?? 0) >= (latest.total ?? latest.items?.length ?? 0), 'scope=all returned fewer results than the default');
});

await check('no console errors across the run', async () => {
  assert(!consoleErrors.length, `${consoleErrors.length}: ${consoleErrors.slice(0, 3).join(' || ')}`);
});

// Cleanup: remove the throwaway workspace (project files are untouched).
if (wsId && !process.env.SMOKE_KEEP) { try { await api('DELETE', `/workspaces/${wsId}`); } catch (e) { results.push(`WARN  cleanup: ${e.message}`); } }
await browser.close();
console.log(results.join('\n'));
console.log(`\n${results.filter((r) => r.startsWith('PASS')).length} passed, ${failed} failed`);
process.exitCode = failed ? 1 : 0;
