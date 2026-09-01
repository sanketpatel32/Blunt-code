import { useEffect, useState } from 'react';
import { api } from '../api';
import type { Workspace } from '../types';
import type { Route } from '../lib/router';
import type { Notice } from '../lib/notice';
import { message } from '../lib/notice';
import { date, relativeTime } from '../lib/format';
import { useLoad } from '../hooks/useLoad';
import { useDebouncedValue } from '../hooks/useDebouncedValue';
import { Empty, ErrorPanel, LanguageBadges } from '../components/ui';
import { Button } from '../components/ui/button';
import { Input } from '../components/ui/input';
import { Card } from '../components/ui/card';
import { Badge } from '../components/ui/badge';
import { FolderIcon } from '../components/icons';
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '../components/ui/dropdown-menu';
import { MoreHorizontal, ShieldCheck } from 'lucide-react';
import { languageNames, languageColor } from '../lib/format';
import { SkeletonCards } from '../components/skeletons';
import { ConfirmationDialog } from '../components/dialogs';
import { WorkspaceTemplates } from '../components/WorkspaceTemplates';
import { PathCopy } from '../components/PathCopy';

export function WorkspacesPage({ go, onAdd, notify }: { go: (r: Route) => void; onAdd: () => void; notify: (n: Notice) => void }) {
  const state = useLoad(api.workspaces, []);
  const [sort, setSort] = useState<WorkspaceSort>(()=>{ const sp=new URLSearchParams(window.location.search); const k=sp.get('sort') as WorkspaceSortKey; const d=sp.get('order'); if(k && ['name','last_scan','findings'].includes(k)) return { key:k, dir: d==='asc'||d==='desc'?d:'desc' }; return { key: 'last_scan', dir: 'desc' }; });
  const [tagQuery, setTagQuery] = useState(()=> new URLSearchParams(window.location.search).get('q') ?? '');
  const [search, setSearch] = useState(()=> new URLSearchParams(window.location.search).get('search') ?? '');
  const debouncedSearch = useDebouncedValue(search, search ? 300 : 0);
  const debouncedTagQuery = useDebouncedValue(tagQuery, tagQuery ? TAG_FILTER_DEBOUNCE_MS : 0);
  // URL sync
  useEffect(()=>{ const sp=new URLSearchParams(); if(debouncedTagQuery) sp.set('q',debouncedTagQuery); if(debouncedSearch) sp.set('search',debouncedSearch); if(sort.key!=='last_scan'||sort.dir!=='desc'){ sp.set('sort',sort.key); sp.set('order',sort.dir); } const qs=sp.toString(); const cur=window.location.search.replace(/^\?/,''); if(qs===cur) return; const nxt=qs? `${window.location.pathname}?${qs}${window.location.hash}` : `${window.location.pathname}${window.location.hash}`; window.history.replaceState(null,'',nxt); },[debouncedTagQuery, debouncedSearch, sort]);
  useEffect(()=>{ const onPop=()=>{ const sp=new URLSearchParams(window.location.search); setTagQuery(sp.get('q')??''); setSearch(sp.get('search')??''); const k=sp.get('sort') as WorkspaceSortKey; const d=sp.get('order'); if(k && ['name','last_scan','findings'].includes(k)) setSort({ key:k, dir: d==='asc'||d==='desc'?d:'desc' }); }; window.addEventListener('popstate',onPop); return()=>window.removeEventListener('popstate',onPop); },[]);
  const workspaces = state.data ?? [];
  const byTag = filterWorkspacesByTag(sortWorkspaces(workspaces, sort.key, sort.dir), debouncedTagQuery);
  const visibleWorkspaces = debouncedSearch ? byTag.filter(w=> w.name.toLowerCase().includes(debouncedSearch.toLowerCase()) || w.root_path.toLowerCase().includes(debouncedSearch.toLowerCase())) : byTag;
  const tagNeedle = debouncedTagQuery.trim().toLowerCase();
  return <div className="page"><header className="page-heading"><div><p className="eyebrow">Workspaces</p><h1>Your local projects</h1><p>Each workspace saves its file rules and scan history on this computer.</p></div><Button onClick={onAdd}>+ Add workspace</Button></header>{state.loading ? <SkeletonCards count={6} variant="workspace" /> : state.error ? <ErrorPanel error={state.error} retry={state.reload} /> : workspaces.length ? <><div className="workspace-filterbar"><Input type="search" aria-label="Search workspaces" placeholder="Search workspaces…" value={search} onChange={(event) => setSearch(event.target.value)} onKeyDown={(event) => { if (event.key === 'Escape') setSearch(''); }} className="workspace-filter-search" /><Input type="search" aria-label="Filter by tag" placeholder="Filter by tag…" value={tagQuery} onChange={(event) => setTagQuery(event.target.value)} onKeyDown={(event) => { if (event.key === 'Escape') setTagQuery(''); }} className="workspace-filter-tag" />{(debouncedTagQuery||debouncedSearch) ? <button type="button" className="text-button workspace-filter-clear" onClick={()=>{ setTagQuery(''); setSearch(''); }}>Clear</button> : null}{/* Only a filtered list needs a result count: with nothing typed it just
          restates the number of cards below it, which is noise. */}
          {(debouncedTagQuery || debouncedSearch) ? <p className="workspace-filter-count" role="status">{visibleWorkspaces.length === workspaces.length ? `${workspaces.length} ${workspaces.length === 1 ? 'workspace' : 'workspaces'}` : `${visibleWorkspaces.length} of ${workspaces.length} ${workspaces.length === 1 ? 'workspace' : 'workspaces'} shown`}</p> : null}</div><WorkspaceSortBar sort={sort} onSort={(key) => setSort((current) => current.key === key ? { key, dir: current.dir === 'asc' ? 'desc' : 'asc' } : { key, dir: firstClickDir[key] })} />{visibleWorkspaces.length ? <div className="workspace-grid">{visibleWorkspaces.map((workspace) => <WorkspaceCard key={workspace.id} workspace={workspace} go={go} notify={notify} onRemoved={state.reload} />)}</div> : <Empty title="No workspaces match this tag" icon={<FolderIcon />}>No workspace tags contain “{tagNeedle}”. Clear the filter to see every project.</Empty>}</> : <><Empty title="No workspaces yet" icon={<FolderIcon />}>Use the folder picker to add a project.</Empty><WorkspaceTemplates onUseTemplate={onAdd} /></>}</div>;
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
const workspaceSortLabels: Record<WorkspaceSortKey, string> = { name: 'Name', last_scan: 'Last scan', findings: 'Findings' };
// Dates and counts read best newest/largest-first, names alphabetically.
const firstClickDir: Record<WorkspaceSortKey, 'asc' | 'desc'> = { name: 'asc', last_scan: 'desc', findings: 'desc' };

function lastScanTime(workspace: Workspace): number | null {
  const time = new Date(workspace.last_scan_at ?? workspace.latest_scan?.finished_at ?? workspace.latest_scan?.started_at ?? '').getTime();
  return Number.isNaN(time) ? null : time;
}

/** Client-side ordering behind the sortable column buttons; workspaces without any scan timestamp always sink to the end. */
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

function WorkspaceSortBar({ sort, onSort }: { sort: WorkspaceSort; onSort: (key: WorkspaceSortKey) => void }) {
  return <fieldset className="workspace-sortbar" aria-label="Sort workspaces"><span>Sort</span>{(Object.keys(workspaceSortLabels) as WorkspaceSortKey[]).map((key) => {
    const active = sort.key === key;
    return <button key={key} type="button" className={`th-sort${active ? ' active' : ''}`} aria-pressed={active} onClick={() => onSort(key)}>{workspaceSortLabels[key]}<span className="sort-arrow" aria-hidden="true">{active ? (sort.dir === 'asc' ? '▲' : '▼') : '↕'}</span>{active && <span className="sr-only"> (sorted {sort.dir === 'asc' ? 'ascending' : 'descending'})</span>}</button>;
  })}</fieldset>;
}

/** Tag chips shown per card: at most three, with any remainder collapsed into a +N chip whose title lists the hidden tags. */
const MAX_CARD_TAGS = 3;

function WorkspaceCard({ workspace, go, notify, onRemoved }: { workspace: Workspace; go: (r: Route) => void; notify?: (n: Notice) => void; onRemoved?: () => void }) {
  const scan = workspace.latest_scan;
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [deleting, setDeleting] = useState(false);
  async function analyze(event?: React.MouseEvent) { event?.stopPropagation(); try { const active = await api.startScan(workspace.id); go({ page: 'scan', id: active.id }); } catch (e) { notify?.({ kind: 'error', text: message(e) }); } }
  async function remove() { setDeleting(true); try { await api.deleteWorkspace(workspace.id); setDeleteOpen(false); notify?.({ kind: 'info', text: 'Workspace removed from Blunt Code.' }); onRemoved?.(); } catch (e) { notify?.({ kind: 'error', text: message(e) }); setDeleting(false); } }
  const findings = scan?.total_findings ?? 0;
  const tags = workspace.tags ?? [];
  const extraTags = tags.length - MAX_CARD_TAGS;
  const risk = workspace.risk;
  const riskTone = risk?.grade === 'A' ? 'success' : risk?.grade === 'B' ? 'warning' : 'failed';
  const langs = workspace.languages ?? [];
  return <><Card className="workspace-card group flex flex-col"><div className="workspace-card-accent" aria-hidden="true" /><div className="workspace-card-inner flex-1 flex flex-col gap-4">
    <div className="workspace-card-head">
      <div className="workspace-card-identity">
        <span className="workspace-card-icon" aria-hidden="true"><ShieldCheck className="h-[18px] w-[18px]" /></span>
        <div className="min-w-0 flex-1">
          <h3 className="truncate">{workspace.name}</h3>
          <PathCopy path={workspace.root_path} />
        </div>
      </div>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button variant="ghost" size="icon" className="workspace-card-menu h-8 w-8 shrink-0" aria-label={`Actions for ${workspace.name}`}>
            <MoreHorizontal className="h-4 w-4" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="min-w-[12rem]">
          <DropdownMenuItem onClick={() => go({ page: 'workspace', id: workspace.id })}>Open details</DropdownMenuItem>
          <DropdownMenuItem onClick={() => void analyze()}>Run scan</DropdownMenuItem>
          <DropdownMenuItem className="text-[var(--color-danger)] focus:text-[var(--color-danger)]" onClick={() => setDeleteOpen(true)}>Remove</DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
    {tags.length > 0 && <ul className="badges workspace-tags flex flex-wrap" aria-label={`${workspace.name} tags`}>{tags.slice(0, MAX_CARD_TAGS).map((tag) => <Badge key={tag} variant="secondary" className="badge rounded-full border-[var(--color-rule-faint)] bg-[var(--color-surface-muted)] text-[var(--color-ink-soft)]">{tag}</Badge>)}{extraTags > 0 && <Badge variant="secondary" className="badge rounded-full border-[var(--color-rule-faint)] bg-[var(--color-surface-muted)] text-[var(--color-ink-soft)]" title={tags.slice(MAX_CARD_TAGS).join(', ')}>+{extraTags}</Badge>}</ul>}
    <section className="workspace-languages">
      <span className="text-[0.68rem] font-mono font-bold tracking-widest uppercase text-[var(--color-ink-faint)]">Languages</span>
      <div className="mt-2">
        {langs.length ? (
          <div className="ws-lang-dots">
            {/* The visible "Languages" caption above labels this row; an aria-label
                on a plain div is dropped by assistive tech anyway. */}
            {langs.map((l) => (
              <span key={l} className="ws-lang-dot"><i aria-hidden="true" style={{ background: languageColor(l) }} />{languageNames[l] ?? l}</span>
            ))}
          </div>
        ) : (
          <span className="muted text-sm">No supported source languages found</span>
        )}
        <div className="sr-only" aria-hidden="true"><LanguageBadges languages={workspace.languages} /></div>
      </div>
    </section>
    <section className="workspace-analysis">
      <div className="flex items-center gap-2 flex-wrap">
        <span className="text-[0.68rem] font-mono font-bold tracking-widest uppercase text-[var(--color-ink-faint)]">Latest analysis</span>
        {risk && <Badge variant={riskTone === 'success' ? 'success' : riskTone === 'warning' ? 'warning' : 'danger'} className={`risk-badge state ${riskTone}`} title={`Weighted risk score ${risk.score}`}>{risk.grade} · {Math.round(risk.score)}</Badge>}
        {scan && <Badge variant={scan.state.includes('completed') ? 'success' : scan.state.includes('failed') ? 'danger' : 'accent'} className={`state ${scan.state}`}>{scan.state.replaceAll('_', ' ')}</Badge>}
      </div>
      {scan ? (
        <>
          <div className="workspace-metric">
            <strong className="tabular-nums">{findings}</strong>
            <span>{findings === 1 ? 'finding' : 'findings'}</span>
          </div>
          <p className="text-sm text-[var(--color-ink-soft)]" title={scan.finished_at ? date(scan.finished_at) : undefined}>{scan.finished_at ? `Completed ${relativeTime(scan.finished_at)}` : 'Analysis in progress'}</p>
        </>
      ) : (
        <p className="mt-2 text-sm text-[var(--color-ink-soft)]">No analysis yet. Run a scan to create the first report.</p>
      )}
      <p className="sr-only">{findings} {findings === 1 ? 'finding' : 'findings'}</p>
    </section>
    <footer>
      <Button variant="ghost" size="sm" className="rounded-[var(--radius-button)]" onClick={() => go({ page: 'workspace', id: workspace.id })}>Open details</Button>
      <Button size="sm" className="ml-auto rounded-[var(--radius-button)] bg-[var(--color-accent)] text-[var(--color-accent-ink)] hover:bg-[var(--color-accent-strong)] shadow-[var(--shadow-accent)]" onClick={() => void analyze()}>Run scan</Button>
      <span className="sr-only">Remove</span>
    </footer>
  </div></Card>{deleteOpen && <ConfirmationDialog title="Remove this workspace?" description="This removes the saved workspace, file rules, and local scan history from Blunt Code. Your project files will not be changed." confirmLabel="Remove workspace" busy={deleting} onCancel={() => setDeleteOpen(false)} onConfirm={remove} />}</>;
}
