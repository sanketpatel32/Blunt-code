import { useRef } from 'react';
import { useDialogA11y } from '../hooks/useDialogA11y';

/** One row of the help list: the keys that trigger it and what they do. */
interface ShortcutEntry {
  keys: string[];
  description: string;
}

const SHORTCUT_GROUPS: Array<{ title: string; entries: ShortcutEntry[] }> = [
  {
    title: 'Navigation',
    entries: [
      { keys: ['Ctrl', 'K'], description: 'Open the command palette' },
      { keys: ['g', 'h'], description: 'Go to Home' },
      { keys: ['g', 'w'], description: 'Go to Workspaces' },
      { keys: ['g', 't'], description: 'Go to Tools' },
      { keys: ['g', 's'], description: 'Go to Settings' },
    ],
  },
  {
    title: 'Actions',
    entries: [
      { keys: ['n'], description: 'Add a workspace' },
      { keys: ['?'], description: 'Show this shortcuts help' },
      { keys: ['Esc'], description: 'Close a dialog' },
    ],
  },
  {
    title: 'Search',
    entries: [
      { keys: ['/'], description: 'Focus the findings or file search' },
    ],
  },
];

/**
 * The "?" help dialog. Uses the shared dialog a11y hook for focus trap,
 * Escape, backdrop close and focus restore. Its dialog element carries
 * `data-shortcuts-dialog` so App's shortcut listener can keep working while
 * this dialog is the only one open.
 */
export function ShortcutsDialog({ onClose }: { onClose: () => void }) {
  const gotItRef = useRef<HTMLButtonElement>(null);
  const { dialogRef, onBackdropMouseDown } = useDialogA11y({ onClose, autoFocusRef: gotItRef });
  return (
    // biome-ignore lint/a11y/noStaticElementInteractions: backdrop-only dismissal is pointer convenience; keyboard users close via Escape and the Got it button.
    <div className="dialog-backdrop" role="presentation" onMouseDown={onBackdropMouseDown}>
    <dialog ref={dialogRef} open aria-modal="true" aria-labelledby="shortcuts-title" data-shortcuts-dialog className="shortcuts-dialog"><div className="confirmation-dialog">
      <header><h2 id="shortcuts-title">Keyboard shortcuts</h2><button type="button" className="icon-button" onClick={onClose} aria-label="Close shortcuts help">×</button></header>
      <p>Sequences start with <kbd className="kbd-hint">g</kbd> then the next key within a moment. Shortcuts pause while you are typing in a field or a dialog is open — <kbd className="kbd-hint">Ctrl</kbd><kbd className="kbd-hint">K</kbd> works everywhere.</p>
      <div className="shortcut-groups">
        {SHORTCUT_GROUPS.map((group) => <section key={group.title} className="shortcut-group">
          <h3>{group.title}</h3>
          <dl>{group.entries.map((entry) => <div className="shortcut-row" key={entry.description}>
            <dt>{entry.keys.map((key) => <kbd className="kbd-hint" key={key}>{key}</kbd>)}</dt>
            <dd>{entry.description}</dd>
          </div>)}</dl>
        </section>)}
      </div>
      <footer><button ref={gotItRef} type="button" className="button primary" onClick={onClose}>Got it</button></footer>
    </div></dialog>
  </div>);
}
