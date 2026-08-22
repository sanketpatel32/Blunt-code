import { useEffect, useRef, type MouseEvent as ReactMouseEvent, type RefObject } from 'react';

/**
 * Shared accessibility behaviour for the app's modal dialogs: initial focus,
 * a Tab/Shift+Tab focus trap, Escape-to-close (ignored while busy), and
 * restoring focus to the element that had focus when the dialog opened.
 *
 * The focusable set is queried on every keypress rather than cached, so
 * controls that become disabled mid-flight (busy) drop out of the cycle
 * automatically. Backdrop closing uses mousedown on the backdrop itself only,
 * so dragging a selection that ends outside the dialog never closes it.
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

export interface UseDialogA11yOptions {
  /** Called on Escape and on backdrop mousedown. */
  onClose: () => void;
  /** While true, Escape and backdrop closing are ignored. */
  busy?: boolean;
  /** Element focused when the dialog opens; defaults to the first focusable child. */
  autoFocusRef?: RefObject<HTMLElement | null>;
}

export function useDialogA11y({ onClose, busy = false, autoFocusRef }: UseDialogA11yOptions) {
  const dialogRef = useRef<HTMLDialogElement>(null);
  const latest = useRef({ onClose, busy });
  useEffect(() => { latest.current = { onClose, busy }; });

  // Remember the trigger, move focus into the dialog, restore focus on unmount.
  useEffect(() => {
    const remembered = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    const initial = autoFocusRef?.current ?? (dialogRef.current ? focusableElements(dialogRef.current)[0] : undefined);
    initial?.focus();
    return () => { if (remembered && remembered.isConnected) remembered.focus(); };
    // Mount-only by design; current onClose/busy values are read through the ref above.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Escape closes (unless busy); Tab/Shift+Tab stay inside the dialog subtree.
  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      const dialog = dialogRef.current;
      if (!dialog || event.defaultPrevented) return;
      if (event.key === 'Escape') {
        if (!latest.current.busy) latest.current.onClose();
        return;
      }
      if (event.key !== 'Tab') return;
      const focusables = focusableElements(dialog);
      if (focusables.length === 0) { event.preventDefault(); return; }
      const active = document.activeElement;
      const current = active instanceof HTMLElement ? focusables.indexOf(active) : -1;
      if (current === -1) {
        const owner = active instanceof HTMLElement ? active.closest('dialog') : null;
        if (owner && owner !== dialog) return; // a stacked dialog owns focus; its trap handles this key
        event.preventDefault();
        focusables[0].focus();
        return;
      }
      event.preventDefault();
      const first = 0;
      const last = focusables.length - 1;
      const next = event.shiftKey ? (current === first ? last : current - 1) : current === last ? first : current + 1;
      focusables[next].focus();
    };
    window.addEventListener('keydown', onKeyDown);
    return () => window.removeEventListener('keydown', onKeyDown);
  }, []);

  /** Spread onto the `.dialog-backdrop` div: closes on mousedown that starts on the backdrop itself, never on its children. */
  const onBackdropMouseDown = (event: ReactMouseEvent<HTMLDivElement>) => {
    if (event.target !== event.currentTarget) return;
    if (!latest.current.busy) latest.current.onClose();
  };

  return { dialogRef, onBackdropMouseDown };
}
