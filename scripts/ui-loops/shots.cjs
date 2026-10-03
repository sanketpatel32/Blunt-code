// Full-app screenshot sweep. Captures every route in light and dark at desktop
// width so before/after can be compared by eye.
//
//   node .playwright-cli/shots.cjs <outDir> [baseUrl]
const PW = 'C:/Users/sanpa/AppData/Roaming/npm/node_modules/@playwright/mcp/node_modules/playwright';
const { chromium } = require(PW);
const EXE = 'C:/Users/sanpa/AppData/Local/ms-playwright/chromium-1243/chrome-win64/chrome.exe';
const fs = require('fs');

const BASE = process.argv[3] || 'http://127.0.0.1:8787';
const OUT = process.argv[2] || '.playwright-cli/shots';
const WS = 'e02dcf05-e6c9-3a0b-aec0-09bd5c6c0831';
const SCAN = 'fbbfebf7-cd3f-f751-640f-f567ad6cfd5c';

const ROUTES = [
  ['/', 'home'],
  [`/workspaces/${WS}`, 'workspace'],
  [`/scans/${SCAN}`, 'report'],
  ['/workspaces', 'workspaces'],
  [`/workspaces/${WS}/scans`, 'history'],
  [`/workspaces/${WS}/files`, 'files'],
  ['/search', 'search'],
  ['/tools', 'tools'],
  ['/rules', 'rules'],
  ['/pentest', 'pentest'],
  ['/settings', 'settings'],
  ['/cli', 'cli'],
  ['/about', 'about'],
];

(async () => {
  fs.mkdirSync(OUT, { recursive: true });
  const b = await chromium.launch({ executablePath: EXE });
  for (const theme of ['light', 'dark']) {
    for (const [route, label] of ROUTES) {
      const c = await b.newContext({
        viewport: { width: 1440, height: 1000 },
        deviceScaleFactor: 1,
        colorScheme: theme === 'dark' ? 'dark' : 'light',
      });
      const p = await c.newPage();
      await p.addInitScript((t) => {
        try { localStorage.setItem('bluntcode-theme', JSON.stringify(t)); } catch {}
      }, theme);
      await p.goto(BASE + route, { waitUntil: 'networkidle', timeout: 45000 }).catch(() => {});
      await p.waitForTimeout(1800);
      await p.screenshot({ path: `${OUT}/${label}-${theme}.png`, fullPage: false });
      console.log('shot', `${label}-${theme}`);
      await c.close();
    }
  }
  await b.close();
})();
