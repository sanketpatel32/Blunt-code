import { useCallback, useEffect, useRef, useState, type CSSProperties, type ReactNode } from 'react';
import type { Notice } from '../lib/notice';
import { useReducedMotion } from '../hooks/useReducedMotion';

export type ToastKind = 'error' | 'info' | 'success';

export interface Toast {
  id: number;
  kind: ToastKind;
  text: string;
  action?: { label: string; onClick: () => void };
}

/** Auto-dismiss delay per kind; errors linger longest so the code can be read. */
const AUTO_DISMISS_MS: Record<ToastKind, number> = { info: 5000, success: 4000, error: 12000 };

/** Grace period for the exit transition before the toast leaves the DOM (matches --dur-slow). */
const EXIT_MS = 360;

/** Maximum simultaneously visible toasts; older toasts are dropped silently. */
const MAX_TOASTS = 4;

/** `message(error)` renders `CODE: Human text`; split ALL_CAPS codes for friendlier display. */
const CODED_TEXT = /^([A-Z][A-Z0-9_]{2,}): (.+)$/;

function splitCode(text: string): [code: string, message: string] | null {
  const match = CODED_TEXT.exec(text);
  return match ? [match[1], match[2]] : null;
}

export function useToasts() {
  const [toasts, setToasts] = useState<Toast[]>([]);
  const nextId = useRef(0);
  const notify = useCallback((notice: Notice) => {
    if (!notice) return;
    setToasts((current) => {
      const [newest, ...rest] = current;
      // A repeat of the visible toast (several loads failing at once) restarts the
      // existing toast instead of stacking identical copies.
      if (newest && newest.kind === notice.kind && newest.text === notice.text) {
        return [{ id: nextId.current++, kind: newest.kind, text: newest.text, action: notice.action ?? newest.action }, ...rest];
      }
      return [{ id: nextId.current++, kind: notice.kind, text: notice.text, action: notice.action }, ...current].slice(0, MAX_TOASTS);
    });
  }, []);
  const dismiss = useCallback((id: number) => setToasts((current) => current.filter((toast) => toast.id !== id)), []);
  return { toasts, notify, dismiss };
}

export function ToastStack({ toasts, onDismiss }: { toasts: Toast[]; onDismiss: (id: number) => void }) {
  const [closingIds, setClosingIds] = useState<number[]>([]);
  const exitTimers = useRef(new Set<number>());
  const closeToast = useCallback((id: number) => {
    setClosingIds((current) => (current.includes(id) ? current : [...current, id]));
    const handle = window.setTimeout(() => {
      exitTimers.current.delete(handle);
      onDismiss(id);
      setClosingIds((current) => current.filter((value) => value !== id));
    }, EXIT_MS);
    exitTimers.current.add(handle);
  }, [onDismiss]);
  useEffect(() => () => { for (const handle of exitTimers.current) window.clearTimeout(handle); }, []);

  useEffect(() => {
    if (!toasts.length) return;
    const onKey = (event: KeyboardEvent) => {
      if (event.key !== 'Escape' || event.defaultPrevented) return;
      const node = event.target as HTMLElement | null;
      if (node && typeof node.closest === 'function' && node.closest('dialog')) return; // let open dialogs own Escape
      closeToast(toasts[0].id);
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [toasts, closeToast]);

  const reduced = useReducedMotion();
  return <div className="toast-stack" role="status" aria-live="polite">
    {toasts.map((toast, idx) => <ToastItem key={toast.id} toast={toast} closing={closingIds.includes(toast.id)} onClose={closeToast} index={idx} reduced={reduced} />)}
  </div>;
}

function ToastItem({ toast, closing, onClose, index, reduced }: { toast: Toast; closing: boolean; onClose: (id: number) => void; index: number; reduced: boolean }) {
  const [entered, setEntered] = useState(false);
  const [paused, setPaused] = useState(false);
  const hovered = useRef(false);
  const focused = useRef(false);
  const sync = useRef<() => void>(() => {});
  const onCloseRef = useRef(onClose);

  useEffect(() => { onCloseRef.current = onClose; }, [onClose]);
  useEffect(() => { const frame = window.setTimeout(() => setEntered(true), 0); return () => window.clearTimeout(frame); }, []);

  useEffect(() => {
    let handle: number | undefined;
    let deadline = 0;
    let remaining = AUTO_DISMISS_MS[toast.kind];
    let finished = false;
    const fire = () => { handle = undefined; remaining = 0; finished = true; onCloseRef.current(toast.id); };
    const pause = () => { if (handle === undefined) return; window.clearTimeout(handle); handle = undefined; remaining = Math.max(0, deadline - Date.now()); };
    const resume = () => { if (finished || handle !== undefined || remaining <= 0) return; deadline = Date.now() + remaining; handle = window.setTimeout(fire, remaining); };
    const run = () => {
      if (hovered.current || focused.current) { pause(); setPaused(true); } else { resume(); setPaused(false); }
    };
    sync.current = run;
    run();
    return () => { sync.current = () => {}; if (handle !== undefined) window.clearTimeout(handle); };
  }, [toast.id, toast.kind]);

  const coded = splitCode(toast.text);
  const springStyle: CSSProperties | undefined = reduced ? undefined : { animationDelay: `${index * 40}ms`, willChange: 'transform, opacity' } as unknown as CSSProperties;
  return (
    // biome-ignore lint/a11y/noStaticElementInteractions: pointer/focus handlers only pause auto-dismiss; the toast itself is not operable (dismissal is a real button).
    <div
    className={`toast ${toast.kind} shadow-lg ${!reduced ? 'anim-slideIn' : ''}`}
    style={springStyle}
    data-state={entered && !closing ? 'open' : 'closed'}
    data-paused={paused || undefined}
    role={toast.kind === 'error' ? 'alert' : undefined}
    onMouseEnter={() => { hovered.current = true; sync.current(); }}
    onMouseLeave={() => { hovered.current = false; sync.current(); }}
    onFocus={() => { focused.current = true; sync.current(); }}
    onBlur={() => { focused.current = false; sync.current(); }}
  >
    <span className="toast-icon" aria-hidden="true">{ICONS[toast.kind]}</span>
    <div className="toast-body">
      {coded
        ? <><p className="toast-text">{coded[1]}</p><small className="toast-code">{coded[0]}</small></>
        : <p className="toast-text">{toast.text}</p>}
      {toast.action && <button type="button" className="text-button toast-action" onClick={() => { toast.action?.onClick(); onClose(toast.id); }}>{toast.action.label}</button>}
    </div>
    <button type="button" className="toast-dismiss" aria-label="Dismiss notification" onClick={() => onClose(toast.id)}>×</button>
    <i className="toast-lifetime" aria-hidden="true" style={{ animationDuration: `${AUTO_DISMISS_MS[toast.kind]}ms` }} />
  </div>);
}

const ICONS: Record<ToastKind, ReactNode> = {
  info: <svg role="img" aria-label="Information" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round"><circle cx="8" cy="8" r="6.5" /><path d="M8 7.4v3.6" /><path d="M8 4.6v.05" /></svg>,
  success: <svg role="img" aria-label="Success" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round"><circle cx="8" cy="8" r="6.5" /><path d="M5.1 8.4l2 2 3.8-4.2" /></svg>,
  error: <svg role="img" aria-label="Error" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round"><path d="M8 2.4 14.6 13.6H1.4Z" /><path d="M8 6.4v3.1" /><path d="M8 11.7v.05" /></svg>,
};
