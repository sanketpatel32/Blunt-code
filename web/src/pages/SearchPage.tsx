import { useEffect, useMemo, useState } from 'react';
import { ChevronDown } from 'lucide-react';
import { api } from '../api';
import type { SearchedFinding } from '../types';
import type { Route } from '../lib/router';
import { analyzerName, findingLocation } from '../lib/format';
import { useLoad } from '../hooks/useLoad';
import { useDebouncedValue } from '../hooks/useDebouncedValue';
import { Empty, ErrorPanel } from '../components/ui';
import { MagnifierIcon } from '../components/icons';
import { SkeletonTable } from '../components/skeletons';
import { SavedFilters } from '../components/SavedFilters';
import type { FindingFilter } from './report/ReportView';
import { QueryBuilder } from '../components/QueryBuilder';
import { filterToQueryGroup, queryGroupToFilter, type QueryGroup } from '../lib/queryBuilder';
import { Sheet, SheetContent, SheetHeader, SheetTitle, SheetDescription } from '../components/ui/sheet';

const SEARCH_PAGE_SIZE = 25;
const SEARCH_DEBOUNCE_MS = 250;
const SEVERITIES = ['critical', 'high', 'medium', 'low', 'info'] as const;
const ANALYZERS = ['ruff', 'biome', 'semgrep', 'sonarqube', 'secrets', 'todo'] as const;
const STATUSES = ['new', 'persistent'] as const;
/** Column key -> label shown in the column picker. The key is state, the label is UI. */
const SEARCH_COLUMNS = [
  ['severity', 'Severity'],
  ['finding', 'Finding'],
  ['location', 'Location'],
  ['actions', 'Actions'],
] as const;

function useSavedSearches() {
  const key = 'bluntcode.savedSearches';
  const load = (): string[] => { try { const r=localStorage.getItem(key); return r?JSON.parse(r):[]; } catch { return []; } };
  const [list, setList] = useState<string[]>(load);
  const add = (q:string) => { const t=q.trim(); if(!t||list.includes(t)) return; const nxt=[t,...list].slice(0,10); setList(nxt); try{localStorage.setItem(key, JSON.stringify(nxt));}catch{} };
  const remove = (q:string) => { const nxt=list.filter(x=>x!==q); setList(nxt); try{localStorage.setItem(key, JSON.stringify(nxt));}catch{} };
  return { list, add, remove };
}

