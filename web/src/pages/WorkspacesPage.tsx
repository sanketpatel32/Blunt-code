import { useEffect, useState } from 'react';
import '../css/workspaces.css';
import { Plus } from 'lucide-react';
import { ApiError, api } from '../api';
import type { Scan, Workspace } from '../types';
import type { Route } from '../lib/router';
import type { Notice } from '../lib/notice';
import { message } from '../lib/notice';
import { count, date, languageColor, languageNames, relativeTime, scanStateDisplay } from '../lib/format';
import { useLoad } from '../hooks/useLoad';
import { useDebouncedValue } from '../hooks/useDebouncedValue';
import { Empty, ErrorPanel } from '../components/ui';
import { Button } from '../components/ui/button';
import { Input } from '../components/ui/input';
import { FolderIcon } from '../components/icons';
import { SkeletonTable } from '../components/skeletons';
import { ConfirmationDialog } from '../components/dialogs';
import { WorkspaceTemplates } from '../components/WorkspaceTemplates';
import { RowMenu, type RowMenuItem } from '../components/RowMenu';
import { PageHeader } from '../components/PageHeader';
import { riskGrade, riskScore, severityCountsOf } from '../lib/risk';

export function WorkspacesPage({ go, onAdd, notify }: { go: (r: Route) => void; onAdd: () => void; notify: (n: Notice) => void }) {
  const state = useLoad(api.workspaces, []);
  const [sort, setSort] = useState<WorkspaceSort>(()=>{ const sp=new URLSearchParams(window.location.search); const k=sp.get('sort') as WorkspaceSortKey; const d=sp.get('order'); if(k && ['name','last_scan','findings'].includes(k)) return { key:k, dir: d==='asc'||d==='desc'?d:'desc' }; return { key: 'last_scan', dir: 'desc' }; });
  const [tagQuery, setTagQuery] = useState(()=> new URLSearchParams(window.location.search).get('q') ?? '');
  const [search, setSearch] = useState(()=> new URLSearchParams(window.location.search).get('search') ?? '');
  const debouncedSearch = useDebouncedValue(search, search ? 300 : 0);
  const debouncedTagQuery = useDebouncedValue(tagQuery, tagQuery ? TAG_FILTER_DEBOUNCE_MS : 0);
  // Overflow actions live at page level so at most one confirm dialog exists at a time.
  const [pendingScan, setPendingScan] = useState<{ workspace: Workspace; profile: string } | null>(null);
  const [scanning, setScanning] = useState(false);
  const [removeTarget, setRemoveTarget] = useState<Workspace | null>(null);
  const [removing, setRemoving] = useState(false);
  // URL sync
  useEffect(()=>{ const sp=new URLSearchParams(); if(debouncedTagQuery) sp.set('q',debouncedTagQuery); if(debouncedSearch) sp.set('search',debouncedSearch); if(sort.key!=='last_scan'||sort.dir!=='desc'){ sp.set('sort',sort.key); sp.set('order',sort.dir); } const qs=sp.toString(); const cur=window.location.search.replace(/^\?/,''); if(qs===cur) return; const nxt=qs? `${window.location.pathname}?${qs}${window.location.hash}` : `${window.location.pathname}${window.location.hash}`; window.history.replaceState(null,'',nxt); },[debouncedTagQuery, debouncedSearch, sort]);
  useEffect(()=>{ const onPop=()=>{ const sp=new URLSearchParams(window.location.search); setTagQuery(sp.get('q')??''); setSearch(sp.get('search')??''); const k=sp.get('sort') as WorkspaceSortKey; const d=sp.get('order'); if(k && ['name','last_scan','findings'].includes(k)) setSort({ key:k, dir: d==='asc'||d==='desc'?d:'desc' }); }; window.addEventListener('popstate',onPop); return()=>window.removeEventListener('popstate',onPop); },[]);
  const workspaces = state.data ?? [];
  const byTag = filterWorkspacesByTag(sortWorkspaces(workspaces, sort.key, sort.dir), debouncedTagQuery);
  const visibleWorkspaces = debouncedSearch ? byTag.filter(w=> w.name.toLowerCase().includes(debouncedSearch.toLowerCase()) || w.root_path.toLowerCase().includes(debouncedSearch.toLowerCase())) : byTag;
  const tagNeedle = debouncedTagQuery.trim().toLowerCase();
  const hasFilter = !!(debouncedTagQuery || debouncedSearch);
  const narrowed = hasFilter && visibleWorkspaces.length !== workspaces.length;
  // The toolbar count is the ONE place this screen reports how many workspaces exist.
  const countLine = narrowed
    ? `${visibleWorkspaces.length} of ${workspaces.length} ${workspaces.length === 1 ? 'workspace' : 'workspaces'} shown`
    : `${workspaces.length} ${workspaces.length === 1 ? 'workspace' : 'workspaces'}`;

  function openScan(workspace: Workspace, profile: string) { setPendingScan({ workspace, profile }); }

  async function runPendingScan() {
    if (!pendingScan || scanning) return;
    setScanning(true);
    try {
      const active = await api.startScan(pendingScan.workspace.id, pendingScan.profile);
      notify({ kind: 'info', text: `${pendingScan.profile.charAt(0).toUpperCase()}${pendingScan.profile.slice(1)} scan initiated.` });
      setPendingScan(null);
      go({ page: 'scan', id: active.id });
    } catch (e) {
      setPendingScan(null);
      // A scan already running for this workspace is not an error to fight —
      // the 409 carries the active scan id, so offer to open it instead of
      // leaving the user with a bare toast.
      if (e instanceof ApiError && e.code === 'SCAN_ALREADY_ACTIVE' && typeof e.details?.scan_id === 'string') {
        const activeId = e.details.scan_id as string;
        notify({ kind: 'info', text: 'A scan is already running for this workspace.', action: { label: 'View scan', onClick: () => go({ page: 'scan', id: activeId }) } });
      } else {
        notify({ kind: 'error', text: message(e) });
      }
    } finally {
      setScanning(false);
    }
  }

  async function removeWorkspace() {
    if (!removeTarget || removing) return;
    setRemoving(true);
    try {
      await api.deleteWorkspace(removeTarget.id);
      notify({ kind: 'info', text: 'Workspace removed from Blunt Code.' });
      setRemoveTarget(null);
      state.reload();
    } catch (e) {
      notify({ kind: 'error', text: message(e) });
      setRemoving(false);
    }
  }

  const addPrimary = <Button size="sm" onClick={onAdd}><Plus aria-hidden="true" />Add workspace</Button>;

  return (
    <div className="page page-workspaces">
      <PageHeader
        title="Workspaces"
        description="Every registered project with its latest scan results."
        actions={workspaces.length ? addPrimary : undefined}
      />
      {state.loading ? <SkeletonTable rows={8} cols={6} /> : state.error ? <ErrorPanel error={state.error} retry={state.reload} /> : !workspaces.length ? (
        <><Empty title="No workspaces yet" icon={<FolderIcon />} action={addPrimary}>Add a project folder to scan it for security findings. Everything runs locally — your code never leaves this machine.</Empty><WorkspaceTemplates onUseTemplate={onAdd} /></>
      ) : (
        <>
          <div className="toolbar-row ws-toolbar">
            <div className="toolbar-filters">
              <Input type="search" aria-label="Search workspaces" placeholder="Search workspaces…" value={search} onChange={(event) => setSearch(event.target.value)} onKeyDown={(event) => { if (event.key === 'Escape') setSearch(''); }} className="ws-filter-search w-44 sm:w-56" />
              <Input type="search" aria-label="Filter by tag" placeholder="Filter by tag…" value={tagQuery} onChange={(event) => setTagQuery(event.target.value)} onKeyDown={(event) => { if (event.key === 'Escape') setTagQuery(''); }} className="ws-filter-tag w-36 sm:w-44" />
              {hasFilter && <button type="button" className="text-button ws-filter-clear" onClick={()=>{ setTagQuery(''); setSearch(''); }}>Clear</button>}
            </div>
            <span className="toolbar-meta ws-count tabular-nums" role="status">{countLine}</span>
          </div>
          {visibleWorkspaces.length ? (
            <div className="table-wrap ws-table-wrap">
              <table className="table-dense ws-table">
                <caption className="sr-only">Registered workspaces with languages, risk grade, findings, and last scan</caption>
                <thead>
                  <tr>
                    <SortHeader label="Workspace" sortKey="name" sort={sort} onSort={(key) => setSort((current) => current.key === key ? { key, dir: current.dir === 'asc' ? 'desc' : 'asc' } : { key, dir: firstClickDir[key] })} />
                    <th scope="col">Languages</th>
                    <th scope="col">Risk</th>
                    <SortHeader label="Findings" sortKey="findings" sort={sort} onSort={(key) => setSort((current) => current.key === key ? { key, dir: current.dir === 'asc' ? 'desc' : 'asc' } : { key, dir: firstClickDir[key] })} />
                    <SortHeader label="Last scan" sortKey="last_scan" sort={sort} onSort={(key) => setSort((current) => current.key === key ? { key, dir: current.dir === 'asc' ? 'desc' : 'asc' } : { key, dir: firstClickDir[key] })} />
                    <th scope="col"><span className="sr-only">Actions</span></th>
                  </tr>
                </thead>
                <tbody>
                  {visibleWorkspaces.map((workspace) => <WorkspaceRow key={workspace.id} workspace={workspace} go={go} onScan={openScan} onRemove={setRemoveTarget} />)}
                </tbody>
              </table>
            </div>
          ) : debouncedSearch ? <Empty title={`No workspaces match “${debouncedSearch}”`} icon={<FolderIcon />} action={<Button variant="outline" size="sm" onClick={() => setSearch('')}>Clear search</Button>}>No workspace name or path contains “{debouncedSearch}”.</Empty> : <Empty title="No workspaces match this tag" icon={<FolderIcon />}>No workspace tags contain “{tagNeedle}”. Clear the filter to see every project.</Empty>}
        </>
      )}
      {pendingScan && <ConfirmationDialog tone="primary" title={`Run ${pendingScan.profile} scan on ${pendingScan.workspace.name}?`} description={`A ${pendingScan.profile} scan runs the enabled analyzers over this workspace and can take several minutes. You can cancel it from the scan page while it runs.`} confirmLabel={`Run ${pendingScan.profile} scan`} busy={scanning} onCancel={() => setPendingScan(null)} onConfirm={runPendingScan} />}
      {removeTarget && <ConfirmationDialog title="Remove this workspace?" description="This removes the saved workspace, file rules, and local scan history from Blunt Code. Your project files will not be changed." confirmLabel="Remove workspace" busy={removing} onCancel={() => setRemoveTarget(null)} onConfirm={removeWorkspace} />}
    </div>
  );
}

