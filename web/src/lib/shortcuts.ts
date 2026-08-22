/**
 * Pure helpers for the app's global keyboard shortcuts. Kept free of DOM
 * focus/navigation side effects so they can be unit tested with plain events;
 * the App component owns the wiring (routing, dialogs, focus moves).
 */

/** Every key the app reacts to on its own; sequence targets ('h'…) only act after 'g' arms. */
export type ShortcutKey = 'g' | 'h' | 'w' | 't' | 's' | 'n' | '/' | '?';

const SHORTCUT_KEYS: ReadonlySet<string> = new Set(['g', 'h', 'w', 't', 's', 'n', '/', '?']);

/**
 * Normalizes a keydown event to one of the app's single-key shortcuts, or null
 * when the key is not a shortcut (multi-char keys like "Escape") or is combined
 * with a modifier (Ctrl/Cmd/Alt combos belong to the browser and screen
 * readers; Shift is allowed because "?" itself is Shift+/).
 */
/** Structural slice of KeyboardEvent; modifiers are optional so tests can pass plain objects. */
export interface ShortcutEvent {
  key: string;
  shiftKey?: boolean;
  ctrlKey?: boolean;
  metaKey?: boolean;
  altKey?: boolean;
}

export function parseShortcut(event: ShortcutEvent): ShortcutKey | null {
  if (event.ctrlKey || event.metaKey || event.altKey) return null;
  if (event.key.length !== 1) return null;
  const key = event.key.toLowerCase();
  return SHORTCUT_KEYS.has(key) ? (key as ShortcutKey) : null;
}

/**
 * True when the event target is, or sits inside, an element the user types
 * into. Shortcut handling must leave those keystrokes alone so typing "?" or
 * "/n/" into a field never navigates or steals focus.
 */
export function isTextEntryTarget(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false;
  if (target.closest('input, textarea, select')) return true;
  return target.isContentEditable || target.closest('[contenteditable]:not([contenteditable="false"])') !== null;
}
