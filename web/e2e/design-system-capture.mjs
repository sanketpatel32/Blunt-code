/**
 * Multi-page synthetic UI capture for design-system before/after evidence.
 *
 * Requires loopback-harness with -demo-seed -i-understand-dev-only -data <tmpdir>.
 *
 *   LABEL=v0.49.1 BASE=http://127.0.0.1:9877 PRESET=large node e2e/design-system-capture.mjs
 *
 * Env:
 *   BASE, OUT, LABEL (filename prefix), PRESET (large|small), DEMO_TOKEN, REDUCED_MOTION=1
 */
import { mkdirSync } from 'node:fs';
import { join } from 'node:path';
import { chromium, devices } from 'playwright';

const BASE = process.env.BASE ?? 'http://127.0.0.1:9876';
const OUT = process.env.OUT ?? '/opt/cursor/artifacts/design-system-capture';
const LABEL = process.env.LABEL ?? 'build';
const PRESET = process.env.PRESET ?? 'large';
const TOKEN = process.env.DEMO_TOKEN ?? '';
const widths = [1280, 1440, 1920];

async function seed(page, preset) {
  const headers = { 'Content-Type': 'application/json' };
  if (TOKEN) headers['X-Bluntcode-Demo-Token'] = TOKEN;
  const res = await page.request.post(`${BASE}/api/v1/system/seed-demo`, {
    headers,
    data: JSON.stringify({ preset }),
  });
  if (!res.ok()) {
    const body = await res.text();
    throw new Error(`seed-demo failed: ${res.status()} ${body.slice(0, 400)}`);
  }
  return res.json();
}

async function firstWorkspaceId(page) {
  const res = await page.request.get(`${BASE}/api/v1/workspaces?page=1&page_size=1`);
  const j = await res.json();
  return j.items?.[0]?.id;
}

const routes = [
  { key: 'overview', path: '/' },
  { key: 'workspaces', path: '/workspaces' },
  { key: 'findings', path: '/findings' },
  { key: 'tools', path: '/tools' },
  { key: 'settings', path: '/settings' },
];

async function capture(page, key, path, width, suffix = '') {
  await page.setViewportSize({ width, height: 900 });
  await page.goto(`${BASE}${path}`, { waitUntil: 'networkidle', timeout: 90_000 });
  await page.waitForTimeout(350);
  const file = `${LABEL}-${key}${suffix}-${width}.png`;
  const outPath = join(OUT, file);
  await page.screenshot({ path: outPath, fullPage: true });
  console.log('wrote', outPath);
}

async function main() {
  mkdirSync(OUT, { recursive: true });
  const reduced = process.env.REDUCED_MOTION === '1';
  const browser = await chromium.launch();
  const context = await browser.newContext(
    reduced ? { ...devices['Desktop Chrome'], reducedMotion: 'reduce' } : {},
  );
  const page = await context.newPage();

  if (PRESET !== 'none') {
    const summary = await seed(page, PRESET);
    console.log('seed', PRESET, 'workspaces', summary.workspaces ?? summary);
  }

  const wsId = await firstWorkspaceId(page);
  if (wsId) {
    routes.push({ key: 'history', path: `/workspaces/${wsId}/scans` });
  }

  for (const w of widths) {
    for (const { key, path } of routes) {
      await capture(page, key, path, w, reduced ? '-reduced-motion' : '');
    }
  }

  if (PRESET === 'large') {
    const ledger = page.locator('section.board-ledger');
    await page.goto(`${BASE}/`, { waitUntil: 'networkidle' });
    const next = ledger.getByRole('button', { name: 'Next' });
    if (await next.isEnabled()) {
      await next.click();
      await page.waitForTimeout(400);
      for (const w of widths) {
        await page.setViewportSize({ width: w, height: 900 });
        const p = join(OUT, `${LABEL}-overview-ledger-p2-${w}.png`);
        await page.screenshot({ path: p, fullPage: true });
        console.log('wrote', p);
      }
    }
  }

  await browser.close();
}

main().catch((e) => {
  console.error(e);
  process.exit(1);
});
