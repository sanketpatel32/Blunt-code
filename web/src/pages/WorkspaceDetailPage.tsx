import { useState } from 'react';
import { api } from '../api';
import type { AnalyzerRun, RiskProfile } from '../types';
import type { Route } from '../lib/router';
import type { Notice } from '../lib/notice';
import { message } from '../lib/notice';
import { date } from '../lib/format';
import { useLoad } from '../hooks/useLoad';
import { Empty, ErrorPanel, LanguageBadges, Loading, SummaryCard } from '../components/ui';
import { ScanIcon } from '../components/icons';
import { SkeletonCards, SkeletonTable } from '../components/skeletons';
import { SeverityTrendSection } from '../components/SeverityTrendChart';
import { SuppressionsSection } from '../components/SuppressionsPanel';
import { ConfirmationDialog } from '../components/dialogs';
import { WorkspaceContextSidebar } from '../components/WorkspaceContext';
import { HistoryTable } from './HistoryPage';

export function WorkspacePage({ id, go, notify }: { id: string; go: (r: Route) => void; notify: (n: Notice) => void }) {
  const workspace = useLoad(() => api.workspace(id), [id]);
  const scans = useLoad(() => api.scans(id), [id]);
  const risk = useLoad(() => api.risk(id), [id]);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [profile, setProfile] = useState('standard');
  const [editing, setEditing] = useState(false);
  const [nameDraft, setNameDraft] = useState('');
  const [profileDraft, setProfileDraft] = useState('standard');
  const [savingSettings, setSavingSettings] = useState(false);
  const [pruneOpen, setPruneOpen] = useState(false);
  const [pruneKeep, setPruneKeep] = useState(20);
  const [pruning, setPruning] = useState(false);
  const latest = workspace.data?.latest_scan ?? scans.data?.[0];
  async function start() { try { const scan = await api.startScan(id, profile); go({ page: 'scan', id: scan.id }); } catch (e) { notify({ kind: 'error', text: message(e) }); } }
  function openSettings() { setNameDraft(item?.name ?? ''); setProfileDraft(item?.default_profile ?? 'standard'); setEditing(true); }
  async function saveSettings() { setSavingSettings(true); try { await api.updateWorkspace(id, { name: nameDraft.trim() || undefined, default_profile: profileDraft }); await workspace.reload(); setEditing(false); notify({ kind: 'success', text: 'Workspace settings saved.' }); } catch (e) { notify({ kind: 'error', text: message(e) }); } finally { setSavingSettings(false); } }
  async function prune() { setPruning(true); try { const result = await api.pruneScans(id, pruneKeep); setPruneOpen(false); await Promise.all([workspace.reload(), scans.reload()]); notify({ kind: 'success', text: `Deleted ${result.deleted} old scan${result.deleted === 1 ? '' : 's'}; kept the newest ${result.kept}.` }); } catch (e) { notify({ kind: 'error', text: message(e) }); } finally { setPruning(false); } }
  async function remove() { setDeleting(true); try { await api.deleteWorkspace(id); go({ page: 'workspaces' }); notify({ kind: 'info', text: 'Workspace removed from Blunt Code.' }); } catch (e) { notify({ kind: 'error', text: message(e) }); setDeleting(false); } }
  if (workspace.loading) return <div className="page"><Loading /></div>;
  if (workspace.error) return <div className="page"><ErrorPanel error={workspace.error} retry={workspace.reload} /></div>;
  const item = workspace.data;
  if (!item) return <div className="page"><Loading /></div>;
  return <div className="page workspace-page"><WorkspaceContextSidebar id={id} current={{ page: 'workspace', id }} onNavigate={go} /><div className="workspace-page-body"><header className="page-heading workspace-heading"><div><p className="eyebrow">Workspace</p><h1>{item.name}</h1><code>{item.root_path}</code><LanguageBadges languages={item.languages} /></div><div className="workspace-next"><span>Next step</span><strong>Run a fresh analysis</strong><label className="profile-picker">Profile{' '}<select value={profile} onChange={(event) => setProfile(event.target.value)} aria-label="Scan profile">{['quick', 'standard', 'deep'].map((value) => <option key={value} value={value}>{value}</option>)}</select></label><button type="button" className="button primary" onClick={start}>Run scan →</button></div></header>
    <div className="action-row"><button type="button" className="button secondary" onClick={() => go({ page: 'files', id })}>Configure files</button><button type="button" className="button secondary" disabled={!latest} onClick={() => latest && go({ page: 'scan', id: latest.id })}>View last report</button>{latest && <a className="button secondary" href={api.markdownUrl(latest.id)}>Export Markdown</a>}<button type="button" className="button secondary" onClick={openSettings}>Workspace settings</button><button type="button" className="button secondary" onClick={() => setPruneOpen(true)}>Prune history…</button><button type="button" className="button danger" onClick={() => setDeleteOpen(true)}>Remove workspace</button></div>
    {pruneOpen && <form className="settings-editor" onSubmit={(event) => { event.preventDefault(); void prune(); }} aria-label="Prune scan history"><label>Keep newest<input type="number" min={1} max={100} value={pruneKeep} onChange={(event) => setPruneKeep(Number(event.target.value))} /></label><div className="editor-actions"><button type="submit" className="button primary" disabled={pruning}>Delete older scans</button><button type="button" className="button secondary" onClick={() => setPruneOpen(false)}>Cancel</button></div></form>}
    {editing && <form className="settings-editor" onSubmit={(event) => { event.preventDefault(); saveSettings(); }} aria-label="Workspace settings"><label>Name<input value={nameDraft} onChange={(event) => setNameDraft(event.target.value)} maxLength={80} /></label><label>Default profile<select value={profileDraft} onChange={(event) => setProfileDraft(event.target.value)}>{['quick', 'standard', 'deep'].map((value) => <option key={value} value={value}>{value}</option>)}</select></label><div className="editor-actions"><button type="submit" className="button primary" disabled={savingSettings}>Save</button><button type="button" className="button secondary" onClick={() => setEditing(false)}>Cancel</button></div></form>}
    {!latest && scans.loading ? <SkeletonCards count={6} /> : <section className="summary-grid" aria-label="Latest scan summary"><RiskCard risk={risk.data} /><SummaryCard label="Critical + high" value={(latest?.critical_count ?? 0) + (latest?.high_count ?? 0)} tone="high" /><SummaryCard label="Medium" value={latest?.medium_count ?? 0} tone="medium" /><SummaryCard label="Low + info" value={(latest?.low_count ?? 0) + (latest?.info_count ?? 0)} /><SummaryCard label="Total findings" value={latest?.total_findings ?? 0} /><SummaryCard label="New since last scan" value={latest?.new_count ?? 0} /><SummaryCard label="Fixed since last scan" value={latest?.fixed_count ?? 0} /></section>}
    <SeverityTrendSection workspaceId={id} />
    <SuppressionsSection workspaceId={id} notify={notify} />
    <section className="split-section"><div><h2>Latest analysis</h2>{latest ? <><p className="muted">{latest.state.replaceAll('_', ' ')} · {date(latest.finished_at ?? latest.started_at)}</p>{latest.error_summary && <div className="inline-warning">Warning: {latest.error_summary}</div>}<AnalyzerStatuses runs={latest.analyzer_runs} /></> : <Empty title="Ready when you are" icon={<ScanIcon />}>Run the first scan to get a combined report.</Empty>}</div><div><h2>Scan history</h2>{scans.loading ? <SkeletonTable rows={4} cols={10} /> : scans.error ? <ErrorPanel error={scans.error} retry={scans.reload} /> : <HistoryTable scans={scans.data ?? []} go={go} />}</div></section>
  {deleteOpen && <ConfirmationDialog title="Remove this workspace?" description="This removes the saved workspace, file rules, and local scan history from Blunt Code. Your project files will not be changed." confirmLabel="Remove workspace" busy={deleting} onCancel={() => setDeleteOpen(false)} onConfirm={remove} />}</div></div>;
}

export function AnalyzerStatuses({ runs }: { runs?: AnalyzerRun[] }) {
  return <div className="analyzers"><h3>Analyzer status</h3>{runs?.length ? runs.map((run) => <div className="analyzer-row" key={run.analyzer_id}><span>{run.analyzer_id}</span><span className={`state ${run.status}`}>{run.status}</span>{run.message && <small>{run.message}</small>}</div>) : <p className="muted">Analyzer detail appears after a scan.</p>}</div>;
}

/** Weighted risk score from the latest completed scan; trend compares the previous one. */
export function RiskCard({ risk }: { risk?: RiskProfile | null }) {
  if (!risk?.available || typeof risk.score !== 'number') return <SummaryCard label="Risk score" value={0} />;
  const arrow = risk.trend === 'up' ? '▲' : risk.trend === 'down' ? '▼' : '＝';
  const delta = typeof risk.previous_score === 'number' ? ` · ${arrow} ${Math.abs(Math.round(risk.score - risk.previous_score))}` : '';
  return <SummaryCard label={`Risk ${risk.grade}${delta}`} value={Math.round(risk.score)} tone={risk.grade === 'A' ? 'positive' : risk.grade === 'B' ? 'medium' : 'high'} />;
}
