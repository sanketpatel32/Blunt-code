/**
 * jsdom has no HTMLDialogElement.showModal()/close(). This minimal stand-in
 * mirrors the observable parts the app relies on: the `open` attribute, the
 * InvalidStateError on re-opening, and the `close` event. It also records
 * which dialogs were opened modally so tests can assert real modal semantics
 * (jsdom cannot evaluate `:modal` or top-layer inertness).
 */
const modalDialogs = new WeakSet<HTMLDialogElement>();

/** True only for a dialog currently open via showModal(), never one rendered with `open`. */
export function isShownModal(dialog: HTMLDialogElement): boolean {
  return dialog.open && modalDialogs.has(dialog);
}

export function installDialogPolyfill() {
  if (typeof HTMLDialogElement === 'undefined') return;
  const proto = HTMLDialogElement.prototype;
  if (typeof proto.showModal === 'function') return;
  proto.showModal = function showModal(this: HTMLDialogElement) {
    if (this.open && !modalDialogs.has(this)) throw new DOMException('The dialog is already open as a non-modal dialog.', 'InvalidStateError');
    if (!this.isConnected) throw new DOMException('The dialog is not connected.', 'InvalidStateError');
    modalDialogs.add(this);
    this.setAttribute('open', '');
  };
  proto.show = function show(this: HTMLDialogElement) { this.setAttribute('open', ''); };
  proto.close = function close(this: HTMLDialogElement, returnValue?: string) {
    if (!this.open) return;
    modalDialogs.delete(this);
    this.removeAttribute('open');
    if (returnValue !== undefined) this.returnValue = returnValue;
    // Browsers queue the close event rather than dispatching it synchronously.
    queueMicrotask(() => this.dispatchEvent(new Event('close')));
  };
}
