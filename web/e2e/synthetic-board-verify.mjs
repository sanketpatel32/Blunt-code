/**
 * Opt-in synthetic UI + API check for the home ledger (Playwright).
 *
 * Requires a RUNNING loopback API on 127.0.0.1 with:
 *   - Production: BLUNTCODE_DEMO_SEED=1 (optional BLUNTCODE_DEMO_TOKEN)
 *   - Linux dev: cmd/loopback-harness -i-understand-dev-only -demo-seed -data <tmpdir> [-static-dir web/dist]
 *
 * This validates that the ledger renders the full server page (25 rows) matching
 * the footer range, that paging advances without skipping workspace ids, and
 * captures multi-width screenshots. It does NOT run Windows analyzers — only API +
 * bundled UI against seeded SQLite data.
 *
 * Usage:
 *   BASE=http://127.0.0.1:9876 DEMO_TOKEN=secret node e2e/synthetic-board-verify.mjs
 *
 * Env:
 *   BASE       — UI + API origin (default http://127.0.0.1:9876)
 *   DEMO_TOKEN — optional, must match server BLUNTCODE_DEMO_TOKEN
 *   OUT        — screenshot directory (default /opt/cursor/artifacts/bluntcode-synthetic-verify)
 *   SKIP_SHOTS — if set to 1, skip PNG writes (assertions only)
 */
import { mkdirSync, existsSync } from 'node:fs';
import { join } from 'node:path';
import { chromium } from 'playwright';

const BASE = process.env.BASE ?? 'http://127.0.0.1:9876';
const TOKEN = process.env.DEMO_TOKEN ?? '';
const OUT = process.env.OUT ?? '/opt/cursor/artifacts/bluntcode-synthetic-verify';
const SKIP_SHOTS = process.env.SKIP_SHOTS === '1';

const widths = [1280, 1440, 1920];
const PAGE_SIZE = 25;

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
  return res.json();
}

async function statsTotal(page) {
  const res = await page.request.get(`${BASE}/api/v1/stats`);
  if (!res.ok()) throw new Error(`stats: ${res.status()}`);
  const j = await res.json();
  return typeof j.workspaces === 'number' ? j.workspaces : 0;
}

/** Ledger panel only — avoids strict-mode collisions with activity-feed Previous/Next. */
function ledgerPanel(host) {
  return host.locator('section.board-ledger');
}

function rowIds(host) {
  return ledgerPanel(host).locator('.ledger-list .ledger-row[data-workspace-id]');
}

const ND = '\u2013'; // en dash, matches UI "Showing 1–25 of …"

async function assertPage(host, { pageNum, total, expectedStart, prevDisabled, nextDisabled }) {
  const rangeEnd = Math.min(expectedStart + PAGE_SIZE - 1, total);
  const rangeRe = new RegExp(`Showing ${expectedStart}${ND}${rangeEnd} of ${total} by risk`);
  await ledgerPanel(host).getByText(rangeRe).waitFor({ state: 'visible', timeout: 30_000 });
  const count = await rowIds(host).count();
  const wantRows = rangeEnd - expectedStart + 1;
  if (count !== wantRows) {
    throw new Error(`page ${pageNum}: want ${wantRows} ledger rows (${expectedStart}-${rangeEnd}), got ${count}`);
  }
  const prev = ledgerPanel(host).getByRole('button', { name: 'Previous' });
  const next = ledgerPanel(host).getByRole('button', { name: 'Next' });
  if ((await prev.isDisabled()) !== prevDisabled) {
    throw new Error(`Previous disabled want ${prevDisabled}`);
  }
  if ((await next.isDisabled()) !== nextDisabled) {
    throw new Error(`Next disabled want ${nextDisabled}`);
  }
}

async function shot(host, name, width) {
  if (SKIP_SHOTS) return;
  mkdirSync(OUT, { recursive: true });
  await host.setViewportSize({ width, height: 900 });
  const path = join(OUT, `${name}-${width}.png`);
  await host.screenshot({ path, fullPage: true });
  console.log('wrote', path);
}

async function main() {
  const browser = await chromium.launch();
  const context = await browser.newContext();
  const host = await context.newPage();

  let seedSummary;
  try {
    seedSummary = await seed(host);
  } catch (e) {
    console.error(String(e.message ?? e));
    console.error('Start the server with BLUNTCODE_DEMO_SEED=1 and an isolated data directory.');
    process.exit(1);
  }
  console.log('seed-demo ok:', JSON.stringify(seedSummary));

  const total = await statsTotal(host);
  if (total < 100) {
    throw new Error(`expected large preset total workspaces >= 100, got ${total}`);
  }

  await host.goto(`${BASE}/`, { waitUntil: 'networkidle', timeout: 120_000 });

  const page1Ids = await rowIds(host).evaluateAll((els) => els.map((el) => el.getAttribute('data-workspace-id')));
  if (page1Ids.length !== PAGE_SIZE) {
    throw new Error(`page 1 want ${PAGE_SIZE} ledger rows, got ${page1Ids.length}`);
  }
  await assertPage(host, {
    pageNum: 1,
    total,
    expectedStart: 1,
    prevDisabled: true,
    nextDisabled: total <= PAGE_SIZE,
  });

  const next = ledgerPanel(host).getByRole('button', { name: 'Next' });
  await next.click();
  await host.waitForTimeout(300);

  const page2Ids = await rowIds(host).evaluateAll((els) => els.map((el) => el.getAttribute('data-workspace-id')));
  if (page2Ids.length !== PAGE_SIZE) {
    throw new Error(`page 2 want ${PAGE_SIZE} ledger rows, got ${page2Ids.length}`);
  }
  const overlap = page1Ids.filter((id) => page2Ids.includes(id));
  if (overlap.length > 0) {
    throw new Error(`page overlap: ${overlap.slice(0, 5).join(',')}`);
  }
  await assertPage(host, {
    pageNum: 2,
    total,
    expectedStart: PAGE_SIZE + 1,
    prevDisabled: false,
    nextDisabled: total <= 2 * PAGE_SIZE,
  });

  for (const w of widths) {
    await shot(host, 'overview-ledger-p2', w);
  }

  await browser.close();
  if (!SKIP_SHOTS && !existsSync(join(OUT, `overview-ledger-p2-${widths[0]}.png`))) {
    process.exit(1);
  }
  console.log('synthetic-board-verify: OK (UI/API only; not Windows analyzers)');
}

main().catch((e) => {
  console.error(e);
  process.exit(1);
});
