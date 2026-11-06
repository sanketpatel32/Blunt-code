/**
 * Opt-in UI scale capture against a RUNNING Blunt Code (same contract as smoke.mjs).
 *
 * Prerequisites:
 *   - BLUNTCODE_DEMO_SEED=1 on the server process
 *   - Optional: BLUNTCODE_DEMO_TOKEN + pass DEMO_TOKEN env to this script
 *   - POST /api/v1/system/seed-demo { preset: "large" } succeeded (or seed manually)
 *
 * Usage:
 *   BASE=http://127.0.0.1:8787 DEMO_TOKEN=secret node e2e/visual-scale.mjs
 *
 * Writes PNGs under /opt/cursor/artifacts/bluntcode-scale/ (1280, 1440, 1920).
 * This is synthetic API data — not Windows native scan validation.
 */
import { mkdirSync, existsSync } from 'node:fs';
import { join } from 'node:path';
import { chromium } from 'playwright';

const BASE = process.env.BASE ?? 'http://127.0.0.1:8787';
const TOKEN = process.env.DEMO_TOKEN ?? '';
const OUT = process.env.OUT ?? '/opt/cursor/artifacts/bluntcode-scale';

const widths = [1280, 1440, 1920];

async function seed(page) {
  const headers = { 'Content-Type': 'application/json' };
  if (TOKEN) headers['X-Bluntcode-Demo-Token'] = TOKEN;
  const res = await page.request.post(`${BASE}/api/v1/system/seed-demo`, {
    headers,
    data: JSON.stringify({ preset: 'large' }),
  });
  if (!res.ok()) {
    const body = await res.text();
    throw new Error(`seed-demo failed: ${res.status()} ${body.slice(0, 500)}`);
  }
}

async function shot(page, name, width) {
  await page.setViewportSize({ width, height: 900 });
  await page.goto(`${BASE}/`, { waitUntil: 'networkidle', timeout: 60_000 });
  await page.waitForTimeout(400);
  const path = join(OUT, `${name}-${width}.png`);
  await page.screenshot({ path, fullPage: true });
  console.log('wrote', path);
}

async function main() {
  mkdirSync(OUT, { recursive: true });
  const browser = await chromium.launch();
  const context = await browser.newContext();
  const page = await context.newPage();

  try {
    await seed(page);
  } catch (e) {
    console.error(String(e.message ?? e));
    console.error('If demo seed is disabled, set BLUNTCODE_DEMO_SEED=1 on the server and retry.');
    process.exit(1);
  }

  for (const w of widths) {
    await shot(page, 'overview', w);
    await shot(page, 'workspaces', w);
  }

  await browser.close();
  if (!existsSync(join(OUT, `overview-${widths[0]}.png`))) {
    process.exit(1);
  }
}

main().catch((e) => {
  console.error(e);
  process.exit(1);
});
