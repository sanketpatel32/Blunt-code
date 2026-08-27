import { useReducedMotion } from '../hooks/useReducedMotion';
import type { Severity } from '../types';
import type { LanguageCoverage, SeverityCounts, TrendPoint } from '../lib/chartData';
import { SEVERITY_ORDER } from '../lib/chartData';

type Props = {
  trends?: TrendPoint[];
  severityCounts?: SeverityCounts;
  languages?: LanguageCoverage[];
};

const SEVERITY_COLOR: Record<Severity, string> = {
  critical: 'var(--color-danger)',
  high: 'var(--color-danger)',
  medium: 'var(--color-warning)',
  low: 'var(--color-accent)',
  info: 'var(--color-success)',
};

function Card({ children, delay, reduced, label }: { children: React.ReactNode; delay: string; reduced: boolean; label: string }) {
  return (
    <article
      aria-label={label}
      className={`flex flex-col gap-3 rounded-[var(--radius-card)] border border-[var(--color-rule-faint)] bg-[var(--color-surface)] p-4 shadow-[var(--shadow-card)] ${reduced ? '' : 'anim-fadeInUp anim-stagger'}`}
      style={reduced ? undefined : ({ animationDelay: delay, willChange: 'transform, opacity' } as React.CSSProperties)}
    >
      {children}
    </article>
  );
}

// 1) Findings over time — line + filled area
function FindingsLineArea({ data }: { data: TrendPoint[]; reduced: boolean }) {
  if (!data.length) {
    return <p className="text-sm text-[var(--color-ink-faint)]">No trend data yet. Run a few scans to see findings over time.</p>;
  }
  const totals = data.map((d) => d.total);
  const max = Math.max(...totals, 1);
  const min = Math.min(...totals);
  const range = Math.max(max - min, 1);
  const W = 320;
  const H = 96;
  const padX = 8;
  const padY = 10;
  const step = data.length > 1 ? (W - padX * 2) / (data.length - 1) : 0;
  const pts = data.map((d, i) => {
    const x = padX + i * step;
    const y = H - padY - ((d.total - min) / range) * (H - padY * 2);
    return { x, y, v: d.total };
  });
  const line = pts.map((p) => `${p.x},${p.y}`).join(' ');
  const area = `${pts[0]?.x ?? padX},${H - padY} ${line} ${pts[pts.length - 1]?.x ?? padX},${H - padY}`;
  const a11y = `Findings over time: ${data.map((d) => `${d.total}`).join(', ')}`;
  return (
    <div className="flex flex-col gap-2">
      <svg viewBox={`0 0 ${W} ${H}`} className="w-full h-[96px] overflow-visible" role="img" aria-label={a11y} preserveAspectRatio="none">
        <polygon points={area} fill="var(--color-accent-soft)" opacity={0.95} />
        <polyline points={line} fill="none" stroke="var(--color-accent)" strokeWidth={2} strokeLinejoin="round" strokeLinecap="round" />
        {pts.map((p, i) => (
          <circle key={i} cx={p.x} cy={p.y} r={3} fill="var(--color-accent)" stroke="var(--color-surface)" strokeWidth={1.2} />
        ))}
      </svg>
      <div className="flex justify-between tabular-nums text-[0.68rem] font-mono text-[var(--color-ink-faint)]" aria-hidden="true">
        <span>{min}</span>
        <span>{max} findings</span>
      </div>
    </div>
  );
}

// 2) Severity donut — hand-rolled SVG arcs
function SeverityDonut({ counts }: { counts: SeverityCounts; reduced: boolean }) {
  const total = SEVERITY_ORDER.reduce((s, k) => s + (counts[k] ?? 0), 0);
  const a11y = `Severity breakdown: ${SEVERITY_ORDER.map((k) => `${counts[k] ?? 0} ${k}`).join(', ')}${total ? `, ${total} total` : ''}`;
  if (!total) {
    return <p className="text-sm text-[var(--color-ink-faint)]">No findings to break down yet.</p>;
  }
  const cx = 60;
  const cy = 60;
  const r = 46;
  const inner = 30;
  let angle = -90;
  const segments = SEVERITY_ORDER.filter((k) => (counts[k] ?? 0) > 0).map((sev) => {
    const value = counts[sev] ?? 0;
    const sweep = (value / total) * 360;
    const start = angle;
    const end = angle + sweep;
    angle = end;
    const large = sweep > 180 ? 1 : 0;
    const rad = (deg: number) => (deg * Math.PI) / 180;
    const x1 = cx + r * Math.cos(rad(start));
    const y1 = cy + r * Math.sin(rad(start));
    const x2 = cx + r * Math.cos(rad(end));
    const y2 = cy + r * Math.sin(rad(end));
    const ix1 = cx + inner * Math.cos(rad(end));
    const iy1 = cy + inner * Math.sin(rad(end));
    const ix2 = cx + inner * Math.cos(rad(start));
    const iy2 = cy + inner * Math.sin(rad(start));
    const d = `M ${x1} ${y1} A ${r} ${r} 0 ${large} 1 ${x2} ${y2} L ${ix1} ${iy1} A ${inner} ${inner} 0 ${large} 0 ${ix2} ${iy2} Z`;
    return { sev, value, d };
  });
  return (
    <div className="flex items-center gap-4">
      <svg viewBox="0 0 120 120" width={120} height={120} role="img" aria-label={a11y} className="shrink-0">
        {segments.map((s) => (
          <path key={s.sev} d={s.d} fill={SEVERITY_COLOR[s.sev]} opacity={s.sev === 'high' ? 0.82 : s.sev === 'critical' ? 1 : 0.9} stroke="var(--color-surface)" strokeWidth={1.2} />
        ))}
        <circle cx={cx} cy={cy} r={inner - 0.5} fill="var(--color-surface)" />
        <text x={cx} y={cy - 2} textAnchor="middle" fontSize={14} fontWeight={800} fill="var(--color-ink)" className="tabular-nums" style={{ fontVariantNumeric: 'tabular-nums' }}>
          {total}
        </text>
        <text x={cx} y={cy + 12} textAnchor="middle" fontSize={7} fontWeight={600} fill="var(--color-ink-faint)" style={{ letterSpacing: '0.06em', textTransform: 'uppercase' as const }}>
          findings
        </text>
      </svg>
      <ul className="grid gap-1.5 text-xs" aria-label="Severity legend">
        {SEVERITY_ORDER.map((sev) => (
          <li key={sev} className="flex items-center gap-2 tabular-nums">
            <i aria-hidden="true" className="h-2.5 w-2.5 shrink-0 rounded-full" style={{ background: SEVERITY_COLOR[sev], opacity: sev === 'high' ? 0.82 : 1 }} />
            <span className="min-w-[4.2rem] capitalize text-[var(--color-ink-soft)]">{sev}</span>
            <span className="font-mono font-semibold text-[var(--color-ink)]">{counts[sev] ?? 0}</span>
            <span className="ml-auto inline-flex rounded-full border border-[var(--color-rule)] bg-[var(--color-surface-muted)] px-1.5 py-0.5 font-mono text-[0.68rem] leading-none text-[var(--color-ink-faint)]">{total ? `${Math.round(((counts[sev] ?? 0) * 1000) / total) / 10}%` : '0%'}</span>
          </li>
        ))}
      </ul>
    </div>
  );
}

