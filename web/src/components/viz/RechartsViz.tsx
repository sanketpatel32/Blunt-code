/**
 * Every Recharts chart in the app, in one module so Vite emits a single
 * `chunk-recharts` that only chart routes download (see Charts.tsx for the
 * lazy door and vite.config.ts for the chunk rule). Colours come from the
 * shared --sev-fill-* / --grade-* tokens, so charts match every other surface.
 */
import { Area, AreaChart, Bar, BarChart, Cell, Pie, PieChart, PolarAngleAxis, RadialBar, RadialBarChart, ResponsiveContainer, Tooltip, XAxis, YAxis, CartesianGrid } from 'recharts';
import { useReducedMotion } from 'motion/react';
import type { Severity } from '../../types';
import { SEVERITY_COLOR, SEVERITY_ORDER } from '../../lib/chartData';

const tooltipStyle = {
  contentStyle: { background: 'var(--color-surface)', border: '1px solid var(--color-rule)', borderRadius: 10, boxShadow: 'var(--shadow-card)', fontSize: 12, padding: '6px 10px', color: 'var(--color-ink)' },
  labelStyle: { color: 'var(--color-ink-soft)', fontWeight: 600, marginBottom: 2 },
  itemStyle: { padding: 0, color: 'var(--color-ink)' },
  cursor: { stroke: 'var(--color-rule-strong)', strokeDasharray: '3 3' },
};

export type TrendDatum = { label: string; total: number; critical?: number; high?: number; medium?: number; low?: number; info?: number; grade?: string; score?: number };

const shortDate = (value: string) => {
  const d = new Date(value);
  return Number.isNaN(d.getTime()) ? value : d.toLocaleDateString(undefined, { month: 'short', day: 'numeric' });
};

/** Stacked severity area over completed scans, oldest → newest. */
export function TrendArea({ data, height = 180, stacked = true }: { data: TrendDatum[]; height?: number; stacked?: boolean }) {
  const reduced = useReducedMotion();
  const bands = SEVERITY_ORDER.filter((sev) => data.some((d) => (d[sev] ?? 0) > 0));
  const label = `Findings over ${data.length} completed scans: ${data.map((d) => d.total).join(', ')}`;
  return (
    <div className="viz-chart" role="img" aria-label={label} style={{ height }}>
      <ResponsiveContainer width="100%" height="100%">
        <AreaChart data={data} margin={{ top: 8, right: 8, bottom: 0, left: -18 }}>
          <defs>
            <linearGradient id="viz-total" x1="0" y1="0" x2="0" y2="1">
              <stop offset="0%" stopColor="var(--color-accent)" stopOpacity={0.28} />
              <stop offset="100%" stopColor="var(--color-accent)" stopOpacity={0.02} />
            </linearGradient>
          </defs>
          <CartesianGrid vertical={false} stroke="var(--color-rule-faint)" />
          <XAxis dataKey="label" tickFormatter={shortDate} tick={{ fontSize: 11, fill: 'var(--color-ink-faint)' }} axisLine={false} tickLine={false} minTickGap={24} />
          <YAxis allowDecimals={false} tick={{ fontSize: 11, fill: 'var(--color-ink-faint)' }} axisLine={false} tickLine={false} width={40} />
          <Tooltip {...tooltipStyle} labelFormatter={(v) => shortDate(String(v))} />
          {stacked && bands.length > 0
            ? bands.map((sev) => (
                <Area key={sev} type="monotone" dataKey={sev} name={sev} stackId="s" stroke={SEVERITY_COLOR[sev]} fill={SEVERITY_COLOR[sev]} fillOpacity={0.35} strokeWidth={1.5} isAnimationActive={!reduced} animationDuration={700} />
              ))
            : <Area type="monotone" dataKey="total" name="findings" stroke="var(--color-accent)" fill="url(#viz-total)" strokeWidth={2} isAnimationActive={!reduced} animationDuration={700} dot={{ r: 3, strokeWidth: 0, fill: 'var(--color-accent)' }} />}
        </AreaChart>
      </ResponsiveContainer>
    </div>
  );
}