/** Typing waits for a pause before re-filtering so long tags never flicker per keystroke; short enough to feel instant. */
const TAG_FILTER_DEBOUNCE_MS = 200;

/** Client-side tag search: a workspace matches when ANY of its tags contains the query substring (case-insensitive); an empty query passes everything through. */
export function filterWorkspacesByTag(workspaces: Workspace[], query: string): Workspace[] {
  const needle = query.trim().toLowerCase();
  if (!needle) return workspaces;
  return workspaces.filter((workspace) => (workspace.tags ?? []).some((tag) => tag.toLowerCase().includes(needle)));
}

export type WorkspaceSortKey = 'name' | 'last_scan' | 'findings';
export interface WorkspaceSort { key: WorkspaceSortKey; dir: 'asc' | 'desc'; }
// Dates and counts read best newest/largest-first, names alphabetically.
const firstClickDir: Record<WorkspaceSortKey, 'asc' | 'desc'> = { name: 'asc', last_scan: 'desc', findings: 'desc' };

function lastScanTime(workspace: Workspace): number | null {
  const time = new Date(workspace.last_scan_at ?? workspace.latest_scan?.finished_at ?? workspace.latest_scan?.started_at ?? '').getTime();
  return Number.isNaN(time) ? null : time;
}

/** Client-side ordering behind the sortable column headers; workspaces without any scan timestamp always sink to the end. */
export function sortWorkspaces(workspaces: Workspace[], key: WorkspaceSortKey, dir: 'asc' | 'desc'): Workspace[] {
  return [...workspaces].sort((a, b) => {
    if (key === 'name') { const order = a.name.localeCompare(b.name, undefined, { sensitivity: 'base' }); return dir === 'asc' ? order : -order; }
    if (key === 'findings') { const delta = (a.latest_scan?.total_findings ?? 0) - (b.latest_scan?.total_findings ?? 0); return dir === 'asc' ? delta : -delta; }
    const at = lastScanTime(a);
    const bt = lastScanTime(b);
    if (at === null || bt === null) return at === null && bt === null ? 0 : at === null ? 1 : -1;
    return dir === 'asc' ? at - bt : bt - at;
  });
}