// 3) Language coverage bar — horizontal bars
function LanguageBars({ items }: { items: LanguageCoverage[] }) {
  if (!items.length) {
    return <p className="text-sm text-[var(--color-ink-faint)]">No language data yet.</p>;
  }
  const max = Math.max(...items.map((i) => i.files), 1);
  const a11y = `Language coverage: ${items.map((i) => `${i.language} ${i.files} files`).join(', ')}`;
  return (
    <div className="grid gap-2.5" role="img" aria-label={a11y}>
      {items.map((row) => (
        <div key={row.language} className="grid gap-1">
          <div className="flex items-center justify-between gap-3 text-xs">
            <span className="truncate font-medium text-[var(--color-ink)]">{row.language}</span>
            <span className="shrink-0 font-mono tabular-nums text-[var(--color-ink-faint)]">{row.files} files</span>
          </div>
          <div className="h-2.5 overflow-hidden rounded-full bg-[var(--color-surface-muted)]">
            <div className="h-full rounded-full bg-[var(--color-accent)]" style={{ width: `${Math.round((row.files * 1000) / max) / 10}%` }} aria-hidden="true" />
          </div>
        </div>
      ))}
    </div>
  );
}

export function AnalyticsCharts({ trends, severityCounts, languages }: Props) {
  const reduced = useReducedMotion();
  const fallbackCounts: SeverityCounts = { critical: 0, high: 0, medium: 0, low: 0, info: 0 };
  const counts = severityCounts ?? fallbackCounts;
  const langs = languages ?? [];
  const lineData = trends ?? [];

  return (
    <section aria-label="Analytics overview" className="grid gap-3 md:grid-cols-3">
      <Card label="Findings over time" delay="0ms" reduced={reduced}>
        <header className="flex items-center justify-between gap-2">
          <h3 className="font-display text-sm font-semibold tracking-tight">Findings over time</h3>
          <span className="rounded-full border border-[var(--color-rule)] bg-[var(--color-surface-muted)] px-2 py-0.5 font-mono text-[0.65rem] font-semibold uppercase tracking-widest text-[var(--color-ink-faint)]">sparklines per workspace</span>
        </header>
        <FindingsLineArea data={lineData} reduced={reduced} />
        <p className="text-xs leading-relaxed text-[var(--color-ink-soft)]">Area shows total findings across recent scans.</p>
      </Card>

      <Card label="Severity breakdown" delay="40ms" reduced={reduced}>
        <header className="flex items-center justify-between gap-2">
          <h3 className="font-display text-sm font-semibold tracking-tight">Severity</h3>
          <span className="rounded-full bg-[var(--color-accent-soft)] px-2 py-0.5 font-mono text-[0.65rem] font-semibold uppercase tracking-widest text-[var(--color-accent-strong)]">donut</span>
        </header>
        <SeverityDonut counts={counts} reduced={reduced} />
      </Card>

      <Card label="Language coverage" delay="80ms" reduced={reduced}>
        <header className="flex items-center justify-between gap-2">
          <h3 className="font-display text-sm font-semibold tracking-tight">Language coverage</h3>
          <span className="rounded-full border border-[var(--color-rule)] bg-[var(--color-surface-muted)] px-2 py-0.5 font-mono text-[0.65rem] font-semibold uppercase tracking-widest text-[var(--color-ink-faint)]">files per language</span>
        </header>
        <LanguageBars items={langs} />
      </Card>
    </section>
  );
}