/** Severity donut with the total in the middle. */
export function SeverityDonut({ counts, size = 148, onSelect }: { counts: Partial<Record<Severity, number>>; size?: number; onSelect?: (sev: Severity) => void }) {
  const reduced = useReducedMotion();
  const rows = SEVERITY_ORDER.map((sev) => ({ sev, value: counts[sev] ?? 0 })).filter((r) => r.value > 0);
  const total = rows.reduce((s, r) => s + r.value, 0);
  const label = `Severity breakdown: ${SEVERITY_ORDER.map((k) => `${counts[k] ?? 0} ${k}`).join(', ')}`;
  return (
    <div className="viz-donut" role="img" aria-label={label} style={{ width: size, height: size }}>
      <ResponsiveContainer width="100%" height="100%">
        <PieChart>
          <Tooltip {...tooltipStyle} cursor={false} />
          <Pie data={rows.length ? rows : [{ sev: 'none', value: 1 }]} dataKey="value" nameKey="sev" innerRadius="66%" outerRadius="100%" paddingAngle={rows.length > 1 ? 2 : 0} stroke="none" isAnimationActive={!reduced} animationDuration={700} onClick={(d) => onSelect && rows.length && onSelect((d as unknown as { sev: Severity }).sev)}>
            {(rows.length ? rows : [{ sev: 'none' as const, value: 1 }]).map((r) => (
              <Cell key={r.sev} fill={r.sev === 'none' ? 'var(--color-rule)' : SEVERITY_COLOR[r.sev as Severity]} style={{ cursor: onSelect ? 'pointer' : undefined, outline: 'none' }} />
            ))}
          </Pie>
        </PieChart>
      </ResponsiveContainer>
      <span className="viz-donut-center" aria-hidden="true"><b>{total}</b></span>
    </div>
  );
}

const GRADE_TONE: Record<string, string> = { A: 'var(--grade-a)', B: 'var(--grade-b)', C: 'var(--grade-c)', D: 'var(--grade-d)' };

/** Radial gauge for the weighted risk score; the arc fills toward D (50+). */
export function GradeGauge({ score, grade, size = 132 }: { score: number; grade: string; size?: number }) {
  const reduced = useReducedMotion();
  const pct = Math.min(100, (score / 60) * 100);
  return (
    <div className="viz-gauge" role="img" aria-label={`Risk grade ${grade}, score ${Math.round(score)}`} style={{ width: size, height: size }}>
      <ResponsiveContainer width="100%" height="100%">
        <RadialBarChart data={[{ v: Math.max(pct, 2) }]} innerRadius="78%" outerRadius="100%" startAngle={220} endAngle={-40} barSize={10}>
          <PolarAngleAxis type="number" domain={[0, 100]} tick={false} axisLine={false} />
          <RadialBar dataKey="v" cornerRadius={8} background={{ fill: 'var(--color-rule-faint)' }} fill={GRADE_TONE[grade] ?? 'var(--color-rule)'} isAnimationActive={!reduced} animationDuration={800} />
        </RadialBarChart>
      </ResponsiveContainer>
      <span className="viz-gauge-center" aria-hidden="true"><b data-grade={grade}>{grade}</b><small>{Math.round(score)}</small></span>
    </div>
  );
}

/** Horizontal bars for a facet (category, analyzer, language). */
export function CategoryBars({ items, height, onSelect, tone = 'var(--color-accent)' }: { items: Array<{ name: string; value: number }>; height?: number; onSelect?: (name: string) => void; tone?: string }) {
  const reduced = useReducedMotion();
  const rows = items.slice(0, 8);
  const h = height ?? Math.max(72, rows.length * 30 + 8);
  const label = `Counts: ${rows.map((r) => `${r.name} ${r.value}`).join(', ')}`;
  return (
    <div className="viz-chart" role="img" aria-label={label} style={{ height: h }}>
      <ResponsiveContainer width="100%" height="100%">
        <BarChart data={rows} layout="vertical" margin={{ top: 0, right: 28, bottom: 0, left: 0 }} barCategoryGap={6}>
          <XAxis type="number" hide allowDecimals={false} />
          <YAxis type="category" dataKey="name" width={96} tick={{ fontSize: 12, fill: 'var(--color-ink-soft)' }} axisLine={false} tickLine={false} />
          <Tooltip {...tooltipStyle} cursor={{ fill: 'var(--color-surface-muted)' }} />
          <Bar dataKey="value" name="findings" fill={tone} radius={[0, 6, 6, 0]} isAnimationActive={!reduced} animationDuration={600} label={{ position: 'right', fontSize: 11, fill: 'var(--color-ink-soft)' }} onClick={(d) => onSelect?.((d as unknown as { name: string }).name)} style={{ cursor: onSelect ? 'pointer' : undefined }} />
        </BarChart>
      </ResponsiveContainer>
    </div>
  );
}
