import { useEffect, useMemo, useRef, useState } from 'react';
import { useDialogA11y } from '../hooks/useDialogA11y';

export type Command = {
  id: string;
  label: string;
  /** Extra match text; never rendered. */
  keywords?: string;
  hint?: string;
  run: () => void;
};

/** Case-insensitive substring filter over label + keywords, ranked
 *  startsWith-before-includes so "ho" puts "Home" above "Show shortcuts". */
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

/** Ctrl/Cmd+K launcher: type to filter, arrows to move, Enter to run.
 *  Focus lives in the input the whole time; the list is plain buttons driven by
 *  aria-activedescendant so screen readers announce the highlighted command.
 *  `note` is an optional passive status line in the footer (e.g. how many
 *  workspace entries were lazily loaded) — purely informational, never focusable. */
export function CommandPalette({ open, onClose, commands, note }: { open: boolean; onClose: () => void; commands: Command[]; note?: string }) {
  const [query, setQuery] = useState('');
  const [activeIndex, setActiveIndex] = useState(0);
  const inputRef = useRef<HTMLInputElement>(null);
  const listRef = useRef<HTMLDivElement>(null);
  const { dialogRef } = useDialogA11y({ onClose, autoFocusRef: inputRef });
  const results = useMemo(() => filterCommands(commands, query), [commands, query]);
  useEffect(() => { if (open) { setQuery(''); setActiveIndex(0); } }, [open]);
  // Keep the highlight clamped when filtering shrinks the list under it.
  useEffect(() => { if (activeIndex >= results.length) setActiveIndex(Math.max(0, results.length - 1)); }, [results.length, activeIndex]);
  useEffect(() => {
    const options = listRef.current?.querySelectorAll<HTMLElement>('[role="option"]');
    const active = options?.[Math.min(activeIndex, Math.max(0, options.length - 1))];
    active?.scrollIntoView?.({ block: 'nearest' });
  }, [activeIndex]);
  if (!open) return null;
  const run = (command: Command) => { onClose(); command.run(); };
  const onKeyDown = (event: React.KeyboardEvent) => {
    if (event.key === 'ArrowDown') { event.preventDefault(); setActiveIndex((i) => Math.min(i + 1, results.length - 1)); }
    else if (event.key === 'ArrowUp') { event.preventDefault(); setActiveIndex((i) => Math.max(i - 1, 0)); }
    else if (event.key === 'Enter') { event.preventDefault(); if (results[activeIndex]) run(results[activeIndex]); }
  };
  return (
    // biome-ignore lint/a11y/noStaticElementInteractions: backdrop-only dismissal is pointer convenience; keyboard users close via Escape.
    <div role="presentation" className="dialog-backdrop palette-backdrop" onMouseDown={(event) => { if (event.target === event.currentTarget) onClose(); }}>
    <dialog ref={dialogRef} open aria-label="Command palette" className="command-palette">
      <input ref={inputRef} value={query} onChange={(event) => { setQuery(event.target.value); setActiveIndex(0); }} onKeyDown={onKeyDown} placeholder="Type a command…" role="combobox" aria-expanded="true" aria-controls="command-palette-list" aria-activedescendant={results[activeIndex] ? `command-option-${results[activeIndex].id}` : undefined} aria-label="Search commands" spellCheck={false} autoComplete="off" />
      <div id="command-palette-list" ref={listRef} role="listbox" aria-label="Commands">
        {results.map((command, index) => <button type="button" key={command.id} id={`command-option-${command.id}`} role="option" className="palette-option" data-active={index === activeIndex} aria-selected={index === activeIndex} onMouseEnter={() => setActiveIndex(index)} onClick={() => run(command)}>
          <span className="palette-label">{command.label}</span>
          {command.hint && <kbd>{command.hint}</kbd>}
        </button>)}
        {!results.length && <p className="palette-empty" aria-live="polite">No matching command.</p>}
      </div>
      <footer><span><kbd>↑</kbd><kbd>↓</kbd> navigate</span><span><kbd>Enter</kbd> run</span><span><kbd>Esc</kbd> close</span>{note && <span className="palette-note">{note}</span>}</footer>
    </dialog>
  </div>);
}
