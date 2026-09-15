import { Suspense, lazy, useState, type ReactNode } from 'react';
import { ChevronDown } from 'lucide-react';
import { api } from '../api';
import type { AnalyzerRun, RiskProfile, Scan } from '../types';
import type { Route } from '../lib/router';
import type { Notice } from '../lib/notice';
import { message } from '../lib/notice';
import { analyzerName, date, languageColor } from '../lib/format';
import { useLoad } from '../hooks/useLoad';
import { Empty, ErrorPanel, LanguageBadges, Loading } from '../components/ui';
import { ScanIcon } from '../components/icons';
import { SkeletonCards, SkeletonTable } from '../components/skeletons';
import { SeverityTrendSection } from '../components/SeverityTrendChart';
import { SuppressionsSection } from '../components/SuppressionsPanel';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '../components/ui/tabs';
import { ConfirmationDialog } from '../components/dialogs';
import { WorkspaceContextSidebar } from '../components/WorkspaceContext';
import { HistoryTable } from './HistoryPage';
import { analyzerMeta, categoryColor, CATEGORY_LABELS } from '../lib/analyzerCatalog';
import { languageCoverageFromSnapshot, severityCountsFromSummary, sparkSeries, trendPointsFromScans, type LanguageCoverage } from '../lib/chartData';
import { useReducedMotion } from '../hooks/useReducedMotion';
import { Sparkles, Copy, Check, ShieldAlert, BarChart3, AlertTriangle, Layers, FileSearch, ShieldCheck, Bug } from 'lucide-react';
import { ScanActionDropdown } from '../components/ScanActionDropdown';
import { PreScanSummary } from '../components/PreScanSummary';
import { PageHeader } from '../components/PageHeader';

const AnalyticsCharts = lazy(() => import('../components/AnalyticsCharts').then((m) => ({ default: m.AnalyticsCharts })));
const DependencyGraph = lazy(() => import('../components/DependencyGraph').then((m) => ({ default: m.DependencyGraph })) );
const ComplianceMatrix = lazy(() => import('../components/ComplianceMatrix').then((m) => ({ default: m.ComplianceMatrix })) );

/** A scan only speaks for the workspace once it finished; everything else (cancelled, interrupted, queued, running) records zeros or unknowns. */
function isCompletedState(state?: string | null): boolean {
  return state === 'completed' || state === 'completed_with_warnings';
}

/** Completion timestamp for ordering; hostile timestamps order as "oldest" instead of throwing. */
function scanTime(scan: Scan): number {
  const time = new Date(scan.finished_at ?? scan.started_at ?? '').getTime();
  return Number.isNaN(time) ? 0 : time;
}

/** C3 · the number is only useful once the math behind it is one hover away. */
const RISK_SCORE_EXPLAINER = 'Weighted risk score: critical ×10, high ×5, medium ×2, low · A 0–4, B 5–19, C 20–49, D 50+';