/** Sortable table header button carrying the shared .th-sort treatment plus aria-sort for assistive tech. */
function SortHeader({ label, sortKey, sort, onSort }: { label: string; sortKey: WorkspaceSortKey; sort: WorkspaceSort; onSort: (key: WorkspaceSortKey) => void }) {
  const active = sort.key === sortKey;
  return (
    <th scope="col" aria-sort={active ? (sort.dir === 'asc' ? 'ascending' : 'descending') : undefined}>
      <button type="button" className={`th-sort${active ? ' active' : ''}`} aria-pressed={active} onClick={() => onSort(sortKey)}>
        {label}
        <span className="sort-arrow" aria-hidden="true">{active ? (sort.dir === 'asc' ? '▲' : '▼') : '↕'}</span>
        {active && <span className="sr-only"> (sorted {sort.dir === 'asc' ? 'ascending' : 'descending'})</span>}
      </button>
    </th>
  );
}

/** Max compact tags shown in the name cell; the rest collapse into a +N chip that titles the hidden ones. */
const MAX_ROW_TAGS = 2;
/** Language dots per the design contract: max three, remainder as a +N tag. */
const MAX_LANG_DOTS = 3;

/** Languages column: up to three color dots (lib/format languageColor) plus a "+N" tag; the full list lives in the tooltip and for screen readers. */
function LanguagesCell({ languages }: { languages?: string[] }) {
  const langs = languages ?? [];
  if (!langs.length) return <td className="ws-cell-langs"><span className="ws-none" title="No supported source languages found">—<span className="sr-only">No supported source languages found</span></span></td>;
  const shown = langs.slice(0, MAX_LANG_DOTS);
  const extra = langs.length - shown.length;
  const names = langs.map((language) => languageNames[language] ?? language).join(', ');
  return (
    <td className="ws-cell-langs">
      <span className="ws-langs" title={names}>
        {shown.map((language) => <i key={language} aria-hidden="true" className="ws-lang" style={{ background: languageColor(language) }} />)}
        {extra > 0 && <span className="tag">+{extra}</span>}
        <span className="sr-only">{names}</span>
      </span>
    </td>
  );
}

