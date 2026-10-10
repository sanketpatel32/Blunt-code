import { useState, useEffect } from 'react';
import type { FindingFilter } from '../pages/report/ReportView';

type Preset = { name: string; filters: FindingFilter };

function load(): Preset[] {
  try { const raw = localStorage.getItem('bluntcode.savedFilters'); return raw ? JSON.parse(raw) : []; } catch { return []; }
}
function save(presets: Preset[]) {
  try { localStorage.setItem('bluntcode.savedFilters', JSON.stringify(presets)); } catch {}
}

/**
 * NAMED filter presets for the search sidebar.
 *
 * This used to be a bare "Saved" button anchored to the bottom of the facet
 * stack, opening an absolutely-positioned menu. Two things were wrong with
 * where it sat:
 *
 *   1. It was 400px below the section that already talked about saved
 *      searches. The sidebar has its own "SAVED SEARCHES" panel — with a
 *      "Save current" link, an empty state reading "No saved search presets
 *      yet.", and a list — and then this second, unrelated button below it,
 *      with no heading and no explanation of how it differed. Two features,
 *      two localStorage keys (`savedFilters` for the named presets,
 *      `savedSearches` for a bare query string), one topic.
 *   2. As an `position: absolute` popover inside a sidebar that is itself
 *      inside a page, it could only ever open within the 260px column, so its
 *      preset list was a 14rem-wide menu showing names truncated at ~18
 *      characters.
 *
 * So the owned-by-the-page trigger button is gone and the panel renders where
 * the section is, as plain list rows: the trigger lives in the section header
 * (a real "+ Save current" affordance, disabled when there is nothing to save),
 * and the presets read as full-width rows in the same rhythm as the analyzer
 * chips above them. The text-only `savedSearches` list is gone with it — a
 * preset that remembers only the query string and silently forgets the
 * severity, analyzer and workspace filters you set beside it is the worse
 * version of this same feature.
 */
export function SavedFilters({ filters, onLoad, open, onOpenChange }: { filters: FindingFilter; onLoad: (f: FindingFilter)=>void; /** the save-current form is showing */ open: boolean; onOpenChange: (open: boolean)=>void }) {
  const [presets, setPresets] = useState<Preset[]>(load);
  const [name, setName] = useState('');
  // Persist on every change, so a deleted preset survives a reload the same way
  // an added one does.
  useEffect(() => save(presets), [presets]);
  const add = () => {
    const n = name.trim();
    if (!n) return;
    setPresets((p) => [...p.filter((x) => x.name !== n), { name: n, filters }]);
    setName('');
    onOpenChange(false);
  };
  const remove = (n: string) => setPresets((p) => p.filter((x) => x.name !== n));

  return (
    <div className="saved-presets">
      {presets.length === 0 ? (
        <p className="saved-presets-empty">No saved presets yet.</p>
      ) : (
        <ul className="saved-presets-list" aria-label="Saved filter presets">
          {presets.map((p) => (
            <li key={p.name} className="saved-preset-row">
              <button type="button" className="saved-preset-name" onClick={() => onLoad(p.filters)} title={`Load “${p.name}”`}>
                {p.name}
              </button>
              <button type="button" className="saved-preset-remove" aria-label={`Delete ${p.name}`} onClick={() => remove(p.name)}>
                ×
              </button>
            </li>
          ))}
        </ul>
      )}
      {open && (
        <form className="saved-preset-add" onSubmit={(event) => { event.preventDefault(); add(); }}>
          <input
            value={name}
            onChange={(event) => setName(event.target.value)}
            placeholder="Preset name"
            aria-label="Preset name"
            autoFocus
            maxLength={40}
          />
          <button type="submit" className="button secondary" disabled={!name.trim()}>Save</button>
        </form>
      )}
    </div>
  );
}
