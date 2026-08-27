import { useState, useEffect } from 'react';
import type { FindingFilter } from '../pages/report/ReportView';

type Preset = { name: string; filters: FindingFilter };

function load(): Preset[] {
  try { const raw = localStorage.getItem('bluntcode.savedFilters'); return raw ? JSON.parse(raw) : []; } catch { return []; }
}
function save(presets: Preset[]) {
  try { localStorage.setItem('bluntcode.savedFilters', JSON.stringify(presets)); } catch {}
}

export function SavedFilters({ filters, onLoad }: { filters: FindingFilter; onLoad: (f: FindingFilter)=>void }) {
  const [presets, setPresets] = useState<Preset[]>(load);
  const [name, setName] = useState('');
  const [open, setOpen] = useState(false);
  useEffect(()=>save(presets),[presets]);
  const add = () => {
    const n = name.trim();
    if (!n) return;
    setPresets(p=>[...p.filter(x=>x.name!==n), { name:n, filters }]);
    setName(''); setOpen(false);
  };
  const remove = (n: string) => setPresets(p=>p.filter(x=>x.name!==n));
  return (
    <div className="saved-filters">
      <button type="button" className="button secondary" aria-haspopup="menu" aria-expanded={open} onClick={()=>setOpen(v=>!v)}>Saved</button>
      {open && (
        <div className="saved-filters-menu" role="menu">
          {presets.length===0 ? <span className="muted">No saved presets</span> : presets.map(p=>(
            <div key={p.name} className="saved-preset-row">
              <button type="button" role="menuitem" onClick={()=>{ onLoad(p.filters); setOpen(false); }}>{p.name}</button>
              <button type="button" aria-label={`Delete ${p.name}`} onClick={()=>remove(p.name)}>×</button>
            </div>
          ))}
          <div className="saved-preset-add">
            <input value={name} onChange={e=>setName(e.target.value)} placeholder="Preset name" aria-label="Preset name" />
            <button type="button" className="button secondary" onClick={add} disabled={!name.trim()}>Save current</button>
          </div>
        </div>
      )}
    </div>
  );
}