/** The finished scan the Findings column grades on: the latest when it actually
 *  finished, else the newest completed one — a cancelled or in-flight latest
 *  must not blank out (or half-report) a good prior result. */
function gradedScan(workspace: Workspace): Scan | undefined {
  const latest = workspace.latest_scan;
  if (latest && (latest.state === 'completed' || latest.state === 'completed_with_warnings')) return latest;
  return workspace.last_completed_scan;
}

/** Map a scanStateDisplay variant onto the shared .state pill classes ('outline' stays on the neutral base). */
const statePillClass: Record<string, string> = { success: 'success', warning: 'warning', danger: 'failed', accent: 'running' };

/** Findings column: the total with colored per-severity counts (severity tokens); zero buckets omitted.
 *  When no scan has finished yet, an in-flight or otherwise unfinished latest scan reports its state
 *  instead of a mute "—" so a running (or hostile-state) scan never reads as "no data". */
function FindingsCell({ workspace }: { workspace: Workspace }) {
  const scan = gradedScan(workspace);
  if (!scan) {
    const latest = workspace.latest_scan;
    if (!latest) return <td className="ws-cell-findings"><span className="ws-none">—</span></td>;
    const display = scanStateDisplay(latest.state);
    return <td className="ws-cell-findings"><span className={`ws-state state ${statePillClass[display.variant] ?? ''}`} title="This scan has not finished — findings appear once it does">{display.label}</span></td>;
  }
  const finishedTime = new Date(scan.finished_at ?? '').getTime();
  const title = !Number.isNaN(finishedTime) ? `Latest finished scan · ${date(scan.finished_at)}` : undefined;
  const critical = scan.critical_count ?? 0;
  const high = scan.high_count ?? 0;
  const medium = scan.medium_count ?? 0;
  const low = scan.low_count ?? 0;
  const total = scan.total_findings;
  if (total == null) return <td className="ws-cell-findings"><span className="ws-none" title={title}>—</span></td>;
  if (!total) return <td className="ws-cell-findings"><span className="ws-findings-zero tabular-nums" title={title}>0</span></td>;
  const categorized = critical + high + medium + low;
  const severities = ([['critical', critical], ['high', high], ['medium', medium], ['low', low]] as const).filter(([, n]) => n > 0);
  return (
    <td className="ws-cell-findings">
      <span className="ws-findings tabular-nums" title={title}>
        {/* The total only leads when the four colored buckets do not already sum to it. */}
        {total > categorized && <span className="ws-findings-total">{count(total)}</span>}
        {severities.map(([severity, n]) => <span key={severity} className={`ws-sev ws-sev-${severity}`}>{count(n)} <em>{severity}</em></span>)}
      </span>
    </td>
  );
}

