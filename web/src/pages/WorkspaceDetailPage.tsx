import { Suspense, lazy, useState } from 'react';
import { api } from '../api';
import type { AnalyzerRun, RiskProfile } from '../types';
import type { Route } from '../lib/router';
import type { Notice } from '../lib/notice';
import { message } from '../lib/notice';
import { date } from '../lib/format';
import { useLoad } from '../hooks/useLoad';
import { Empty, ErrorPanel, LanguageBadges, Loading } from '../components/ui';
import { ScanIcon } from '../components/icons';
import { SkeletonCards, SkeletonTable } from '../components/skeletons';
import { SeverityTrendSection } from '../components/SeverityTrendChart';
import { SuppressionsSection } from '../components/SuppressionsPanel';
import { Accordion, AccordionContent, AccordionItem, AccordionTrigger } from '../components/ui/accordion';
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from '../components/ui/dropdown-menu';
import { ConfirmationDialog } from '../components/dialogs';
import { WorkspaceContextSidebar } from '../components/WorkspaceContext';
import { HistoryTable } from './HistoryPage';
import { analyzerMeta, categoryColor, CATEGORY_LABELS } from '../lib/analyzerCatalog';
import { languageCoverageFromLanguages, severityCountsFromSummary, trendPointsFromScans } from '../lib/chartData';
import { Sparkles, Copy, Check, ShieldAlert, BarChart3, Clock as ClockIcon, AlertTriangle, Layers, FileSearch, History as HistoryIcon, ShieldCheck } from 'lucide-react';

