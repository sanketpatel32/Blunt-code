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
await check('rules editor renders and validates', async () => {
  await visit('/rules');
  await page.getByText(/Draft validation/i).first().waitFor();
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
const routes = ['/', '/workspaces', `/workspaces/${wsId}`, `/workspaces/${wsId}/files`, `/workspaces/${wsId}/scans`, `/workspaces/${wsId}/pentest`, `/scans/${scanId}`, '/search', '/tools', '/rules', '/settings', '/about', '/cli'];
for (const viewport of [{ width: 1280, height: 800 }, { width: 390, height: 844 }]) {
  await check(`every route renders at ${viewport.width}px`, async () => {
    await page.setViewportSize(viewport);
    for (const route of routes) await visit(route);
  });
}

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

await check('no console errors across the run', async () => {
  assert(!consoleErrors.length, `${consoleErrors.length}: ${consoleErrors.slice(0, 3).join(' || ')}`);
});

// Cleanup: remove the throwaway workspace (project files are untouched).
if (wsId && !process.env.SMOKE_KEEP) { try { await api('DELETE', `/workspaces/${wsId}`); } catch (e) { results.push(`WARN  cleanup: ${e.message}`); } }
await browser.close();
console.log(results.join('\n'));
console.log(`\n${results.filter((r) => r.startsWith('PASS')).length} passed, ${failed} failed`);
process.exitCode = failed ? 1 : 0;
