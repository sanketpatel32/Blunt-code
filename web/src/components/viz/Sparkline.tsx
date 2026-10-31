/** Light SVG sparkline for cards and rows; no chart library. Fewer than two
 *  points draws nothing — one number is not a trend. */
export function Sparkline({ values, width = 96, height = 28, label, tone = 'var(--color-accent)', className }: { values: number[]; width?: number; height?: number; label?: string; tone?: string; className?: string }) {
  if (values.length < 2) return null;
  const max = Math.max(...values, 1);
  const min = Math.min(...values, 0);
  const range = Math.max(max - min, 1);
  const step = (width - 4) / (values.length - 1);
  const pts = values.map((v, i) => [2 + i * step, height - 3 - ((v - min) / range) * (height - 6)] as const);
  const line = pts.map(([x, y]) => `${x.toFixed(1)},${y.toFixed(1)}`).join(' ');
  const area = `2,${height} ${line} ${pts[pts.length - 1][0].toFixed(1)},${height}`;
  const [lx, ly] = pts[pts.length - 1];
  return (
    <svg className={`viz-spark ${className ?? ''}`} viewBox={`0 0 ${width} ${height}`} width={width} height={height} role="img" aria-label={label ?? `Trend: ${values.join(', ')}`}>
      <polygon points={area} fill={tone} opacity={0.12} />
      <polyline points={line} fill="none" stroke={tone} strokeWidth={1.75} strokeLinejoin="round" strokeLinecap="round" />
      <circle cx={lx} cy={ly} r={2.4} fill={tone} />
    </svg>
  );
}