const AnalyticsCharts = lazy(() => import('../components/AnalyticsCharts').then((m) => ({ default: m.AnalyticsCharts })));
const DependencyGraph = lazy(() => import('../components/DependencyGraph').then((m) => ({ default: m.DependencyGraph })) );
const ComplianceMatrix = lazy(() => import('../components/ComplianceMatrix').then((m) => ({ default: m.ComplianceMatrix })) );

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
  const [copied, setCopied] = useState(false);
  const latest = workspace.data?.latest_scan ?? scans.data?.[0];
  const isCompleted = latest?.state === 'completed' || latest?.state === 'completed_with_warnings';
  async function start() { try { const scan = await api.startScan(id, profile); go({ page: 'scan', id: scan.id }); } catch (e) { notify({ kind: 'error', text: message(e) }); } }
  function copyPath() {
    const p = workspace.data?.root_path ?? '';
    if (!p) return;
    navigator.clipboard?.writeText(p).then(()=>{ setCopied(true); setTimeout(()=>setCopied(false), 1400); }).catch(()=>{});
  }
  function openSettings() { setNameDraft(item?.name ?? ''); setProfileDraft(item?.default_profile ?? 'standard'); setEditing(true); }
  async function saveSettings() { setSavingSettings(true); try { await api.updateWorkspace(id, { name: nameDraft.trim() || undefined, default_profile: profileDraft }); await workspace.reload(); setEditing(false); notify({ kind: 'success', text: 'Workspace settings saved.' }); } catch (e) { notify({ kind: 'error', text: message(e) }); } finally { setSavingSettings(false); } }
  async function prune() { setPruning(true); try { const result = await api.pruneScans(id, pruneKeep); setPruneOpen(false); await Promise.all([workspace.reload(), scans.reload()]); notify({ kind: 'success', text: `Deleted ${result.deleted} old scan${result.deleted === 1 ? '' : 's'}; kept the newest ${result.kept}.` }); } catch (e) { notify({ kind: 'error', text: message(e) }); } finally { setPruning(false); } }
  async function remove() { setDeleting(true); try { await api.deleteWorkspace(id); go({ page: 'workspaces' }); notify({ kind: 'info', text: 'Workspace removed from Blunt Code.' }); } catch (e) { notify({ kind: 'error', text: message(e) }); setDeleting(false); } }
  if (workspace.loading) return <div className="page"><Loading /></div>;
  if (workspace.error) return <div className="page"><ErrorPanel error={workspace.error} retry={workspace.reload} /></div>;
  const item = workspace.data;
  if (!item) return <div className="page"><Loading /></div>;
  const sparkVals = (v:number)=>[Math.max(0,v-2),Math.max(0,v-1),v,Math.max(0,v-1),Math.max(0,v)];
  return <div className="page workspace-page"><WorkspaceContextSidebar id={id} current={{ page: 'workspace', id }} onNavigate={go} /><div className="workspace-page-body">
    {/* Hero bento card */}
    <header className={`workspace-hero-bento ${isCompleted ? 'confetti' : ''}`}>
      <div className="workspace-hero-main">
        <p className="eyebrow">Workspace</p>
        <h1 className="workspace-hero-title">{item.name}</h1>
        <div className="workspace-path-row">
          <code className="workspace-path" title={item.root_path}>{item.root_path}</code>
          <button type="button" className="workspace-copy-btn" onClick={copyPath} aria-label="Copy path">{copied ? <Check className="h-3.5 w-3.5" /> : <Copy className="h-3.5 w-3.5" />}</button>
        </div>
        <div className="workspace-lang-dots" aria-label="Detected languages">
          {item.languages?.length ? item.languages.map((lang,i)=>(
            <span key={lang} className="ws-lang-dot"><i style={{background: ['var(--color-accent)','var(--color-success)','var(--color-warning)','oklch(62% 0.18 285)'][i%4]}} />{lang}</span>
          )) : <span className="muted text-xs">No supported source languages found</span>}
          {/* keep LanguageBadges for tests/assistive but visually hidden */}
          <span className="sr-only"><LanguageBadges languages={item.languages} /></span>
        </div>
        {isCompleted && <div className="workspace-confetti" aria-hidden="true"><i /><i /><i /><i /><i /><i /></div>}
      </div>
      <div className="workspace-next">
        <span className="workspace-next-kicker">Next step</span>
        <strong className="workspace-next-title">Run a fresh analysis</strong>
        <p className="workspace-next-desc">Scan with the latest rules. Results stay local.</p>
        <label className="profile-picker">Profile{' '}<select value={profile} onChange={(event) => setProfile(event.target.value)} aria-label="Scan profile">{['quick', 'standard', 'deep'].map((value) => <option key={value} value={value}>{value}</option>)}</select></label>
        <button type="button" className="button primary workspace-run" onClick={start}><Sparkles className="h-4 w-4" />Run scan</button>
        {latest && <a className="workspace-view-report" href={latest ? `/scans/${latest.id}` : '#'} onClick={(e)=>{e.preventDefault(); latest && go({page:'scan', id:latest.id})}}>View last report →</a>}
      </div>
    </header>
    <div className="workspace-toolbar"><div className="toolbar-left"><button type="button" className="button ghost" onClick={() => go({ page: 'files', id })}>Configure files</button><button type="button" className="button ghost" disabled={!latest} onClick={() => latest && go({ page: 'scan', id: latest.id })}>View last report</button></div><div className="toolbar-right">{latest && <a className="button secondary" href={api.markdownUrl(latest.id)}>Export Markdown</a>}<DropdownMenu><DropdownMenuTrigger className="button secondary">More ▾</DropdownMenuTrigger><DropdownMenuContent align="end"><DropdownMenuItem onSelect={openSettings}>Workspace settings</DropdownMenuItem><DropdownMenuItem onSelect={()=>setPruneOpen(true)}>Prune history…</DropdownMenuItem><DropdownMenuSeparator/><DropdownMenuItem className="text-[var(--color-danger)]" onSelect={()=>setDeleteOpen(true)}>Remove workspace</DropdownMenuItem></DropdownMenuContent></DropdownMenu></div></div>
    {pruneOpen && <form className="settings-editor" onSubmit={(event) => { event.preventDefault(); void prune(); }} aria-label="Prune scan history"><label>Keep newest<input type="number" min={1} max={100} value={pruneKeep} onChange={(event) => setPruneKeep(Number(event.target.value))} /></label><div className="editor-actions"><button type="submit" className="button primary" disabled={pruning}>Delete older scans</button><button type="button" className="button secondary" onClick={() => setPruneOpen(false)}>Cancel</button></div></form>}
    {editing && <form className="settings-editor" onSubmit={(event) => { event.preventDefault(); saveSettings(); }} aria-label="Workspace settings"><label>Name<input value={nameDraft} onChange={(event) => setNameDraft(event.target.value)} maxLength={80} /></label><label>Default profile<select value={profileDraft} onChange={(event) => setProfileDraft(event.target.value)}>{['quick', 'standard', 'deep'].map((value) => <option key={value} value={value}>{value}</option>)}</select></label><div className="editor-actions"><button type="submit" className="button primary" disabled={savingSettings}>Save</button><button type="button" className="button secondary" onClick={() => setEditing(false)}>Cancel</button></div></form>}
    {!latest && scans.loading ? <SkeletonCards count={6} /> : <section className="summary-grid premium" aria-label="Latest scan summary">
      <RiskCard risk={risk.data} />
      <PremiumSummaryCard label="Critical + high" value={(latest?.critical_count ?? 0) + (latest?.high_count ?? 0)} tone="high" icon={<ShieldAlert className="h-4 w-4" />} spark={sparkVals((latest?.critical_count ?? 0)+(latest?.high_count ?? 0))} delay={1} />
      <PremiumSummaryCard label="Medium" value={latest?.medium_count ?? 0} tone="medium" icon={<AlertTriangle className="h-4 w-4" />} spark={sparkVals(latest?.medium_count ?? 0)} delay={2} />
      <PremiumSummaryCard label="Low + info" value={(latest?.low_count ?? 0) + (latest?.info_count ?? 0)} icon={<Layers className="h-4 w-4" />} spark={sparkVals((latest?.low_count ?? 0)+(latest?.info_count ?? 0))} delay={3} />
      <PremiumSummaryCard label="Total findings" value={latest?.total_findings ?? 0} icon={<BarChart3 className="h-4 w-4" />} spark={sparkVals(latest?.total_findings ?? 0)} delay={4} />
      <PremiumSummaryCard label="New" value={latest?.new_count ?? 0} icon={<FileSearch className="h-4 w-4" />} spark={sparkVals(latest?.new_count ?? 0)} delay={5} />
      <PremiumSummaryCard label="Fixed" value={latest?.fixed_count ?? 0} icon={<ShieldCheck className="h-4 w-4" />} spark={sparkVals(latest?.fixed_count ?? 0)} delay={6} />
    </section>}
    <Suspense fallback={<div className="skeleton-chart" aria-busy="true" />}>
      <AnalyticsCharts
        trends={trendPointsFromScans(scans.data ?? [])}
        severityCounts={severityCountsFromSummary({ critical_count: latest?.critical_count, high_count: latest?.high_count, medium_count: latest?.medium_count, low_count: latest?.low_count, info_count: latest?.info_count })}
        languages={languageCoverageFromLanguages(item.languages)}
      />
    </Suspense>
    <LanguageDistributionDonut languages={languageCoverageFromLanguages(item.languages)} workspaceId={id} go={go} />
    <SeverityTrendSection workspaceId={id} />
    <Accordion type="single" collapsible className="workspace-card-accordion">
      <AccordionItem value="dependency-graph">
        <AccordionTrigger className="text-sm font-semibold">Dependency graph</AccordionTrigger>
        <AccordionContent>
          <Suspense fallback={<SkeletonCards count={1} variant="chart" />}>
            <DependencyGraph languages={item.languages} />
          </Suspense>
        </AccordionContent>
      </AccordionItem>
    </Accordion>
    {latest && <ComplianceSection scanId={latest.id} go={go} />}
    <SuppressionsSection workspaceId={id} notify={notify} />
    <section className="split-section workspace-history-section"><div className="workspace-section-card"><h2>Latest analysis</h2>{latest ? <><p className="muted">{latest.state.replaceAll('_', ' ')} · {date(latest.finished_at ?? latest.started_at)}</p>{latest.error_summary && <div className="inline-warning">Warning: {latest.error_summary}</div>}<AnalyzerStatuses runs={latest.analyzer_runs} /></> : <Empty title="Ready when you are" icon={<ScanIcon />}>Run the first scan to get a combined report.</Empty>}</div><div className="workspace-section-card"><h2>Scan history</h2>{scans.loading ? <SkeletonTable rows={4} cols={10} /> : scans.error ? <ErrorPanel error={scans.error} retry={scans.reload} /> : <HistoryTable scans={scans.data ?? []} go={go} />}</div></section>
  {deleteOpen && <ConfirmationDialog title="Remove this workspace?" description="This removes the saved workspace, file rules, and local scan history from Blunt Code. Your project files will not be changed." confirmLabel="Remove workspace" busy={deleting} onCancel={() => setDeleteOpen(false)} onConfirm={remove} />}</div></div>;
}

