import { TrendArea, SeverityDonut } from './viz/Charts';
import type { LanguageCoverage, SeverityCounts, TrendPoint } from '../lib/chartData';
import { SEVERITY_COLOR, SEVERITY_ORDER } from '../lib/chartData';
import type { SeverityTrendPoint } from '../types';
import type { TrendDatum } from './viz/Charts';
import { BarChart3, Code2, PieChart } from 'lucide-react';

type Props = {
  /** Per-scan severity series from /trends (completed scans only, oldest first). */
  series?: SeverityTrendPoint[];
  /** Fallback totals when the series is unavailable. */
  trends?: TrendPoint[];
  severityCounts?: SeverityCounts;
  languages?: LanguageCoverage[];
};

function TrendLegend({ data }: { data: TrendDatum[] }) {
  const bands = SEVERITY_ORDER.filter((sev) => data.some((d) => (d[sev] ?? 0) > 0));
  return (
    <ul className="viz-legend" aria-hidden="true">
      {bands.map((sev) => <li key={sev}><i style={{ background: SEVERITY_COLOR[sev] }} />{sev}</li>)}
    </ul>
  );
}

function Card({ children, label, icon, note }: { children: React.ReactNode; label: string; icon: React.ReactNode; note?: React.ReactNode }) {
  return (
    <article aria-label={label} className="viz-card">
      <header className="viz-card-head">
        <h3>{icon}{label}</h3>
        {note != null && <span className="viz-card-chip tabular-nums">{note}</span>}
      </header>
      {children}
    </article>
  );
}

// 3) Language coverage bar — horizontal bars
function LanguageBars({ items }: { items: LanguageCoverage[] }) {
  if (!items.length) {
    return <p className="text-sm text-[var(--color-ink-faint)]">No language data yet.</p>;
  }
  const max = Math.max(...items.map((i) => i.files), 1);
  const a11y = `Language coverage: ${items.map((i) => `${i.language} ${i.files} ${i.files === 1 ? 'file' : 'files'}`).join(', ')}`;
  return (
    <div className="grid gap-2.5" role="img" aria-label={a11y}>
      {items.map((row) => (
        <div key={row.language} className="grid gap-1">
          <div className="flex items-center justify-between gap-3 text-xs">
            <span className="truncate font-medium text-[var(--color-ink)]">{row.language}</span>
            <span className="shrink-0 tabular-nums text-[var(--color-ink-faint)]">{row.files} {row.files === 1 ? 'file' : 'files'}</span>
          </div>
          <div className="h-2.5 overflow-hidden rounded-full bg-[var(--color-surface-muted)]">
            <div className="h-full rounded-full bg-[var(--color-accent)]" style={{ width: `${Math.round((row.files * 1000) / max) / 10}%` }} aria-hidden="true" />
          </div>
        </div>
      ))}
    </div>
  );
}

export function AnalyticsCharts({ series, trends, severityCounts, languages }: Props) {
  const counts: SeverityCounts = severityCounts ?? { critical: 0, high: 0, medium: 0, low: 0, info: 0 };
  const langs = languages ?? [];
  const data: TrendDatum[] = series?.length
    ? series.map((p, i) => ({ label: p.finished_at ?? `T${i + 1}`, total: p.total, ...p.severity, score: p.score, grade: p.grade }))
    : (trends ?? []).map((p) => ({ label: p.label, total: p.total }));
  const severityBands = SEVERITY_ORDER.filter((key) => (counts[key] ?? 0) > 0).length;

  return (
    <section aria-label="Analytics overview" className="bento">
      <div className="span-8">
        <Card label="Findings over time" icon={<BarChart3 aria-hidden="true" />} note={data.length ? `${data.length} scans` : undefined}>
          {data.length >= 2
            ? <><TrendArea data={data} stacked={!!series?.length} height={200} />{series?.length ? <TrendLegend data={data} /> : null}</>
            : <p className="viz-empty">{data.length === 1 ? 'One completed scan so far' : 'No completed scans yet'}</p>}
        </Card>
      </div>
      <div className="span-4">
        <Card label="Severity" icon={<PieChart aria-hidden="true" />} note={severityBands ? `${severityBands}/${SEVERITY_ORDER.length}` : undefined}>
          {severityBands ? <div className="viz-center"><SeverityDonut counts={counts} size={164} /></div> : <p className="viz-empty">No findings</p>}
        </Card>
      </div>
      <div className="span-12">
        <Card label="Languages" icon={<Code2 aria-hidden="true" />} note={langs.length || undefined}>
          <LanguageBars items={langs} />
        </Card>
      </div>
    </section>
  );
}
