import type { Severity } from '../types';

export function QuickFilters({ selected, counts, onSelect }: {
  selected: string;
  counts: Record<Severity, number>;
  onSelect: (severity: string) => void;
}) {
  const items: Array<{ key: string; label: string; count?: number }> = [
    { key: '', label: 'All' },
    { key: 'critical', label: 'Critical', count: counts.critical },
    { key: 'high', label: 'High', count: counts.high },
    { key: 'needs-review', label: 'Needs review' },
    { key: 'new', label: 'New' },
  ];
  return (
    <div className="quick-filters" role="group" aria-label="Quick filters">
      {items.map(it => {
        const active = selected === it.key || (it.key==='' && !selected);
        return (
          <button key={it.key||'all'} type="button" className={`quick-pill ${active?'selected':''}`} aria-pressed={active} onClick={()=>onSelect(it.key)}>
            {it.label}{it.count!==undefined ? <span className="count">{it.count}</span> : null}
          </button>
        );
      })}
    </div>
  );
}
