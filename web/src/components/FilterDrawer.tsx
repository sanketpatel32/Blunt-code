import { useEffect, useState } from 'react';
import type { FindingFilter, SortState, SortKey } from '../pages/report/ReportView';
import { analyzerName } from '../lib/format';

const SEVERITIES = ['critical', 'high', 'medium', 'low', 'info'] as const;
const STATUSES = ['', 'new', 'persistent', 'suppressed'] as const;

export function FilterDrawer({ open, onClose, filters, setFilters, sort, setSort, analyzers, rules, pageKey, onApply }: {
  open: boolean;
  onClose: () => void;
  filters: FindingFilter;
  setFilters: (f: FindingFilter) => void;
  sort: SortState;
  setSort: (s: SortState) => void;
  analyzers: string[];
  rules: string[];
  pageKey: string;
  onApply: () => void;
}) {
  const [draft, setDraft] = useState<FindingFilter>(filters);
  const [draftSort, setDraftSort] = useState<SortState>(sort);
  useEffect(() => { if (open) { setDraft(filters); setDraftSort(sort); } }, [open, filters, sort]);
  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => { if (e.key === 'Escape') onClose(); };
    document.addEventListener('keydown', onKey);
    return () => document.removeEventListener('keydown', onKey);
  }, [open, onClose]);

  const apply = () => {
    setFilters(draft);
    setSort(draftSort);
    try { localStorage.setItem(`bluntcode.filters.${pageKey}`, JSON.stringify({ filters: draft, sort: draftSort })); } catch {}
    onApply();
    onClose();
  };
  const reset = () => {
    const empty: FindingFilter = { severity: '', category: '', analyzer: '', rule: '', path: '', status: '', q: '' };
    setDraft(empty);
  };
  const toggleSeverity = (s: string) => setDraft(d => ({ ...d, severity: d.severity === s ? '' : s }));

  if (!open) return null;
  return (
    <div role="presentation" className="filter-drawer-backdrop" onMouseDown={e => { if (e.target === e.currentTarget) onClose(); }} style={{ backdropFilter: 'blur(8px)', WebkitBackdropFilter: 'blur(8px)' }}>
      <aside role="dialog" aria-modal="true" aria-label="Advanced filters" className="filter-drawer" style={{ animation: 'anim-filterDrawerIn 200ms var(--ease-out-quart) both' }}>
        <header className="filter-drawer-head">
          <h2>Filters</h2>
          <button type="button" className="icon-button" aria-label="Close filters" onClick={onClose}>×</button>
        </header>
        <div className="filter-drawer-body">
          <fieldset>
            <legend>Severity</legend>
            <div className="severity-pills" role="group" aria-label="Severity">
              {SEVERITIES.map(s => (
                <button key={s} type="button" className={`severity-pill ${draft.severity===s?'selected':''}`} aria-pressed={draft.severity===s} onClick={()=>toggleSeverity(s)}>{s}</button>
              ))}
            </div>
          </fieldset>

          <div className="filter-field">
            <span className="filters-field-label">Analyzer</span>
            <fieldset className="chip-group" aria-label="Analyzer">
              <button type="button" className="chip" aria-pressed={draft.analyzer === ''} onClick={()=>setDraft(d=>({...d, analyzer:''}))}>All analyzers</button>
              {analyzers.map(a=><button key={a} type="button" className="chip" aria-pressed={draft.analyzer===a} onClick={()=>setDraft(d=>({...d, analyzer:a}))}>{analyzerName(a)}</button>)}
            </fieldset>
          </div>

          <label>Rule
            <input list="filter-rule-list" value={draft.rule} onChange={e=>setDraft(d=>({...d, rule:e.target.value}))} placeholder="All rules" />
            <datalist id="filter-rule-list">{rules.map(r=><option key={r} value={r} />)}</datalist>
          </label>

          <label>Path prefix
            <input value={draft.path} onChange={e=>setDraft(d=>({...d, path:e.target.value}))} placeholder="src/" />
          </label>

          <label>Category
            <input value={draft.category} onChange={e=>setDraft(d=>({...d, category:e.target.value}))} placeholder="All categories" />
          </label>

          <div className="filter-field">
            <span className="filters-field-label">Status</span>
            <fieldset className="chip-group" aria-label="Status">
              <button type="button" className="chip" aria-pressed={draft.status === ''} onClick={()=>setDraft(d=>({...d, status:''}))}>All</button>
              {STATUSES.filter(Boolean).map(s=><button key={s} type="button" className="chip" aria-pressed={draft.status===s} onClick={()=>setDraft(d=>({...d, status:s}))}>{s}</button>)}
            </fieldset>
          </div>

          <label>Search
            <input value={draft.q} onChange={e=>setDraft(d=>({...d, q:e.target.value}))} placeholder="Message or rule" />
          </label>

          <div className="filter-drawer-sort">
            <label>Sort by
              <select value={draftSort.key} onChange={e=>setDraftSort(d=>({...d, key:e.target.value as SortKey}))}>
                <option value="severity">Severity</option>
                <option value="path">Path</option>
                <option value="analyzer">Analyzer</option>
                <option value="status">Status</option>
              </select>
            </label>
            <label>Direction
              <select value={draftSort.dir} onChange={e=>setDraftSort(d=>({...d, dir:e.target.value as 'asc'|'desc'}))}>
                <option value="asc">Ascending</option>
                <option value="desc">Descending</option>
              </select>
            </label>
          </div>
        </div>
        <footer className="filter-drawer-foot">
          <button type="button" className="button secondary" onClick={reset}>Reset</button>
          <button type="button" className="button primary" onClick={apply}>Apply</button>
        </footer>
      </aside>
    </div>
  );
}
