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

// The palette lives in tokens.css (oklch). Every pairing below is one that
// actually ships; each names the rule that creates it.
const tokensCss = readFileSync(new URL('../src/tokens.css', import.meta.url), 'utf8');
const darkStart = tokensCss.indexOf(":root[data-theme='dark']");
const lightBlock = tokensCss.slice(tokensCss.indexOf(':root {'), darkStart);
const darkBlock = tokensCss.slice(darkStart);
function token(block, name) {
  const match = block.match(new RegExp(`--${name}:\\s*oklch\\(([\\d.]+)%\\s+([\\d.]+)\\s+([\\d.]+)\\)`));
  if (!match) throw new Error(`Missing oklch token --${name}`);
  return oklchToLinear(+match[1], +match[2], +match[3]);
}
let failures = 0;
function check(name, bg, fg, min = 4.5) {
  const r = ratio(bg, fg);
  const pass = r >= min;
  if (!pass) failures++;
  console.log(name.padEnd(64), r.toFixed(2).padStart(5), '  ', pass ? 'PASS' : 'FAIL', min !== 4.5 ? `(min ${min})` : '');
}
console.log('pair'.padEnd(64), 'ratio   AA-normal(4.5)');
for (const [theme, block] of [['LIGHT', lightBlock], ['DARK', darkBlock]]) {
  // Body text on every surface. ink-ghost is decorative (placeholders, rules) and not text-bearing.
  for (const ink of ['color-ink', 'color-ink-soft', 'color-ink-faint', 'color-accent-strong', 'color-success-text', 'color-warning-text', 'color-danger-text']) {
    for (const surface of ['color-paper', 'color-surface', 'color-surface-muted']) check(`${theme} ${ink} on ${surface}`, token(block, surface), token(block, ink));
  }
  // The one primary button: coral fill, dark ink (Button default, .button.primary).
  check(`${theme} accent-ink on accent (primary button)`, token(block, 'color-accent'), token(block, 'color-accent-ink'));
  // Grade tiles: dark ink on each grade fill (GradeTile).
  for (const g of ['a', 'b', 'c', 'd']) check(`${theme} grade-tile-ink on grade-${g} (GradeTile)`, token(block, `grade-${g}`), token(block, 'grade-tile-ink'));
}
if (failures) { console.error(`${failures} pairing(s) below WCAG AA.`); process.exitCode = 1; }
