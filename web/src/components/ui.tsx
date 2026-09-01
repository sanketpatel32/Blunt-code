import type { ReactNode } from 'react';
import type { Scan, Severity, Tool } from '../types';
import { languageNames, languageColor } from '../lib/format';
import { useCountUp } from '../hooks/useCountUp';
import { Card, CardContent } from './ui/card';
import { Badge } from './ui/badge';
import { Button } from './ui/button';
import { ShieldCheck, AlertTriangle } from 'lucide-react';

export function ErrorPanel({ error, retry }: { error: string; retry?: () => void }) {
  return (
    <Card className="error-panel border-[var(--color-danger)] bg-[var(--color-danger-soft)]" role="alert">
      <CardContent className="p-8 text-center">
        <div className="mx-auto mb-3 flex h-10 w-10 items-center justify-center rounded-full bg-[var(--color-danger-soft)] border border-[var(--color-danger)]/20 text-[var(--color-danger)]">
          <AlertTriangle className="h-5 w-5" />
        </div>
        <h2 className="font-display text-xl font-bold">Could not load this view</h2>
        <p className="mt-2 text-sm text-[var(--color-ink-soft)]">{error}</p>
        {retry && <Button variant="outline" size="sm" className="mt-4" onClick={retry}>Try again</Button>}
      </CardContent>
    </Card>
  );
}

export function Loading() {
  return <div className="loading" aria-live="polite">Loading…</div>;
}

/** Shared empty-state panel. `icon` renders inside a circular medallion; `tone: 'positive'` switches the medallion to the success tokens (used for the all-clear scan state). */
export function Empty({ title, children, action, icon, tone = 'neutral' }: { title: string; children: ReactNode; action?: ReactNode; icon?: ReactNode; tone?: 'neutral' | 'positive' }) {
  return (
    <Card className={`empty${tone === 'positive' ? ' positive' : ''} border-dashed bg-[var(--color-surface)] p-10 text-center`}>
      <CardContent className="p-0 flex flex-col items-center">
        {icon && <span className={`empty-icon ${tone === 'positive' ? 'border-[color-mix(in_oklch,var(--color-success)_40%,var(--color-rule))] bg-[var(--color-success-soft)] text-[var(--color-success)]' : ''}`}>{icon}</span>}
        <h2 className="font-display text-xl font-bold mt-2">{title}</h2>
        <p className="mt-2 text-sm text-[var(--color-ink-soft)] max-w-prose">{children}</p>
        {action && <div className="mt-6">{action}</div>}
      </CardContent>
    </Card>
  );
}

export function PrivacyNotice() {
  return (
    <Card className="privacy flex items-center gap-3 border-[var(--color-rule)] bg-[var(--color-surface-muted)] p-4">
      <span className="flex h-8 w-8 items-center justify-center rounded-full bg-[var(--color-accent-soft)] text-[var(--color-accent-strong)]">
        <ShieldCheck className="h-4 w-4" />
      </span>
      <div className="text-sm">
        <strong className="font-semibold">Private by design</strong>
        <span className="ml-2 text-[var(--color-ink-soft)]">Your source code stays on this computer. No account or telemetry required.</span>
      </div>
    </Card>
  );
}

export function LanguageBadges({ languages }: { languages?: string[] }) {
  return languages?.length
    // Real <li> elements: the old <Badge/> children rendered <div>s inside this <ul>.
    ? <ul className="badges flex flex-wrap gap-1.5" aria-label="Detected languages">{languages.map((language) => <li key={language} className="badge"><i className="lang-dot" aria-hidden="true" style={{ background: languageColor(language) }} />{languageNames[language] ?? language}</li>)}</ul>
    : <span className="muted">No supported source languages found</span>;
}

export function SeverityCounts({ scan }: { scan?: Scan }) {
  const values: Array<[Severity, number | undefined, string]> = [
    ['critical', scan?.critical_count, 'critical'],
    ['high', scan?.high_count, 'high'],
    ['medium', scan?.medium_count, 'medium'],
    ['low', scan?.low_count, 'low'],
  ];
  return <div className="severity-counts flex gap-6">{values.map(([severity, count]) => <span key={severity} className={`${severity}${severity === 'critical' && (count ?? 0) > 0 ? ' critical-live' : ''}`}>{count ?? 0} <small>{severity}</small></span>)}</div>;
}

export function ToolSummary({ tools }: { tools: Tool[] }) {
  return <div className="tool-summary flex flex-wrap gap-3">{tools.length ? tools.map((tool) => <Badge key={tool.id} variant={tool.ready ? 'success' : 'danger'} className="gap-1.5"><i className={`dot ${tool.ready ? 'ready' : 'not-ready'} !m-0`} />{tool.id}: {tool.ready ? 'ready' : 'not installed'}</Badge>) : <span className="muted">Tool status will appear once the backend is ready.</span>}</div>;
}

export function SummaryCard({ label, value, tone }: { label: string; value: number; tone?: string }) {
  const shown = useCountUp(value);
  return (
    <Card className={`summary-card ${tone ?? ''} p-5`}>
      <strong className="font-display text-2xl font-extrabold tracking-tight">{shown}</strong>
      <span className="mt-1 text-xs font-mono font-semibold tracking-wide text-[var(--color-ink-faint)] uppercase">{label}</span>
    </Card>
  );
}
