import { useEffect, useLayoutEffect, useRef, type RefObject } from 'react';

/**
 * Shared behaviour for the app's modal dialogs, built on the native modal
 * `<dialog>`: `showModal()` puts the dialog in the top layer and makes the
 * rest of the page inert (no pointer, focus or screen-reader access behind
 * it). On top of that the hook adds initial focus, a Tab/Shift+Tab wrap,
 * Escape and backdrop closing (both ignored while busy), and focus restore.
 *
 * Render the `<dialog>` without the `open` attribute and attach `dialogRef`;
 * the hook opens it modally on mount (or when `open` turns true) and closes
 * it on unmount.
 *
 * Escape is handled on the dialog's own keydown so it is scoped to the
 * topmost dialog (the only one that can hold focus) and preventDefault stops
 * the browser's own close. The native `cancel` (a close request that did not
 * come through our keydown) is routed through the same busy guard, and a
 * dialog the browser force-closes anyway is reopened while busy.
 *
 * The focusable set is queried on every Tab rather than cached, so controls
 * that become disabled mid-flight drop out of the cycle. The backdrop closes
 * on a mousedown that lands outside the dialog box, so dragging a selection
 * out of the dialog or grabbing its scrollbar never closes it.
 */

const FOCUSABLE_SELECTOR = 'a[href], button:not([disabled]), textarea:not([disabled]), input:not([disabled]), select:not([disabled]), [tabindex]:not([tabindex="-1"])';

/** jsdom has no layout engine, so client rects are empty there even on <html>. */
function layoutAvailable(): boolean {
  return document.documentElement.getClientRects().length > 0;
}

function visible(element: HTMLElement): boolean {
  if (element.hidden || element.style.display === 'none' || element.style.visibility === 'hidden') return false;
  // In a real browser an element without client rects is not rendered.
  return !layoutAvailable() || element.getClientRects().length > 0;
}

function focusableElements(container: HTMLElement): HTMLElement[] {
  return [...container.querySelectorAll<HTMLElement>(FOCUSABLE_SELECTOR)].filter((element) => !(element instanceof HTMLInputElement && element.type === 'hidden') && visible(element));
}

function outsideBox(dialog: HTMLElement, event: MouseEvent): boolean {
  const box = dialog.getBoundingClientRect();
  return event.clientX < box.left || event.clientX > box.right || event.clientY < box.top || event.clientY > box.bottom;
}

export interface UseDialogA11yOptions {
  /** Called on Escape and on backdrop mousedown. */
  onClose: () => void;
  /** While true, Escape and backdrop closing are ignored. */
  busy?: boolean;
  /** Element focused when the dialog opens; defaults to the first focusable child. */
  autoFocusRef?: RefObject<HTMLElement | null>;
  /** For dialogs that stay mounted while hidden; defaults to true (open while mounted). */
  open?: boolean;
}

export function useDialogA11y({ onClose, busy = false, autoFocusRef, open = true }: UseDialogA11yOptions) {
  const dialogRef = useRef<HTMLDialogElement>(null);
  const latest = useRef({ onClose, busy });
  useEffect(() => { latest.current = { onClose, busy }; });

  // Layout effect: the dialog must be shown before anything inside it can take focus.
  useLayoutEffect(() => {
    const dialog = dialogRef.current;
    if (!open || !dialog) return;
    const remembered = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    if (!dialog.open) dialog.showModal();
    const initial = autoFocusRef?.current ?? focusableElements(dialog)[0];
    initial?.focus();

    const requestClose = () => { if (!latest.current.busy) latest.current.onClose(); };
    const onKeyDown = (event: KeyboardEvent) => {
      // A dialog nested inside this one's subtree already handled the key.
      if (event.defaultPrevented) return;
      if (event.key === 'Escape') {
        event.preventDefault();
        requestClose();
        return;
      }
      if (event.key !== 'Tab') return;
      const focusables = focusableElements(dialog);
      event.preventDefault();
      if (focusables.length === 0) return;
      const active = document.activeElement;
      const current = active instanceof HTMLElement ? focusables.indexOf(active) : -1;
      const last = focusables.length - 1;
      const next = current === -1 ? 0 : event.shiftKey ? (current === 0 ? last : current - 1) : current === last ? 0 : current + 1;
      focusables[next].focus();
    };
    const onCancel = (event: Event) => { event.preventDefault(); requestClose(); };
    // `close` fires asynchronously, so one queued by an earlier cleanup (StrictMode's
    // remount, an effect re-run) can land after the dialog was reopened: ignore it.
    const onNativeClose = () => {
      if (!dialog.isConnected || dialog.open) return;
      if (latest.current.busy) dialog.showModal();
      else latest.current.onClose();
    };
    const onMouseDown = (event: MouseEvent) => {
      if (event.target !== dialog || !outsideBox(dialog, event)) return;
      // Keep the press from moving focus to <body>, so restoring it to the trigger sticks.
      event.preventDefault();
      requestClose();
    };
    dialog.addEventListener('keydown', onKeyDown);
    dialog.addEventListener('cancel', onCancel);
    dialog.addEventListener('close', onNativeClose);
    dialog.addEventListener('mousedown', onMouseDown);
    return () => {
      dialog.removeEventListener('keydown', onKeyDown);
      dialog.removeEventListener('cancel', onCancel);
      dialog.removeEventListener('close', onNativeClose);
      dialog.removeEventListener('mousedown', onMouseDown);
      if (dialog.open) dialog.close();
      if (remembered?.isConnected) remembered.focus();
    };
    // autoFocusRef is a stable useRef from every caller; onClose/busy values
    // are read through the `latest` ref so this only reruns when `open` flips.
  }, [open, autoFocusRef]);

  return { dialogRef };
}