export function WorkspacePage({ id, go, notify }: { id: string; go: (r: Route) => void; notify: (n: Notice) => void }) {
  const workspace = useLoad(() => api.workspace(id), [id]);
  const scans = useLoad(() => api.scans(id), [id]);
  const risk = useLoad(() => api.risk(id), [id]);
  // Pre-flight data (IMP-14): what the selected profile will run, whether the
  // engines are installed, and which exclusions will shape the selection.
  const analyzers = useLoad(api.analyzers, []);
  const rules = useLoad(() => api.rules(id), [id]);
  const overrides = useLoad(() => api.pathOverrides(id), [id]);
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
  const reduced = useReducedMotion();
  // Declared before the handlers below use it (openSettings reads name/profile).
  const item = workspace.data;
  const scanHistory = scans.data ?? [];
  // The workspace payload's latest_scan omits the discovery snapshot that scan-list
  // rows carry (Scan.snapshot), so backfill it from the matching history row before
  // computing per-language coverage — without the backfill the Languages tab never
  // shows counts even though the snapshot is recorded.
  const withSnapshot = (scan?: Scan | null): Scan | undefined =>
    scan ? { ...scan, snapshot: scan.snapshot ?? scanHistory.find((row) => row.id === scan.id)?.snapshot } : undefined;
  const rawLatest = withSnapshot(item?.latest_scan);
  // C2 · a cancelled/interrupted/queued "latest" scan records zeros, so metric cards
  // built from it lied ("0 findings") beside a Risk card graded on real data. Metrics
  // grade on the newest scan that actually finished: prefer latest_scan only when it
  // completed, else the server's last_completed_scan, else the newest completed row
  // from the loaded history. The raw latest stays on the Latest-scan panel, where
  // "what ran last" belongs, with a notice bridging the two.
  const newestCompletedScan = scanHistory
    .filter((scan) => isCompletedState(scan.state))
    .reduce<Scan | undefined>((newest, scan) => (!newest || scanTime(scan) > scanTime(newest) ? scan : newest), undefined);
  const latest = isCompletedState(rawLatest?.state)
    ? rawLatest
    : withSnapshot(item?.last_completed_scan ?? newestCompletedScan);
  const fellBackToCompleted = Boolean(rawLatest && latest && rawLatest.id !== latest.id);
  // Plain-words bridge for the fallback above, shown next to the Latest-scan panel.
  const fallbackNotice = fellBackToCompleted && rawLatest && latest
    ? `Last scan was ${rawLatest.state.replaceAll('_', ' ')} — showing the last completed scan (${latest.total_findings ?? 0} findings, ${date(latest.finished_at ?? latest.started_at)}).`
    : null;
  // Real per-language file counts live on the effective scan's discovery snapshot; when it is absent the language list renders without counts rather than inventing them.
  const coverage = languageCoverageFromSnapshot(latest?.snapshot);
  // Confetti celebrates a fresh completed run — a cancelled one is not a milestone.
  const isCompleted = isCompletedState(rawLatest?.state);
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
  // A NOT_FOUND workspace never loads, so "Try again" alone is a dead end — offer the way back.
  if (workspace.error) return <div className="page"><ErrorPanel error={workspace.error} retry={workspace.reload} />{workspace.error.includes('NOT_FOUND') && <div className="mt-4 flex justify-center"><button type="button" className="button secondary" onClick={() => go({ page: 'workspaces' })}>Back to Workspaces</button></div>}</div>;
  if (!item) return <div className="page"><Loading /></div>;
  // Null (not 0) when nothing has completed: an unscanned workspace must not read as "all clear".
  const criticalHigh = latest ? (latest.critical_count ?? 0) + (latest.high_count ?? 0) : null;
  return <div className="page workspace-page"><WorkspaceContextSidebar id={id} current={{ page: 'workspace', id }} onNavigate={go} /><div className="workspace-page-body">
    {/* 1 · Identify — who this is. Clean unified PageHeader. */}
    <PageHeader
      eyebrow="Workspace"
      title={item.name}
      badge={
        item.languages?.length ? (
          <div className="workspace-lang-dots flex items-center gap-1.5 flex-wrap" aria-label="Detected languages">
            {item.languages.map((lang) => (
              <span key={lang} className="ws-lang-dot text-xs flex items-center gap-1 px-2 py-0.5 rounded-[var(--radius-xs)] bg-[var(--color-surface-muted)] border border-[var(--color-rule-faint)] text-[var(--color-ink-soft)] font-mono">
                <i className="w-1.5 h-1.5 rounded-full shrink-0" style={{ background: languageColor(lang) }} />
                {lang}
              </span>
            ))}
            <span className="sr-only"><LanguageBadges languages={item.languages} /></span>
          </div>
        ) : (
          <>
            <span className="text-xs text-[var(--color-ink-faint)]">No supported source languages found</span>
            <span className="sr-only"><LanguageBadges languages={item.languages} /></span>
          </>
        )
      }
      description={
        <div className="workspace-path-row flex items-center gap-1.5 font-mono text-xs">
          {/* w-fit keeps the code element hugging the text; a reserved max-width left the copy button floating far from short paths. */}
          <code className="workspace-path w-fit max-w-full text-[var(--color-ink-faint)] truncate" title={item.root_path}>
            {item.root_path || 'No root path configured'}
          </code>
          {item.root_path && (
            <button type="button" className="workspace-copy-btn shrink-0" onClick={copyPath} aria-label="Copy path" title={copied ? 'Copied' : 'Copy path'}>
              {copied ? <Check className="h-3.5 w-3.5 text-[var(--color-success)]" /> : <Copy className="h-3.5 w-3.5" />}
            </button>
          )}
        </div>
      }
    >
      {isCompleted && !reduced && <div className="workspace-confetti" aria-hidden="true"><i /><i /><i /><i /><i /><i /></div>}
    </PageHeader>

    {/* 2 · Act — one primary action, everything else quietly grouped beside it. */}
    <div className="workspace-toolbar workspace-action-rail">
      <div className="action-rail-primary">
        <fieldset className="profile-picker segmented" aria-label="Scan profile"><span className="profile-picker-label">Profile</span>{['quick', 'standard', 'deep', 'pentest'].map((value) => <button key={value} type="button" aria-pressed={profile === value} onClick={() => setProfile(value)}>{value}</button>)}</fieldset>
        <ScanActionDropdown workspaceId={id} workspaceName={item.name} defaultProfile={profile} size="default" variant="primary" go={go} notify={notify} />
      </div>
      <div className="action-rail-secondary">
        <button type="button" className="button ghost" onClick={() => go({ page: 'pentest', id })}>Pentest suite</button>
        <button type="button" className="button ghost" onClick={() => go({ page: 'files', id })}>Configure files</button>
        <button type="button" className="button ghost" disabled={!latest} onClick={() => latest && go({ page: 'scan', id: latest.id })}>View last report</button>
        {latest && <a className="button ghost" href={api.markdownUrl(latest.id)}>Export Markdown</a>}
        <button type="button" className="button ghost" onClick={openSettings}>Workspace settings</button>
        <button type="button" className="button ghost" onClick={()=>setPruneOpen(true)}>Prune history…</button>
        <button type="button" className="button ghost workspace-danger-item" onClick={()=>setDeleteOpen(true)}>Remove workspace</button>
      </div>
    </div>
    {/* Pre-flight: what this profile will run before it runs (IMP-14). */}
    <PreScanSummary
      profile={profile}
      languages={item.languages}
      analyzers={analyzers.data ?? []}
      exclusionCount={
        (rules.data?.rules ?? []).filter((rule) => {
          const typed = rule as { rule_type?: string; enabled?: boolean };
          return typed.enabled !== false && typed.rule_type?.includes('exclude');
        }).length +
        (overrides.data ?? []).filter((override) => override.mode === 'exclude').length
      }
    />
    {pruneOpen && <form className="settings-editor" onSubmit={(event) => { event.preventDefault(); void prune(); }} aria-label="Prune scan history"><label>Keep newest<input type="number" min={1} max={100} value={pruneKeep} onChange={(event) => setPruneKeep(Number(event.target.value))} /></label><div className="editor-actions"><button type="submit" className="button primary" disabled={pruning}>Delete older scans</button><button type="button" className="button secondary" onClick={() => setPruneOpen(false)}>Cancel</button></div></form>}
    {editing && <form className="settings-editor" onSubmit={(event) => { event.preventDefault(); saveSettings(); }} aria-label="Workspace settings"><label>Name<input value={nameDraft} onChange={(event) => setNameDraft(event.target.value)} maxLength={80} /></label><div className="settings-editor-profile"><span>Default profile</span><fieldset className="segmented" aria-label="Default profile">{['quick', 'standard', 'deep', 'pentest'].map((value) => <button key={value} type="button" aria-pressed={profileDraft === value} onClick={() => setProfileDraft(value)}>{value}</button>)}</fieldset></div><div className="editor-actions"><button type="submit" className="button primary" disabled={savingSettings}>Save</button><button type="button" className="button secondary" onClick={() => setEditing(false)}>Cancel</button></div></form>}

    {/* 3 · Verdict — three headline numbers; the rest are one click away.
        Cards grade on the effective (completed) scan; with nothing completed they
        read "—" + "no scan yet" instead of zeros that imply all-clear. */}
    {!latest && scans.loading ? <SkeletonCards count={6} /> : <section className={`workspace-verdict ${reduced ? '' : 'is-animated'}`} aria-label="Latest scan summary">
      <div className="summary-grid premium">
        <RiskCard risk={risk.data} unscanned={!latest} />
        <PremiumSummaryCard label="Critical + high" value={criticalHigh} tone="high" icon={<ShieldAlert className="h-4 w-4" />} spark={latest ? sparkSeries(scanHistory, (s) => s.critical_count != null && s.high_count != null ? s.critical_count + s.high_count : undefined) : null} delay={1} />
        <PremiumSummaryCard label="Total findings" value={latest ? latest.total_findings ?? 0 : null} icon={<BarChart3 className="h-4 w-4" />} spark={latest ? sparkSeries(scanHistory, (s) => s.total_findings) : null} delay={2} />
      </div>
      <Disclosure label="All severity counts" hint={latest ? `${latest.total_findings ?? 0} findings` : 'no scan yet'}>
        <div className="summary-grid premium">
          <PremiumSummaryCard label="Medium" value={latest ? latest.medium_count ?? 0 : null} tone="medium" icon={<AlertTriangle className="h-4 w-4" />} spark={latest ? sparkSeries(scanHistory, (s) => s.medium_count) : null} delay={1} />
          <PremiumSummaryCard label="Low + info" value={latest ? (latest.low_count ?? 0) + (latest.info_count ?? 0) : null} icon={<Layers className="h-4 w-4" />} spark={latest ? sparkSeries(scanHistory, (s) => s.low_count != null && s.info_count != null ? s.low_count + s.info_count : undefined) : null} delay={2} />
          <PremiumSummaryCard label="New" value={latest ? latest.new_count ?? 0 : null} icon={<FileSearch className="h-4 w-4" />} spark={latest ? sparkSeries(scanHistory, (s) => s.new_count) : null} delay={3} />
          <PremiumSummaryCard label="Fixed" value={latest ? latest.fixed_count ?? 0 : null} icon={<ShieldCheck className="h-4 w-4" />} spark={latest ? sparkSeries(scanHistory, (s) => s.fixed_count) : null} delay={4} />
        </div>
      </Disclosure>
    </section>}

    {/* 4 · Insights — five diagnostics share one card, one on screen at a time.
        Radix only mounts the selected panel, so the lazy chunks stay unloaded
        until someone actually asks for them. */}
    <Tabs defaultValue="trends" className="workspace-insights">
      <div className="workspace-insights-head">
        <h2>Insights</h2>
        <TabsList>
          <TabsTrigger value="trends">Trends</TabsTrigger>
          <TabsTrigger value="pentest">Pentest &amp; OWASP</TabsTrigger>
          <TabsTrigger value="severity">Severity</TabsTrigger>
          <TabsTrigger value="languages">Languages</TabsTrigger>
          <TabsTrigger value="dependencies">Dependencies</TabsTrigger>
          <TabsTrigger value="compliance">Compliance</TabsTrigger>
        </TabsList>
      </div>
      <TabsContent value="trends">
        <Suspense fallback={<div className="skeleton-chart" aria-busy="true" />}>
          <AnalyticsCharts
            trends={trendPointsFromScans(scanHistory)}
            severityCounts={severityCountsFromSummary({ critical_count: latest?.critical_count, high_count: latest?.high_count, medium_count: latest?.medium_count, low_count: latest?.low_count, info_count: latest?.info_count })}
            languages={coverage}
          />
        </Suspense>
      </TabsContent>
      <TabsContent value="pentest"><WorkspacePentestInsight workspaceId={id} scanId={latest?.id} go={go} /></TabsContent>
      <TabsContent value="severity"><SeverityTrendSection workspaceId={id} /></TabsContent>
      <TabsContent value="languages">
        {item.languages?.length
          ? <LanguageDistributionDonut names={item.languages} coverage={coverage} workspaceId={id} go={go} />
          : <Empty title="No languages detected" icon={<FileSearch />}>Run a scan to see how this project is split across languages.</Empty>}
      </TabsContent>
      <TabsContent value="dependencies">
        <Suspense fallback={<SkeletonCards count={1} variant="chart" />}>
          <DependencyGraph languages={item.languages} />
        </Suspense>
      </TabsContent>
      <TabsContent value="compliance">
        {latest
          ? <ComplianceSection scanId={latest.id} go={go} />
          : <Empty title="Nothing to map yet" icon={<ScanIcon />}>Compliance mapping needs a completed scan.</Empty>}
      </TabsContent>
    </Tabs>

    {/* 5 · Activity — what ran, and what it found. The panel shows the scan that actually
        ran last (even a cancelled one — that is the truthful "latest attempt"); when the
        verdict cards above grade on an older completed scan instead, the notice says so. */}
    <section className="split-section workspace-history-section"><div className="workspace-section-card"><h2>Latest scan</h2>{(rawLatest ?? latest) ? <>{fallbackNotice && <p className="muted" role="note">{fallbackNotice}</p>}<p className="muted">{(rawLatest ?? latest)!.state.replaceAll('_', ' ')} · {date((rawLatest ?? latest)!.finished_at ?? (rawLatest ?? latest)!.started_at)}</p>{(rawLatest ?? latest)!.error_summary && <div className="inline-warning">Warning: {(rawLatest ?? latest)!.error_summary}</div>}<AnalyzerStatuses runs={(rawLatest ?? latest)!.analyzer_runs} /></> : <Empty title="Ready when you are" icon={<ScanIcon />}>Run the first scan to get a combined report.</Empty>}</div><div className="workspace-section-card"><h2>Scan history</h2>{scans.loading ? <SkeletonTable rows={4} cols={10} /> : scans.error ? <ErrorPanel error={scans.error} retry={scans.reload} /> : <HistoryTable scans={scans.data ?? []} go={go} />}</div></section>

    {/* 6 · Housekeeping — set once, then ignored. The gloss keeps "Suppress" from reading as jargon. */}
    <p className="muted suppressions-gloss">Suppress = “not an issue for us” — hides a finding from all future scans and reports for this workspace.</p>
    <SuppressionsSection workspaceId={id} notify={notify} />
  </div>
  {deleteOpen && <ConfirmationDialog title="Remove this workspace?" description="This removes the saved workspace, file rules, and local scan history from Blunt Code. Your project files will not be changed." confirmLabel="Remove workspace" busy={deleting} onCancel={() => setDeleteOpen(false)} onConfirm={remove} />}</div>;
}

