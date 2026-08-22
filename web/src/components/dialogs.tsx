import { useEffect, useRef, useState, type FormEvent } from 'react';
import { api } from '../api';
import type { Finding, SourcePreview, Workspace } from '../types';
import type { Notice } from '../lib/notice';
import { message } from '../lib/notice';
import { findingLocation } from '../lib/format';
import { copyToClipboard } from '../lib/clipboard';
import { useLoad } from '../hooks/useLoad';
import { useDialogA11y } from '../hooks/useDialogA11y';
import { ErrorPanel, Loading } from './ui';

export function AddWorkspaceDialog({ onClose, onCreated, notify }: { onClose: () => void; onCreated: (workspace: Workspace) => void; notify: (n: Notice) => void }) {
  const [path, setPath] = useState('');
  const [name, setName] = useState('');
  const [busy, setBusy] = useState(false);
  const pathInputRef = useRef<HTMLInputElement>(null);
  const { dialogRef, onBackdropMouseDown } = useDialogA11y({ onClose, busy, autoFocusRef: pathInputRef });
  const pick = async () => { setBusy(true); try { const result = await api.selectFolder(); if (!result.cancelled && result.path) { const folder = result.path; setPath(folder); setName((current) => current || folder.split(/[\\/]/).filter(Boolean).pop() || 'Workspace'); } } catch (e) { notify({ kind: 'error', text: message(e) }); } finally { setBusy(false); } };
  const submit = async (event: FormEvent) => { event.preventDefault(); if (!path.trim()) return; setBusy(true); try { onCreated(await api.createWorkspace({ root_path: path.trim(), name: name.trim() || undefined })); } catch (e) { notify({ kind: 'error', text: message(e) }); } finally { setBusy(false); } };
  return (
  // biome-ignore lint/a11y/noStaticElementInteractions: backdrop-only dismissal is pointer convenience; keyboard users close via Escape and the dialog's own Cancel/close button.
    <div className="dialog-backdrop" role="presentation" onMouseDown={onBackdropMouseDown}><dialog ref={dialogRef} open aria-modal="true" aria-labelledby="add-workspace-title"><form onSubmit={(event) => void submit(event)}><header><h2 id="add-workspace-title">Add workspace</h2><button type="button" className="icon-button" onClick={onClose} aria-label="Close">×</button></header><p>Choose a project folder. Blunt Code keeps a reference and never changes your source files.</p><label>Folder path<div className="picker-row"><input ref={pathInputRef} value={path} onChange={(event) => setPath(event.target.value)} placeholder="C:\\Projects\\my-app" required /><button type="button" className="button secondary" onClick={() => void pick()} disabled={busy}>Browse…</button></div></label><label>Workspace name <input value={name} onChange={(event) => setName(event.target.value)} placeholder="Optional" /></label><footer><button type="button" className="button secondary" onClick={onClose}>Cancel</button><button type="submit" className="button primary" disabled={busy || !path.trim()}>{busy ? 'Adding…' : 'Add workspace'}</button></footer></form></dialog></div>);
}

export function ConfirmationDialog({ title, description, confirmLabel, busy, onCancel, onConfirm }: { title: string; description: string; confirmLabel: string; busy: boolean; onCancel: () => void; onConfirm: () => void }) {
  const cancelRef = useRef<HTMLButtonElement>(null);
  const { dialogRef, onBackdropMouseDown } = useDialogA11y({ onClose: onCancel, busy, autoFocusRef: cancelRef });
  return (
  // biome-ignore lint/a11y/noStaticElementInteractions: backdrop-only dismissal is pointer convenience; keyboard users close via Escape and the dialog's own Cancel/close button.
    <div className="dialog-backdrop" role="presentation" onMouseDown={onBackdropMouseDown}><dialog ref={dialogRef} open aria-modal="true" aria-labelledby="confirmation-title"><div className="confirmation-dialog"><header><h2 id="confirmation-title">{title}</h2><button type="button" className="icon-button" onClick={onCancel} disabled={busy} aria-label="Close">×</button></header><p>{description}</p><footer><button ref={cancelRef} type="button" className="button secondary" onClick={onCancel} disabled={busy}>Cancel</button><button type="button" className="button danger" onClick={onConfirm} disabled={busy}>{busy ? 'Working…' : confirmLabel}</button></footer></div></dialog></div>);
}

export function FindingPreviewDialog({ scanId, finding, onClose }: { scanId: string; finding: Finding; onClose: () => void }) {
  const preview = useLoad(() => api.findingPreview(scanId, finding.id), [scanId, finding.id]);
  const closeRef = useRef<HTMLButtonElement>(null);
  const { dialogRef, onBackdropMouseDown } = useDialogA11y({ onClose, autoFocusRef: closeRef });
  const [copied, setCopied] = useState(false);
  const copyTimer = useRef<number | undefined>(undefined);
  useEffect(() => () => window.clearTimeout(copyTimer.current), []);
  /** Copies the file:line:col location using the shared clipboard helper; the label confirms briefly instead of raising a toast. */
  const copyLocation = async () => {
    if (!(await copyToClipboard(findingLocation(finding)))) return;
    setCopied(true);
    window.clearTimeout(copyTimer.current);
    copyTimer.current = window.setTimeout(() => setCopied(false), 2000);
  };
  const data: SourcePreview | undefined = preview.data;
  return (
  // biome-ignore lint/a11y/noStaticElementInteractions: backdrop-only dismissal is pointer convenience; keyboard users close via Escape and the dialog's own Cancel/close button.
    <div className="dialog-backdrop" role="presentation" onMouseDown={onBackdropMouseDown}><dialog ref={dialogRef} open aria-modal="true" aria-labelledby="source-preview-title" className="code-preview-dialog"><div className="confirmation-dialog"><header><div><h2 id="source-preview-title">Source preview</h2><p className="code-preview-location"><code>{findingLocation(finding)}</code></p></div><button ref={closeRef} type="button" className="icon-button" onClick={onClose} aria-label="Close preview">×</button></header>{preview.loading ? <Loading /> : preview.error ? <ErrorPanel error={preview.error} retry={preview.reload} /> : data ? <><p>{data.note ?? 'Current source near this finding.'}</p><pre className="code-preview">{data.lines.map((line) => <code key={line.number} className={line.number >= (data.highlight_start_line ?? 0) && line.number <= (data.highlight_end_line ?? 0) ? 'highlight' : ''}><span aria-hidden="true">{line.number}</span>{line.text || ' '}</code>)}</pre></> : null}<footer><button type="button" className={`button secondary copy-location${copied ? ' copied' : ''}`} onClick={() => void copyLocation()}>{copied ? 'Copied' : 'Copy location'}</button><button type="button" className="button secondary" onClick={onClose}>Close</button></footer></div></dialog></div>);
}
