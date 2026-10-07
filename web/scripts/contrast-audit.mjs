import { readFileSync } from 'node:fs';
/**
 * WCAG contrast audit for the Blunt Code palette.
 * Converts oklch(L% C H) -> sRGB -> relative luminance -> contrast ratio.
 * Run: node scripts/contrast-audit.mjs
 */
const ok2deg = (v) => (v * Math.PI) / 180;

// oklch -> oklab
function oklchToOklab(L, C, H) {
  const h = ok2deg(H);
  return [L, C * Math.cos(h), C * Math.sin(h)];
}
// oklab -> linear sRGB (Björn Ottosson's matrices)
function oklabToLinearSrgb(L, a, b) {
  const l_ = L + 0.3963377774 * a + 0.2158037573 * b;
  const m_ = L - 0.1055613458 * a - 0.0638541728 * b;
  const s_ = L - 0.0894841775 * a - 1.291485548 * b;
  const l = l_ * l_ * l_;
  const m = m_ * m_ * m_;
  const s = s_ * s_ * s_;
  return [
    +4.0767416621 * l - 3.3077115913 * m + 0.2309699292 * s,
    -1.2684380046 * l + 2.6097574011 * m - 0.3413193965 * s,
    -0.0041960863 * l - 0.7034186147 * m + 1.707614701 * s,
  ];
}
function oklchToLinear(L, C, H) {
  const [l, a, b] = oklchToOklab(L / 100, C, H);
  return oklabToLinearSrgb(l, a, b);
}
function luminance([r, g, b]) {
  // linear sRGB already; clamp then ITU-R BT.709 weights
  const cl = (x) => Math.min(1, Math.max(0, x));
  return 0.2126 * cl(r) + 0.7152 * cl(g) + 0.0722 * cl(b);
}
function ratio(L1, L2) {
  const a = luminance(L1);
  const b = luminance(L2);
  const hi = Math.max(a, b);
  const lo = Math.min(a, b);
  return (hi + 0.05) / (lo + 0.05);
}
const white = [1, 1, 1];
const nearWhite = oklchToLinear(99, 0.002, 280);
const nearBlack = oklchToLinear(14, 0.015, 268);

// Every pairing below is one that actually ships. Each entry names the CSS rule
// that creates it, so a regression here points straight at the culprit.
const checks = [
  // Light theme
  ['LIGHT accent bg + accent-ink          (.button.primary, .segmented-btn.active)', [56, 0.22, 268], nearWhite],
  ['LIGHT accent-strong bg + accent-ink   (.button.primary:hover)', [50, 0.24, 268], nearWhite],
  ['LIGHT danger bg + danger-ink          (.button.danger:hover)', [58, 0.22, 26], nearWhite],
  ['LIGHT danger-strong bg + on-accent    (NotificationsCenter unread badge, 10px)', [46, 0.21, 26], nearWhite],
  ['LIGHT success-strong bg + success-ink (.scan-flow li.done .flow-marker)', [50, 0.16, 152], oklchToLinear(99, 0.004, 152)],
  ['LIGHT warning bg + warning-ink        (badge fill)', [70, 0.15, 75], oklchToLinear(20, 0.02, 75)],
  // Dark theme
  ['DARK accent bg + on-accent            (.button.primary, .segmented-btn.active)', [64, 0.18, 268], nearBlack],
  ['DARK danger bg + danger-ink           (.button.danger:hover)', [64, 0.2, 24], oklchToLinear(14, 0.015, 24)],
  ['DARK danger-strong bg + on-accent     (NotificationsCenter unread badge, 10px)', [66, 0.22, 24], nearBlack],
  ['DARK success-strong bg + success-ink  (.scan-flow li.done .flow-marker)', [56, 0.16, 152], oklchToLinear(14, 0.015, 152)],
];

const design = readFileSync(new URL('../src/css/design-system.css', import.meta.url), 'utf8');
const lightBlock = design.slice(design.indexOf(':root {'), design.indexOf(":root[data-theme='dark']"));
const darkBlock = design.slice(design.indexOf(":root[data-theme='dark']"), design.indexOf('body {'));
function token(block, name) {
  const match = block.match(new RegExp(`--color-${name}:\\s*(#[0-9a-f]{3,6})`, 'i'));
  if (!match) throw new Error(`Missing design token ${name}`);
  let hex = match[1].slice(1);
  if (hex.length === 3) hex = [...hex].map((c) => c+c).join('');
  return [0,2,4].map((i) => {
    const c = parseInt(hex.slice(i,i+2),16)/255;
    return c <= .04045 ? c/12.92 : ((c+.055)/1.055)**2.4;
  });
}
let failures = 0;
function check(name, bg, fg) {
  const r = ratio(bg, fg);
  const pass = r >= 4.5;
  if (!pass) failures++;
  console.log(name.padEnd(74), r.toFixed(2).padStart(5), '  ', pass ? 'PASS' : 'FAIL');
}
console.log('pair'.padEnd(74), 'ratio   AA-normal(4.5)');
for (const [name, bg, fg] of checks) {
  const block = name.startsWith('DARK') ? darkBlock : lightBlock;
  const isAccent = name.includes('accent bg') || name.includes('accent-strong bg');
  check(name, isAccent ? token(block, name.includes('accent-strong') ? 'accent-strong' : 'accent') : oklchToLinear(...bg), isAccent ? token(block, 'accent-ink') : fg);
}
for (const [theme, block] of [['LIGHT', lightBlock], ['DARK', darkBlock]]) {
  for (const ink of ['ink','ink-soft','ink-faint','ink-ghost','accent-strong']) {
    for (const surface of ['paper','surface','surface-muted']) {
      check(`${theme} ${ink} on ${surface} (design-system.css)`, token(block, surface), token(block, ink));
    }
  }
}
if (failures) { console.error(`${failures} pairing(s) below WCAG AA.`); process.exitCode = 1; }
