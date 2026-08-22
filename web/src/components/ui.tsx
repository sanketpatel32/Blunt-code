import type { ReactNode } from 'react';
import type { Scan, Severity, Tool } from '../types';
import type { Notice } from '../lib/notice';
import { languageNames } from '../lib/format';

export function NoticeBox({ notice, onDismiss }: { notice: Exclude<Notice, null>; onDismiss: () => void }) {
  return <div className={`notice ${notice.kind}`} role={notice.kind === 'error' ? 'alert' : 'status'}>{notice.text}<button onClick={onDismiss} aria-label="Dismiss">×</button></div>;
}

export function ErrorPanel({ error, retry }: { error: string; retry?: () => void }) {
  return <section className="error-panel" role="alert"><h2>Could not load this view</h2><p>{error}</p>{retry && <button className="button secondary" onClick={retry}>Try again</button>}</section>;
}

export function Loading() {
  return <div className="loading" aria-live="polite">Loading…</div>;
}

export function Empty({ title, children, action }: { title: string; children: ReactNode; action?: ReactNode }) {
  return <section className="empty"><h2>{title}</h2><p>{children}</p>{action}</section>;
}

export function PrivacyNotice() {
  return <aside className="privacy"><strong>Private by design</strong><span>Your source code stays on this computer. No account or telemetry required.</span></aside>;
}

export function LanguageBadges({ languages }: { languages?: string[] }) {
  return languages?.length
    ? <div className="badges" aria-label="Detected languages">{languages.map((language) => <span className="badge" key={language}>{languageNames[language] ?? language}</span>)}</div>
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