/** Last scan column: relative time with the absolute stamp in the tooltip; a failed scan keeps its danger signal. */
function LastScanCell({ workspace }: { workspace: Workspace }) {
  const value = workspace.last_scan_at ?? workspace.latest_scan?.finished_at ?? workspace.latest_scan?.started_at ?? null;
  const time = value ? new Date(value).getTime() : NaN;
  if (!value || Number.isNaN(time)) return <td className="ws-cell-scan"><span className="ws-none">Never</span></td>;
  const failed = workspace.latest_scan?.state === 'failed';
  return <td className="ws-cell-scan"><span className={`ws-scan tabular-nums${failed ? ' ws-scan-failed' : ''}`} title={failed ? `Last scan failed · ${date(value)}` : date(value)}>{relativeTime(value)}</span></td>;
}

/** One table row = one workspace. The row itself opens the detail view (click or
 *  Enter on the focused row); its ONE visible control is the default-profile Scan
 *  button — everything else, including Remove (danger, last), lives in the RowMenu. */
function WorkspaceRow({ workspace, go, onScan, onRemove }: { workspace: Workspace; go: (r: Route) => void; onScan: (workspace: Workspace, profile: string) => void; onRemove: (workspace: Workspace) => void }) {
  const open = () => go({ page: 'workspace', id: workspace.id });
  const menuItems: RowMenuItem[] = [
    { label: 'Open details', onSelect: open },
    { label: 'View files', onSelect: () => go({ page: 'files', id: workspace.id }) },
    { label: 'Scan history', onSelect: () => go({ page: 'history', id: workspace.id }) },
    { label: 'Quick scan', onSelect: () => onScan(workspace, 'quick') },
    { label: 'Deep scan', onSelect: () => onScan(workspace, 'deep') },
    // Destructive action last, in the danger tone, behind its own confirmation dialog.
    { label: 'Remove workspace…', tone: 'danger', onSelect: () => onRemove(workspace) },
  ];
  const tags = workspace.tags ?? [];
  const visibleTags = tags.slice(0, MAX_ROW_TAGS);
  const hiddenTags = tags.slice(MAX_ROW_TAGS);
  // No endpoint populates Workspace.risk yet — grade the same scan the Findings
  // column reports on, through the same lib/risk math the Risk board uses, so a
  // grade here can never disagree with the one on Home.
  const graded = gradedScan(workspace);
  const score = graded ? riskScore(severityCountsOf(graded)) : null;
  const grade = score == null ? null : riskGrade(score);
  const riskTone = grade === 'A' ? 'success' : grade === 'B' ? 'warning' : 'failed';
  return (
    <tr tabIndex={0} role="link" aria-label={`Open ${workspace.name || workspace.root_path || 'workspace'}`} onClick={open} onKeyDown={(event) => { if (event.target === event.currentTarget && (event.key === 'Enter' || event.key === ' ')) { event.preventDefault(); open(); } }}>
      <td className="ws-cell-name">
        <span className="ws-name">{workspace.name || 'Untitled workspace'}</span>
        <span className="ws-sub">
          {workspace.root_path && <span className="ws-path" title={workspace.root_path}>{workspace.root_path}</span>}
          {visibleTags.length > 0 && (
            <span className="ws-tags">
              {visibleTags.map((tag) => <span key={tag} className="tag">{tag}</span>)}
              {hiddenTags.length > 0 && <span className="tag" title={hiddenTags.join(', ')}>+{hiddenTags.length}</span>}
            </span>
          )}
        </span>
      </td>
      <LanguagesCell languages={workspace.languages} />
      <td className="ws-cell-risk">{grade ? <span className={`ws-risk state ${riskTone}`} title={`Weighted risk score ${score} from the latest finished scan`}>{grade}</span> : <span className="ws-none">Never scanned</span>}</td>
      <FindingsCell workspace={workspace} />
      <LastScanCell workspace={workspace} />
      {/* Buttons inside must not double-fire the row's open-on-click. */}
      <td className="ws-cell-actions" onClick={(event) => event.stopPropagation()}>
        <div className="table-actions ws-actions">
          <Button variant="secondary" size="sm" title={`Run ${workspace.default_profile ?? 'standard'} scan`} onClick={(event) => { event.stopPropagation(); onScan(workspace, workspace.default_profile ?? 'standard'); }}>Scan</Button>
          <RowMenu items={menuItems} label={`Actions for ${workspace.name || 'workspace'}`} />
        </div>
      </td>
    </tr>
  );
}
