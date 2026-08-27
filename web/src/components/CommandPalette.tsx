import { useEffect, useMemo, useRef, useState } from 'react';
import { useDialogA11y } from '../hooks/useDialogA11y';

export type Command = {
  id: string;
  label: string;
  keywords?: string;
  hint?: string;
  group?: string;
  run: () => void;
};

export function filterCommands(commands: Command[], query: string): Command[] {
  const q = query.trim().toLowerCase();
  if (!q) return commands;
  const starts: Command[] = [];
  const includes: Command[] = [];
  for (const command of commands) {
    const haystack = `${command.label} ${command.keywords ?? ''}`.toLowerCase();
    if (command.label.toLowerCase().startsWith(q)) starts.push(command);
    else if (haystack.includes(q)) includes.push(command);
  }
  return [...starts, ...includes];
}

export function CommandPalette({ open, onClose, commands, note }: { open: boolean; onClose: () => void; commands: Command[]; note?: string }) {
  const [query, setQuery] = useState('');
  const [activeIndex, setActiveIndex] = useState(0);
  const inputRef = useRef<HTMLInputElement>(null);
  const listRef = useRef<HTMLDivElement>(null);
  const { dialogRef } = useDialogA11y({ onClose, autoFocusRef: inputRef });
  const recent = useMemo(()=>{
    try { const r=localStorage.getItem('bluntcode.recentSearches'); return r?JSON.parse(r):[]; } catch { return []; }
  },[open]);
  const results = useMemo(() => filterCommands(commands, query), [commands, query]);
  useEffect(() => { if (open) { setQuery(''); setActiveIndex(0); } }, [open]);
  useEffect(() => { if (activeIndex >= results.length) setActiveIndex(Math.max(0, results.length - 1)); }, [results.length, activeIndex]);
  useEffect(() => {
    const options = listRef.current?.querySelectorAll<HTMLElement>('[role="option"]');
    const active = options?.[Math.min(activeIndex, Math.max(0, options.length - 1))];
    active?.scrollIntoView?.({ block: 'nearest' });
  }, [activeIndex]);
  // group results — must stay before early return (rules of hooks)
  const grouped = useMemo(()=>{
    const map=new Map<string, Command[]>();
    for(const c of results){ const g=c.group ?? 'Navigation'; if(!map.has(g)) map.set(g,[]); map.get(g)!.push(c); }
    return [...map.entries()];
  },[results]);
  if (!open) return null;
  const run = (command: Command) => {
    try { const cur=JSON.parse(localStorage.getItem('bluntcode.recentSearches')??'[]'); const nxt=[command.label,...cur.filter((x:string)=>x!==command.label)].slice(0,5); localStorage.setItem('bluntcode.recentSearches',JSON.stringify(nxt)); } catch {}
    onClose(); command.run();
  };
  const onKeyDown = (event: React.KeyboardEvent) => {
    if (event.key === 'ArrowDown') { event.preventDefault(); setActiveIndex((i) => Math.min(i + 1, results.length - 1)); }
    else if (event.key === 'ArrowUp') { event.preventDefault(); setActiveIndex((i) => Math.max(i - 1, 0)); }
    else if (event.key === 'Enter') { event.preventDefault(); if (results[activeIndex]) run(results[activeIndex]); }
  };
  let globalIndex=0;
  return (
    <div role="presentation" className="dialog-backdrop palette-backdrop" onMouseDown={(event) => { if (event.target === event.currentTarget) onClose(); }}>
    <dialog ref={dialogRef} open aria-label="Command palette" className="command-palette">
      <input ref={inputRef} value={query} onChange={(event) => { setQuery(event.target.value); setActiveIndex(0); }} onKeyDown={onKeyDown} placeholder="Type a command…" role="combobox" aria-expanded="true" aria-controls="command-palette-list" aria-activedescendant={results[activeIndex] ? `command-option-${results[activeIndex].id}` : undefined} aria-label="Search commands" spellCheck={false} autoComplete="off" />
      {recent.length>0 && !query && <div className="palette-recent"><span className="muted">Recent searches</span>{recent.map((r:string)=><span key={r} className="badge">{r}</span>)}</div>}
      <div id="command-palette-list" ref={listRef} role="listbox" aria-label="Commands">
        {grouped.map(([group, cmds])=>(
          <div key={group} className="palette-group">
            <span className="palette-group-title">{group}</span>
            {cmds.map((command)=>{
              const idx=globalIndex++;
              return <button type="button" key={command.id} id={`command-option-${command.id}`} role="option" className="palette-option" data-active={idx === activeIndex} aria-selected={idx === activeIndex} onMouseEnter={() => setActiveIndex(idx)} onClick={() => run(command)}>
                <span className="palette-label">{command.label}</span>
                {command.hint && <kbd>{command.hint}</kbd>}
              </button>;
            })}
          </div>
        ))}
        {!results.length && <p className="palette-empty" aria-live="polite">No matching command.</p>}
      </div>
      <footer><span><kbd>↑</kbd><kbd>↓</kbd> navigate</span><span><kbd>Enter</kbd> run</span><span><kbd>Esc</kbd> close</span>{note && <span className="palette-note">{note}</span>}<span className="palette-demo">8-state demo: navigation, filters, workspaces</span></footer>
    </dialog>
  </div>);
}
