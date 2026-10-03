// Side-by-side before/after plates.
//
// Reads the STORED captures in .playwright-cli/{before,after} and crops them
// with CSS (a fixed window with overflow hidden over a negatively-offset img),
// so "before" really is the before build. node .playwright-cli/shots.cjs has to
// have been run against each build for this to mean anything.
//
//   node .playwright-cli/compare.cjs <label> <route> <x> <y> <w> <h> [theme]
const PW = 'C:/Users/sanpa/AppData/Roaming/npm/node_modules/@playwright/mcp/node_modules/playwright';
const { chromium } = require(PW);
const EXE = 'C:/Users/sanpa/AppData/Local/ms-playwright/chromium-1243/chrome-win64/chrome.exe';
const fs = require('fs');
const path = require('path');

const [, , label, route, X, Y, W, H, themeArg] = process.argv;
const theme = themeArg || 'light';
const clip = { x: +X, y: +Y, w: +W, h: +H };
const OUT = '.playwright-cli/compare';
fs.mkdirSync(OUT, { recursive: true });

const b64 = (rel) => {
  const p = path.join('.playwright-cli', rel);
  if (!fs.existsSync(p)) throw new Error('missing capture: ' + p);
  return 'data:image/png;base64,' + fs.readFileSync(p).toString('base64');
};

(async () => {
  const before = b64(path.join('before', `${route}-${theme}.png`));
  const after = b64(path.join('after', `${route}-${theme}.png`));

  const html = `<!doctype html><meta charset=utf-8>
<style>
  body{margin:0;background:#0b0d11;font:13px/1.4 ui-sans-serif,system-ui,"Segoe UI",sans-serif;color:#e6e9ef;padding:22px}
  h1{font-size:15px;margin:0 0 16px;letter-spacing:.02em;font-weight:700}
  h1 small{color:#8b93a3;font-weight:400;margin-left:8px}
  .row{display:grid;grid-template-columns:1fr 1fr;gap:16px;align-items:start}
  figure{margin:0}
  figcaption{font:700 11px/1 ui-monospace,Consolas,monospace;letter-spacing:.16em;
             text-transform:uppercase;margin-bottom:8px}
  .before figcaption{color:#f2777a}
  .after figcaption{color:#5ec98a}
  /* The crop: a window of exactly clip.w x clip.h with the full-page capture
     pushed up and left by clip.x / clip.y underneath it. */
  .win{width:${clip.w}px;height:${clip.h}px;overflow:hidden;border-radius:8px;
       border:1px solid #2b313d;position:relative;background:#fff}
  .win img{position:absolute;left:${-clip.x}px;top:${-clip.y}px;max-width:none}
</style>
<h1>${label}<small>${route} &middot; ${theme}</small></h1>
<div class=row>
  <figure class=before><figcaption>before</figcaption>
    <div class=win><img src="${before}"></div></figure>
  <figure class=after><figcaption>after</figcaption>
    <div class=win><img src="${after}"></div></figure>
</div>`;

  const file = path.join(OUT, `${label}.html`);
  fs.writeFileSync(file, html);

  const b = await chromium.launch({ executablePath: EXE });
  const c = await b.newContext({ viewport: { width: clip.w * 2 + 90, height: clip.h + 140 }, deviceScaleFactor: 1.5 });
  const p = await c.newPage();
  await p.goto('file:///' + path.resolve(file).replace(/\\/g, '/'), { waitUntil: 'load' });
  await p.waitForTimeout(500);
  await p.screenshot({ path: path.join(OUT, `${label}.png`), fullPage: true });
  await b.close();
  console.log(path.join(OUT, `${label}.png`));
})();