export function SearchPage({ go }: { go: (route: Route) => void }) {
  const [query, setQuery] = useState(() => new URLSearchParams(window.location.search).get('q') ?? '');
  const [severities, setSeverities] = useState<ReadonlySet<string>>(() => {
    const v=new URLSearchParams(window.location.search).get('severity'); return new Set(v?v.split(',').filter(Boolean):[]);
  });
  const [analyzer, setAnalyzer] = useState(() => new URLSearchParams(window.location.search).get('analyzer') ?? '');
  const [status, setStatus] = useState(() => new URLSearchParams(window.location.search).get('status') ?? '');
  const [workspace, setWorkspace] = useState(() => new URLSearchParams(window.location.search).get('workspace') ?? '');
  const [pathPrefix, setPathPrefix] = useState(() => new URLSearchParams(window.location.search).get('path') ?? '');
  const [visibleCols, setVisibleCols] = useState({ severity:true, finding:true, location:true, actions:true });
  const [page, setPage] = useState(() => { const p=Number(new URLSearchParams(window.location.search).get('page')); return Number.isFinite(p)&&p>=1?Math.floor(p):1; });
  const [facetsOpen, setFacetsOpen] = useState(false);
  const [advancedOpen, setAdvancedOpen] = useState(false);
  const [acc, setAcc] = useState({ sev:true, ana:true, ws:true, st:true });
  const [queryGroup, setQueryGroup] = useState<QueryGroup>(() => filterToQueryGroup({ severity:[...(() => { const v=new URLSearchParams(window.location.search).get('severity'); return new Set(v?v.split(',').filter(Boolean):[]); })()].join(','), category:'', analyzer:new URLSearchParams(window.location.search).get('analyzer')??'', rule:'', path:new URLSearchParams(window.location.search).get('workspace')??'', status:new URLSearchParams(window.location.search).get('status')??'', q:new URLSearchParams(window.location.search).get('q')??'' } as FindingFilter));
  const debouncedQuery = useDebouncedValue(query, query ? SEARCH_DEBOUNCE_MS : 0);

  const params = useMemo(() => {
    const value: Record<string, string> = { page: String(page), page_size: String(SEARCH_PAGE_SIZE) };
    if (debouncedQuery) value.q = debouncedQuery;
    if (analyzer) value.analyzer = analyzer;
    if (severities.size) value.severity = [...severities].join(',');
    if (status) value.status = status;
    if (workspace) value.workspace = workspace;
    if (pathPrefix) value.path = pathPrefix;
    return value;
  }, [debouncedQuery, severities, analyzer, status, workspace, pathPrefix, page]);

  useEffect(() => { setPage(1); }, [debouncedQuery, severities, analyzer, status, workspace, pathPrefix]);

  // URL sync
  useEffect(() => {
    const sp=new URLSearchParams();
    if(debouncedQuery) sp.set('q', debouncedQuery);
    if(analyzer) sp.set('analyzer', analyzer);
    if(severities.size) sp.set('severity', [...severities].join(','));
    if(status) sp.set('status', status);
    if(workspace) sp.set('workspace', workspace);
    if(pathPrefix) sp.set('path', pathPrefix);
    if(page!==1) sp.set('page',String(page));
    const qs=sp.toString();
    const cur=window.location.search.replace(/^\?/,'');
    if(qs===cur) return;
    const next= qs? `${window.location.pathname}?${qs}${window.location.hash}` : `${window.location.pathname}${window.location.hash}`;
    window.history.replaceState(null,'',next);
  }, [debouncedQuery, analyzer, severities, status, workspace, pathPrefix, page]);
  useEffect(()=>{
    const onPop=()=>{
      const sp=new URLSearchParams(window.location.search);
      setQuery(sp.get('q')??'');
      setAnalyzer(sp.get('analyzer')??'');
      setStatus(sp.get('status')??'');
      setWorkspace(sp.get('workspace')??'');
      setPathPrefix(sp.get('path')??'');
      const v=sp.get('severity'); setSeverities(new Set(v?v.split(',').filter(Boolean):[]));
      const p=Number(sp.get('page')); setPage(Number.isFinite(p)&&p>=1?Math.floor(p):1);
      setQueryGroup(filterToQueryGroup({ severity:v??'', category:'', analyzer:sp.get('analyzer')??'', rule:'', path:sp.get('workspace')??'', status:sp.get('status')??'', q:sp.get('q')??'' } as FindingFilter));
    };
    window.addEventListener('popstate',onPop); return()=>window.removeEventListener('popstate',onPop);
  },[]);
  useEffect(()=>{ setQueryGroup(filterToQueryGroup({ severity:[...severities].join(','), category:'', analyzer, rule:'', path:workspace, status, q:query } as FindingFilter)); },[severities, analyzer, status, workspace, query]);

  const state = useLoad(() => api.searchFindings(params), [params.q, params.severity, params.analyzer, params.page, params.status]);
  const hiddenColumnCount = SEARCH_COLUMNS.filter(([key]) => !visibleCols[key]).length;
  const items = state.data?.items ?? [];
  const total = state.data?.total ?? 0;
  const pageSize = state.data?.page_size ?? SEARCH_PAGE_SIZE;
  const first = (page - 1) * pageSize;
  const saved = useSavedSearches();

  const toggleSeverity = (severity: string) => setSeverities((current) => {
    const next = new Set(current);
    if (next.has(severity)) next.delete(severity); else next.add(severity);
    return next;
  });

  // live severity counts from current result set
  const severityCounts = useMemo(()=>{
    const c:Record<string,number>={critical:0,high:0,medium:0,low:0,info:0};
    for(const f of items) c[f.severity]=(c[f.severity]??0)+1;
    return c;
  },[items]);

  const clearAll = () => { setQuery(''); setSeverities(new Set()); setAnalyzer(''); setStatus(''); setWorkspace(''); setPathPrefix(''); setPage(1); };

  const facets = (
    <div className="search-facets">
      <div className="facet-section">
        <button type="button" className="facet-head" aria-expanded={acc.sev} onClick={()=>setAcc(a=>({...a,sev:!a.sev}))}>Severity {acc.sev?'−':'+'}</button>
        {acc.sev && <div className="severity-pills" role="group" aria-label="Filter by severity">
          {SEVERITIES.map((severity) => (
            <button key={severity} type="button" className={`severity-pill ${severities.has(severity) ? 'selected' : ''}`} aria-pressed={severities.has(severity)} onClick={() => toggleSeverity(severity)}>{severity}<span className="count">{severityCounts[severity]??0}</span></button>
          ))}
        </div>}
      </div>
      <div className="facet-section">
        <button type="button" className="facet-head" aria-expanded={acc.ana} onClick={()=>setAcc(a=>({...a,ana:!a.ana}))}>Analyzer {acc.ana?'−':'+'}</button>
        {acc.ana && <select className="search-analyzer" aria-label="Filter by analyzer" value={analyzer} onChange={(event) => setAnalyzer(event.target.value)}>
          <option value="">All analyzers</option>
          {ANALYZERS.map((id) => <option key={id} value={id}>{analyzerName(id)}</option>)}
        </select>}
      </div>
      <div className="facet-section">
        <button type="button" className="facet-head" aria-expanded={acc.ws} onClick={()=>setAcc(a=>({...a,ws:!a.ws}))}>Workspace {acc.ws?'−':'+'}</button>
        {acc.ws && <input value={workspace} onChange={e=>setWorkspace(e.target.value)} placeholder="Workspace id" aria-label="Filter by workspace" />}
      </div>
      <div className="facet-section">
        <button type="button" className="facet-head" aria-expanded={acc.st} onClick={()=>setAcc(a=>({...a,st:!a.st}))}>Status {acc.st?'−':'+'}</button>
        {acc.st && <select aria-label="Filter by status" value={status} onChange={e=>setStatus(e.target.value)}>
          <option value="">All</option>
          {STATUSES.map(s=><option key={s} value={s}>{s}</option>)}
        </select>}
      </div>
      <div className="facet-saved">
        <p className="muted">Saved searches</p>
        {saved.list.length===0 ? <span className="muted">No saved searches</span> : saved.list.map(q=>(
          <div key={q} className="saved-preset-row"><button type="button" onClick={()=>setQuery(q)}>{q}</button><button type="button" aria-label={`Delete ${q}`} onClick={()=>saved.remove(q)}>×</button></div>
        ))}
        {query.trim() && <button type="button" className="button secondary" onClick={()=>saved.add(query)}>Save search</button>}
      </div>
      <SavedFilters filters={{severity:[...severities].join(','), analyzer, q:query, status, category:'', rule:'', path:workspace} as FindingFilter} onLoad={(f)=>{ setSeverities(new Set(f.severity?f.severity.split(','):[])); setAnalyzer(f.analyzer); setQuery(f.q); setStatus(f.status); setWorkspace(f.path); }} />
    </div>
  );

  return <div className="page search-page">
    <header className="page-heading">
      <div>
        <p className="eyebrow">Search</p>
        <h1>Find findings everywhere</h1>
        <p>Searches every stored scan on this computer. Suppressed findings stay hidden.</p>
      </div>
      <div className="search-page-actions flex items-center gap-2">
        <button type="button" className="button secondary" onClick={()=>setAdvancedOpen(true)} aria-expanded={advancedOpen} aria-controls="search-advanced">Advanced</button>
        <button type="button" className="button secondary md:hidden" onClick={()=>setFacetsOpen(v=>!v)}>Filters</button>
        {(query||analyzer||severities.size||status||workspace||pathPrefix) && <button type="button" className="text-button" onClick={clearAll}>Clear</button>}
      </div>
    </header>
    <div className="search-toolbar" role="search">
      <input type="search" className="search-input" placeholder="Search message, rule, or path…" aria-label="Search findings" value={query} onChange={(event) => setQuery(event.target.value)} />
    </div>
    <Sheet open={advancedOpen} onOpenChange={setAdvancedOpen}><SheetContent side="right" className="w-full max-w-[420px] sm:max-w-[420px] overflow-y-auto" aria-label="Global search query builder" id="search-advanced"><SheetHeader><SheetTitle>Advanced search</SheetTitle><SheetDescription>Workspace scope + severity query builder. Apply writes to global search URL and results.</SheetDescription></SheetHeader><div className="mt-4"><QueryBuilder group={queryGroup} onChange={setQueryGroup} onApply={(f)=>{ setSeverities(new Set(f.severity?f.severity.split(','):[])); setAnalyzer(f.analyzer); setQuery(f.q); setStatus(f.status); setWorkspace(f.path); setAdvancedOpen(false); }} facetCounts={{ severity: severityCounts as Record<string,number> }} analyzers={[...ANALYZERS]} /></div></SheetContent></Sheet>
    <div className="search-layout">
      <aside className="search-sidebar desktop-only">{facets}</aside>
      {facetsOpen && <div className="filter-drawer-backdrop" role="presentation" onMouseDown={e=>{ if(e.target===e.currentTarget) setFacetsOpen(false); }}><aside role="dialog" aria-modal="true" aria-label="Filters" className="filter-drawer">{facets}<button type="button" className="button secondary" onClick={()=>setFacetsOpen(false)}>Close</button></aside></div>}
      <div className="search-main">
        {state.loading ? <SkeletonTable rows={8} cols={4} />
          : state.error ? <ErrorPanel error={state.error} retry={state.reload} />
            : total === 0 ? <Empty title="No matching findings" icon={<MagnifierIcon />}>Run a scan or loosen the filters — only scans already stored on this computer are searched.</Empty>
              : <>
                <div className="table-wrap overflow-x-auto overscroll-x-contain">
                  {/* Hiding a column is a taste call most people never make, so
                      it no longer sits open above every result set — and the
                      labels are words, not the internal state keys. */}
                  <details className="search-columns">
                    <summary className="search-columns-toggle">
                      <ChevronDown className="search-columns-caret" aria-hidden="true" />
                      Columns
                      <small>{hiddenColumnCount ? `${hiddenColumnCount} hidden` : 'All shown'}</small>
                    </summary>
                    <div className="search-columns-panel">
                      {SEARCH_COLUMNS.map(([key, label]) => (
                        <label key={key} className="search-column-toggle">
                          <input type="checkbox" checked={visibleCols[key]} onChange={() => setVisibleCols(v=>({...v,[key]:!v[key]}))} />
                          {label}
                        </label>
                      ))}
                    </div>
                  </details>
                <table className="search-results"><caption className="sr-only">Global search results</caption><thead className="sticky top-0 z-[1] bg-[var(--color-surface-muted)]"><tr>{visibleCols.severity && <th scope="col">Severity</th>}{visibleCols.finding && <th scope="col">Finding</th>}{visibleCols.location && <th scope="col">Location</th>}{visibleCols.actions && <th scope="col"><span className="sr-only">Actions</span></th>}</tr></thead><tbody>
                  {items.map((finding: SearchedFinding) => <tr key={`${finding.scan_id}:${finding.id}`}>
                    <td><span className={`severity ${finding.severity}`}>{finding.severity}</span></td>
                    <td className="finding-summary">{finding.title ? <strong>{finding.title}</strong> : null}<span>{finding.message}</span>{finding.rule_id ? <code className="badge">{finding.rule_id}</code> : null}</td>
                    <td><code>{findingLocation(finding)}</code></td>
                    <td><a href={`/scans/${finding.scan_id}`} onClick={(event) => { event.preventDefault(); go({ page: 'scan', id: finding.scan_id }); }}>Open report</a></td>
                  </tr>)}
                </tbody></table></div>
                <nav className="findings-pagination" aria-label="Search result pagination">
                  <span>Showing {total === 0 ? 0 : first + 1}–{Math.min(first + items.length, total)} of {total}</span>
                  <div>
                    <button type="button" className="button secondary" onClick={() => setPage(page - 1)} disabled={page <= 1}>Previous</button>
                    <output aria-live="polite">Page {page}</output>
                    <button type="button" className="button secondary" onClick={() => setPage(page + 1)} disabled={!state.data?.has_next}>Next</button>
                  </div>
                </nav>
              </>}
      </div>
    </div>
  </div>;
}