/** Collapsed-by-default section; children mount on first open so lazy panels are never fetched for someone who does not look. */
function Disclosure({ label, hint, children }: { label: string; hint?: string; children: ReactNode }) {
  const [mounted, setMounted] = useState(false);
  return <details className="workspace-disclosure" onToggle={(event) => { if (event.currentTarget.open) setMounted(true); }}>
    <summary className="workspace-disclosure-toggle"><ChevronDown className="workspace-disclosure-caret" aria-hidden="true" />{label}{hint && <small>{hint}</small>}</summary>
    {mounted && <div className="workspace-disclosure-panel">{children}</div>}
  </details>;
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

function PremiumSummaryCard({ label, value, tone, icon, spark, delay }: { label: string; /** null = nothing has completed yet — the card reads "—" / "no scan yet" instead of a lying 0. */ value: number | null; tone?: string; icon: ReactNode; spark?: number[] | null; delay: number }) {
  const unscanned = value == null;
  return <div className={`summary-card premium-card ${tone ?? ''}`} style={{ animationDelay: `${delay*40}ms` }}>
    <span className="premium-card-icon">{icon}</span>
    <strong>{unscanned ? '—' : value}</strong>
    <span>{unscanned ? 'no scan yet' : label}</span>
    {/* Only a real multi-scan trend draws a line (sparkSeries returns null below two points). */}
    {spark && spark.length > 1 && <MiniSparkline values={spark} />}
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
    // Display names read like products ("Gitleaks", not "gitleaks-secrets"); an
    // id missing from the map falls back to itself, and the raw id stays on hover.
    return <div className="analyzer-row" key={run.analyzer_id}><span title={run.analyzer_id}>{analyzerName(run.analyzer_id)}</span><span className={`state ${run.status}`}>{run.status}</span>{skipped ? <small role="note" className="text-amber-600">Skipped — {run.message || 'no applicable files or profile excluded this analyzer'}</small> : run.message ? <small>{run.message}</small> : null}</div>;
  })}</div>)}</div>;
}

