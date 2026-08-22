import type { ReactNode } from 'react';
import type { Scan, Severity, Tool } from '../types';
import { languageNames } from '../lib/format';

export function ErrorPanel({ error, retry }: { error: string; retry?: () => void }) {
  return <section className="error-panel" role="alert"><h2>Could not load this view</h2><p>{error}</p>{retry && <button type="button" className="button secondary" onClick={retry}>Try again</button>}</section>;
}

export function Loading() {
  return <div className="loading" aria-live="polite">Loading…</div>;
}

/** Shared empty-state panel. `icon` renders inside a circular medallion; `tone: 'positive'` switches the medallion to the success tokens (used for the all-clear scan state). */
export function Empty({ title, children, action, icon, tone = 'neutral' }: { title: string; children: ReactNode; action?: ReactNode; icon?: ReactNode; tone?: 'neutral' | 'positive' }) {
  return <section className={`empty${tone === 'positive' ? ' positive' : ''}`}>{icon && <span className="empty-icon">{icon}</span>}<h2>{title}</h2><p>{children}</p>{action}</section>;
}

export function PrivacyNotice() {
  return <aside className="privacy"><strong>Private by design</strong><span>Your source code stays on this computer. No account or telemetry required.</span></aside>;
}

export function LanguageBadges({ languages }: { languages?: string[] }) {
  return languages?.length
    ? <ul className="badges" aria-label="Detected languages">{languages.map((language) => <li className="badge" key={language}>{languageNames[language] ?? language}</li>)}</ul>
    : <span className="muted">No supported source languages found</span>;
}

export function SeverityCounts({ scan }: { scan?: Scan }) {
  const values: Array<[Severity, number | undefined]> = [['critical', scan?.critical_count], ['high', scan?.high_count], ['medium', scan?.medium_count], ['low', scan?.low_count]];
  return <div className="severity-counts">{values.map(([severity, count]) => <span key={severity} className={severity}>{count ?? 0} <small>{severity}</small></span>)}</div>;
}

export function ToolSummary({ tools }: { tools: Tool[] }) {
  return <div className="tool-summary">{tools.length ? tools.map((tool) => <span key={tool.id}><i className={`dot ${tool.ready ? 'ready' : 'not-ready'}`} />{tool.id}: {tool.ready ? 'ready' : 'not installed'}</span>) : <span className="muted">Tool status will appear once the backend is ready.</span>}</div>;
}

export function SummaryCard({ label, value, tone }: { label: string; value: number; tone?: string }) {
  return <article className={`summary-card ${tone ?? ''}`}><strong>{value}</strong><span>{label}</span></article>;
}