function MiniSparkline({ values }: { values: number[] }) {
  if (!values.length) return null;
  const max = Math.max(...values,1);
  const min = Math.min(...values);
  const range = Math.max(max-min,1);
  const w=64,h=20;
  const step = values.length>1 ? w/(values.length-1) : 0;
  const pts = values.map((v,i)=> `${i*step},${h - ((v-min)/range)*h}`);
  return <svg viewBox={`0 0 ${w} ${h}`} width={64} height={20} aria-hidden="true" className="premium-sparkline"><polyline fill="none" stroke="var(--color-accent)" strokeWidth={1.6} strokeLinejoin="round" strokeLinecap="round" points={pts.join(' ')} /></svg>;
}

function PremiumSummaryCard({ label, value, tone, icon, spark, delay }: { label: string; value: number; tone?: string; icon: React.ReactNode; spark: number[]; delay: number }) {
  return <div className={`summary-card premium-card ${tone ?? ''}`} style={{ animationDelay: `${delay*40}ms` } as React.CSSProperties}>
    <span className="premium-card-icon">{icon}</span>
    <strong>{value}</strong>
    <span>{label}</span>
    <MiniSparkline values={spark} />
  </div>;
}

export function AnalyzerStatuses({ runs }: { runs?: AnalyzerRun[] }) {
  if (!runs?.length) return <div className="analyzers"><h3>Analyzer status</h3><p className="muted">Analyzer detail appears after a scan.</p></div>;
  const grouped = new Map<string, typeof runs>();
  for (const r of runs) {
    const cat = analyzerMeta(r.analyzer_id)?.category ?? 'other';
    const arr = grouped.get(cat) ?? [];
    arr.push(r);
    grouped.set(cat, arr);
  }
  return <div className="analyzers"><h3>Analyzer status</h3>{[...grouped.entries()].map(([cat, items]) => <div key={cat} className="analyzer-group"><span className="analyzer-group-label" style={{ borderLeftColor: categoryColor(cat as never) }}>{(CATEGORY_LABELS as Record<string, string>)[cat] ?? cat}</span>{items.map((run) => {
    const skipped = run.status === 'skipped';
    return <div className="analyzer-row" key={run.analyzer_id}><span>{run.analyzer_id}</span><span className={`state ${run.status}`}>{run.status}</span>{skipped ? <small role="note" className="text-amber-600">Skipped — {run.message || 'no applicable files or profile excluded this analyzer'}</small> : run.message ? <small>{run.message}</small> : null}</div>;
  })}</div>)}</div>;
}