function LanguageDistributionDonut({ names, coverage, workspaceId, go }: { names: string[]; coverage: LanguageCoverage[]; workspaceId: string; go: (r: Route) => void }) {
  function drill(lang: string) {
    // One navigation that carries the filter (Route.q) — a single history entry, and the files page opens pre-filtered.
    go({ page: 'files', id: workspaceId, q: `lang=${encodeURIComponent(lang)}` });
  }
  // Names-only mode: the scan snapshot recorded no per-language counts, so the
  // list renders without counts, percentages, or a total rather than inventing them.
  if (!coverage.length) {
    return (
      <section aria-label="Language distribution" className="workspace-section-card lang-donut-card">
        <div className="mb-3 flex items-center justify-between">
          <h3 className="font-display text-sm font-semibold tracking-tight">Language distribution</h3>
          <span className="rounded-full border border-[var(--color-rule)] bg-[var(--color-surface-muted)] px-2 py-0.5 font-mono text-[0.65rem] font-semibold uppercase tracking-widest text-[var(--color-ink-faint)]">click to filter files</span>
        </div>
        <ul className="lang-pill-legend" aria-label="Languages, select to filter files">
          {names.map((language) => (
            <li key={language}>
              <button type="button" onClick={() => drill(language)} className="lang-pill" aria-label={`Show only ${language} files`}>
                <i aria-hidden="true" className="h-2.5 w-2.5 shrink-0 rounded-full" style={{ background: languageColor(language) }} />
                <span>{language}</span>
              </button>
            </li>
          ))}
        </ul>
        <p className="muted mt-3 text-xs">No discovery snapshot available for this scan yet.</p>
      </section>
    );
  }
  const total = coverage.reduce((s, l) => s + l.files, 0);
  const colors = ['var(--color-accent)', 'var(--color-success)', 'var(--color-warning)', 'var(--color-danger)', 'var(--color-ink)', 'var(--color-accent-strong)', 'var(--color-ink-soft)', 'var(--color-ink-faint)'];
  const cx = 60; const cy = 60; const r = 46; const inner = 30;
  let angle = -90;
  const segs = coverage.map((row, i) => {
    // Clamp so a single-language project still draws its (near-)full ring: an arc from a point to itself renders nothing.
    const sweep = Math.min(total ? (row.files / total) * 360 : 0, 359.9);
    const start = angle; const end = angle + sweep; angle = end;
    const large = sweep > 180 ? 1 : 0;
    const rad = (d: number) => (d * Math.PI) / 180;
    const x1 = cx + r * Math.cos(rad(start)); const y1 = cy + r * Math.sin(rad(start));
    const x2 = cx + r * Math.cos(rad(end)); const y2 = cy + r * Math.sin(rad(end));
    const ix1 = cx + inner * Math.cos(rad(end)); const iy1 = cy + inner * Math.sin(rad(end));
    const ix2 = cx + inner * Math.cos(rad(start)); const iy2 = cy + inner * Math.sin(rad(start));
    const d = `M ${x1} ${y1} A ${r} ${r} 0 ${large} 1 ${x2} ${y2} L ${ix1} ${iy1} A ${inner} ${inner} 0 ${large} 0 ${ix2} ${iy2} Z`;
    return { ...row, d, color: row.aggregate ? 'var(--color-ink-faint)' : colors[i % colors.length] };
  });
  return (
    <section aria-label="Language distribution" className="workspace-section-card lang-donut-card">
      <div className="mb-3 flex items-center justify-between">
        <h3 className="font-display text-sm font-semibold tracking-tight">Language distribution</h3>
        <span className="rounded-full border border-[var(--color-rule)] bg-[var(--color-surface-muted)] px-2 py-0.5 font-mono text-[0.65rem] font-semibold uppercase tracking-widest text-[var(--color-ink-faint)]">click to filter files</span>
      </div>
      <div className="flex flex-wrap items-center gap-6">
        <svg viewBox="0 0 120 120" width={180} height={180} role="img" aria-label={`Language distribution: ${coverage.map((l) => `${l.language} ${l.files}`).join(', ')}`} className="shrink-0">
          {segs.map((s) => s.aggregate ? (
            // The "N more" slice stands for several languages, so it cannot drill into one.
            <path key={s.language} d={s.d} fill={s.color} stroke="var(--color-surface)" strokeWidth={1.2} />
          ) : (
            <path key={s.language} d={s.d} fill={s.color} stroke="var(--color-surface)" strokeWidth={1.2} className="cursor-pointer hover:opacity-80 focus:opacity-80" tabIndex={0} role="button" aria-label={`Filter files by ${s.language}`} onClick={() => drill(s.language)} onKeyDown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); drill(s.language); } }} />
          ))}
          {/* The hole paints after the segments but must never intercept their clicks. */}
          <circle cx={cx} cy={cy} r={inner - 0.5} fill="var(--color-surface)" style={{ pointerEvents: 'none' }} />
          <text x={cx} y={cy - 2} textAnchor="middle" fontSize={14} fontWeight={800} fill="var(--color-ink)" className="tabular-nums">{total}</text>
          <text x={cx} y={cy + 12} textAnchor="middle" fontSize={7} fontWeight={600} fill="var(--color-ink-faint)" style={{ letterSpacing: '0.06em', textTransform: 'uppercase' as const }}>files</text>
        </svg>
        <ul className="lang-pill-legend" aria-label="Language legend, select to filter">
          {segs.map((s) => (
            <li key={s.language}>
              {s.aggregate ? (
                <span className="lang-pill">
                  <i aria-hidden="true" className="h-2.5 w-2.5 shrink-0 rounded-full" style={{ background: s.color }} />
                  <span>{s.language}</span>
                  <span className="font-mono font-semibold">{s.files}</span>
                  <span className="lang-pill-pct">{total ? `${Math.round((s.files * 1000) / total) / 10}%` : '0%'}</span>
                </span>
              ) : (
                <button type="button" onClick={() => drill(s.language)} className="lang-pill" aria-label={`Show only ${s.language} files`}>
                  <i aria-hidden="true" className="h-2.5 w-2.5 shrink-0 rounded-full" style={{ background: s.color }} />
                  <span>{s.language}</span>
                  <span className="font-mono font-semibold">{s.files}</span>
                  <span className="lang-pill-pct">{total ? `${Math.round((s.files * 1000) / total) / 10}%` : '0%'}</span>
                </button>
              )}
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
        <ComplianceMatrix findings={findings} scanId={scanId} onFilterOwasp={() => go({ page: 'scan', id: scanId })} />
      </div>
    </Suspense>
  );
}

export function RiskCard({ risk, unscanned }: { risk?: RiskProfile | null; /** true when no completed scan exists: "—" beats a 0 that reads as all-clear. */ unscanned?: boolean }) {
  if (!risk?.available || typeof risk.score !== 'number') return <div className="summary-card premium-card risk-hero"><span className="premium-card-icon"><ShieldAlert className="h-4 w-4" /></span><strong>{unscanned ? '—' : '0'}</strong><span>Risk score{unscanned ? ' · no scan yet' : ''}</span></div>;
  // C8 · words beat glyphs: "Risk D · ▼ 4" made screen readers say "down arrow" and
  // left everyone else guessing whether down was good. Trend direction now says
  // what happened, and color carries the verdict (down = good here).
  const delta = typeof risk.previous_score === 'number' ? Math.abs(Math.round(risk.score - risk.previous_score)) : null;
  const noChange = { text: 'no change', tone: 'text-[var(--color-ink-soft)]' };
  const trendNote = risk.trend === 'flat'
    ? noChange
    : (risk.trend === 'up' || risk.trend === 'down') && delta != null
      ? delta === 0
        ? noChange
        : risk.trend === 'down'
          ? { text: `improved ${delta} pts since last scan`, tone: 'text-[var(--color-success)]' }
          : { text: `worsened ${delta} pts`, tone: 'text-[var(--color-warning)]' }
      : null;
  const gradeTone = risk.grade === 'A' ? 'positive' : risk.grade === 'B' ? 'medium' : 'high';
  // A grade is only as strong as the scan behind it: when analyzer runs
  // failed or degraded, the score reflects partial coverage and the card
  // must say so instead of implying full assurance.
  const coverage = risk.coverage;
  const partial = coverage && risk.complete === false;
  const coverageNote = partial
    ? `partial coverage: ${coverage!.succeeded} of ${coverage!.total} analyzers completed` +
      (coverage!.failed > 0 ? `, ${coverage!.failed} failed` : '') +
      (coverage!.warned > 0 ? `, ${coverage!.warned} degraded` : '')
    : null;
  return <div className={`summary-card premium-card risk-hero ${gradeTone}`} title={risk.finished_at ? `Latest scan finished ${risk.finished_at}` : undefined}>
    <span className="premium-card-icon"><ShieldAlert className="h-4 w-4" /></span>
    <span className="risk-grade">{risk.grade}</span>
    <strong className="risk-score" title={RISK_SCORE_EXPLAINER}>{Math.round(risk.score)}</strong>
    <span>Risk {risk.grade}</span>
    {trendNote && <small role="note" className={`risk-trend ${trendNote.tone}`}>{trendNote.text}</small>}
    {coverageNote && <span className="risk-coverage-note" data-partial="true">{coverageNote}</span>}
  </div>;
}

function WorkspacePentestInsight({ workspaceId, scanId, go }: { workspaceId: string; scanId?: string; go: (r: Route) => void }) {
  const report = useLoad(() => (scanId ? api.report(scanId) : Promise.resolve(null)), [scanId]);
  const findings = report.data?.findings ?? [];
  const secFindings = findings.filter((f) => {
    const cat = (f.category || '').toLowerCase();
    const a = (f.analyzer_id || '').toLowerCase();
    const r = (f.rule_id || '').toLowerCase();
    return (
      cat === 'pentest' ||
      cat === 'security' ||
      cat === 'vulnerability' ||
      a === 'pentest' ||
      a === 'secrets' ||
      a === 'gitleaks-secrets' ||
      a === 'semgrep' ||
      r.includes('sqli') ||
      r.includes('xss') ||
      r.includes('ssrf') ||
      r.includes('jwt') ||
      r.includes('rce') ||
      r.includes('secret')
    );
  });

  const critical = secFindings.filter((f) => f.severity === 'critical').length;
  const high = secFindings.filter((f) => f.severity === 'high').length;
  const medium = secFindings.filter((f) => f.severity === 'medium').length;

  return (
    <div className="workspace-section-card space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h3 className="text-base font-semibold text-[var(--color-ink)] flex items-center gap-2">
            <ShieldAlert className="h-4 w-4 text-[var(--color-danger)]" />OWASP Top 10 &amp; Pentest Posture
          </h3>
          <p className="text-xs text-[var(--color-ink-soft)]">
            Active security vulnerabilities, injection flaws, broken authentication, and exposed secrets in this workspace.
          </p>
        </div>
        <button type="button" className="button ghost" onClick={() => go({ page: 'pentest', id: workspaceId })}>
          Open full Pentest Suite →
        </button>
      </div>

      <div className="grid gap-3 sm:grid-cols-3">
        <div className="p-3 rounded-[var(--radius-md)] border border-[var(--color-rule-faint)] bg-[var(--color-surface)]">
          <span className="text-xs font-semibold text-[var(--color-danger)]">Critical Flaws</span>
          <p className="font-display text-2xl font-bold mt-1 text-[var(--color-danger)]">{critical}</p>
          <span className="text-[11px] text-[var(--color-ink-faint)]">RCE, SQLi, Hardcoded JWT, XXE</span>
        </div>
        <div className="p-3 rounded-[var(--radius-md)] border border-[var(--color-rule-faint)] bg-[var(--color-surface)]">
          <span className="text-xs font-semibold text-[var(--color-warning)]">High Severity</span>
          <p className="font-display text-2xl font-bold mt-1 text-[var(--color-warning)]">{high}</p>
          <span className="text-[11px] text-[var(--color-ink-faint)]">SSRF, XSS, Path Traversal, CORS</span>
        </div>
        <div className="p-3 rounded-[var(--radius-md)] border border-[var(--color-rule-faint)] bg-[var(--color-surface)]">
          <span className="text-xs font-semibold text-[var(--color-ink-soft)]">Medium / Warnings</span>
          <p className="font-display text-2xl font-bold mt-1 text-[var(--color-ink)]">{medium}</p>
          <span className="text-[11px] text-[var(--color-ink-faint)]">Weak Hashes, Missing Headers, Debug</span>
        </div>
      </div>

      {secFindings.length > 0 ? (
        <div className="space-y-2">
          <h4 className="text-xs font-semibold text-[var(--color-ink)] uppercase tracking-wide">Top Identified Security Issues</h4>
          <div className="space-y-1.5">
            {secFindings.slice(0, 5).map((f) => (
              <div key={f.id || f.fingerprint} className="flex items-center justify-between gap-2 p-2.5 rounded-[var(--radius-sm)] border border-[var(--color-rule-faint)] bg-[var(--color-surface)] text-xs">
                <div className="min-w-0 flex-1">
                  <span className="font-semibold text-[var(--color-ink)]">{f.relative_path}:{f.start_line}</span>
                  <span className="text-[var(--color-ink-soft)] ml-2">{f.message}</span>
                </div>
                <span className={`state ${f.severity} text-[10px] uppercase font-mono px-2 py-0.5 rounded`}>{f.severity}</span>
              </div>
            ))}
          </div>
        </div>
      ) : (
        <div className="p-4 rounded-[var(--radius-md)] border border-[var(--color-success)]/30 bg-[var(--color-success-soft)] text-xs text-[var(--color-ink-soft)] flex items-center gap-2">
          <ShieldCheck className="h-4 w-4 text-[var(--color-success)] shrink-0" />
          <span>No critical OWASP vulnerabilities detected in the latest scan. Run a Pentest Scan to verify against all 22+ checks.</span>
        </div>
      )}
    </div>
  );
}