function LanguageDistributionDonut({ languages, workspaceId, go }: { languages: ReturnType<typeof languageCoverageFromLanguages>; workspaceId: string; go: (r: Route) => void }) {
  if (!languages.length) return null;
  const total = languages.reduce((s, l) => s + l.files, 0);
  const colors = ['var(--color-accent)', 'var(--color-success)', 'var(--color-warning)', 'var(--color-danger)', 'var(--color-ink)', 'var(--color-accent-strong)', 'var(--color-ink-soft)', 'var(--color-ink-faint)'];
  const cx = 60; const cy = 60; const r = 46; const inner = 30;
  let angle = -90;
  const segs = languages.map((row, i) => {
    const sweep = total ? (row.files / total) * 360 : 0;
    const start = angle; const end = angle + sweep; angle = end;
    const large = sweep > 180 ? 1 : 0;
    const rad = (d: number) => (d * Math.PI) / 180;
    const x1 = cx + r * Math.cos(rad(start)); const y1 = cy + r * Math.sin(rad(start));
    const x2 = cx + r * Math.cos(rad(end)); const y2 = cy + r * Math.sin(rad(end));
    const ix1 = cx + inner * Math.cos(rad(end)); const iy1 = cy + inner * Math.sin(rad(end));
    const ix2 = cx + inner * Math.cos(rad(start)); const iy2 = cy + inner * Math.sin(rad(start));
    const d = `M ${x1} ${y1} A ${r} ${r} 0 ${large} 1 ${x2} ${y2} L ${ix1} ${iy1} A ${inner} ${inner} 0 ${large} 0 ${ix2} ${iy2} Z`;
    return { ...row, d, color: colors[i % colors.length] };
  });
  function drill(lang: string) {
    const url = `/workspaces/${workspaceId}/files?lang=${encodeURIComponent(lang)}`;
    window.history.pushState(null, '', url);
    go({ page: 'files', id: workspaceId });
  }
  return (
    <section aria-label="Language distribution" className="workspace-section-card lang-donut-card">
      <div className="mb-3 flex items-center justify-between">
        <h3 className="font-display text-sm font-semibold tracking-tight">Language distribution</h3>
        <span className="rounded-full border border-[var(--color-rule)] bg-[var(--color-surface-muted)] px-2 py-0.5 font-mono text-[0.65rem] font-semibold uppercase tracking-widest text-[var(--color-ink-faint)]">click to filter files</span>
      </div>
      <div className="flex flex-wrap items-center gap-6">
        <svg viewBox="0 0 120 120" width={180} height={180} role="img" aria-label={`Language distribution: ${languages.map((l) => `${l.language} ${l.files}`).join(', ')}`} className="shrink-0">
          {segs.map((s) => (
            <path key={s.language} d={s.d} fill={s.color} stroke="var(--color-surface)" strokeWidth={1.2} className="cursor-pointer hover:opacity-80 focus:opacity-80" tabIndex={0} role="button" aria-label={`Filter files by ${s.language}`} onClick={() => drill(s.language)} onKeyDown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); drill(s.language); } }} />
          ))}
          <circle cx={cx} cy={cy} r={inner - 0.5} fill="var(--color-surface)" />
          <text x={cx} y={cy - 2} textAnchor="middle" fontSize={14} fontWeight={800} fill="var(--color-ink)" className="tabular-nums">{total}</text>
          <text x={cx} y={cy + 12} textAnchor="middle" fontSize={7} fontWeight={600} fill="var(--color-ink-faint)" style={{ letterSpacing: '0.06em', textTransform: 'uppercase' as const }}>files</text>
        </svg>
        <ul className="lang-pill-legend" aria-label="Language legend, select to filter">
          {segs.map((s) => (
            <li key={s.language}>
              <button type="button" onClick={() => drill(s.language)} className="lang-pill" aria-label={`Show only ${s.language} files`}>
                <i aria-hidden="true" className="h-2.5 w-2.5 shrink-0 rounded-full" style={{ background: s.color }} />
                <span>{s.language}</span>
                <span className="font-mono font-semibold">{s.files}</span>
                <span className="lang-pill-pct">{total ? `${Math.round((s.files * 1000) / total) / 10}%` : '0%'}</span>
              </button>
            </li>
          ))}
        </ul>
      </div>
    </section>
  );
}

function ComplianceSection({ scanId, go }: { scanId: string; go: (r: Route) => void }) {
  const report = useLoad(() => api.report(scanId), [scanId]);
  const findings = report.data?.findings ?? [];
  if (report.loading) return <div className="skeleton-chart" aria-busy="true" />;
  if (!findings.length && !report.loading) return null;
  return (
    <Suspense fallback={<div className="skeleton-chart" aria-busy="true" />}>
      <div className="workspace-section-card">
        <ComplianceMatrix findings={findings} scanId={scanId} onFilterOwasp={(owasp) => go({ page: 'scan', id: scanId })} />
      </div>
    </Suspense>
  );
}

export function RiskCard({ risk }: { risk?: RiskProfile | null }) {
  if (!risk?.available || typeof risk.score !== 'number') return <div className="summary-card premium-card risk-hero"><span className="premium-card-icon"><ShieldAlert className="h-4 w-4" /></span><strong>0</strong><span>Risk score</span></div>;
  const arrow = risk.trend === 'up' ? '▲' : risk.trend === 'down' ? '▼' : '＝';
  const delta = typeof risk.previous_score === 'number' ? ` · ${arrow} ${Math.abs(Math.round(risk.score - risk.previous_score))}` : '';
  const gradeTone = risk.grade === 'A' ? 'positive' : risk.grade === 'B' ? 'medium' : 'high';
  return <div className={`summary-card premium-card risk-hero ${gradeTone}`}>
    <span className="premium-card-icon"><ShieldAlert className="h-4 w-4" /></span>
    <span className="risk-grade">{risk.grade}</span>
    <strong className="risk-score">{Math.round(risk.score)}</strong>
    <span>Risk {risk.grade}{delta}</span>
  </div>;
}
