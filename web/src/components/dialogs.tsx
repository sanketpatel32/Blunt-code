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
import { Button } from './ui/button';
import { Input } from './ui/input';

export function AddWorkspaceDialog({ onClose, onCreated, notify }: { onClose: () => void; onCreated: (workspace: Workspace) => void; notify: (n: Notice) => void }) {
  const [path, setPath] = useState('');
  const [name, setName] = useState(() => {
    try { const raw = localStorage.getItem('bluntcode.templatePrefill'); if (raw) { const p = JSON.parse(raw) as { name?: string }; if (p.name) return p.name; } } catch { /* ignore */ }
    return '';
  });
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    const onTemplate = (e: Event) => {
      const ce = e as CustomEvent<{ name?: string }>;
      if (ce.detail?.name) setName((cur) => cur || ce.detail.name!);
    };
    window.addEventListener('bluntcode:use-template', onTemplate as EventListener);
    return () => window.removeEventListener('bluntcode:use-template', onTemplate as EventListener);
  }, []);
  const pathInputRef = useRef<HTMLInputElement>(null);
  const { dialogRef, onBackdropMouseDown } = useDialogA11y({ onClose, busy, autoFocusRef: pathInputRef });
  const pick = async () => { setBusy(true); try { const result = await api.selectFolder(); if (!result.cancelled && result.path) { const folder = result.path; setPath(folder); setName((current) => current || folder.split(/[\\/]/).filter(Boolean).pop() || 'Workspace'); } } catch (e) { notify({ kind: 'error', text: message(e) }); } finally { setBusy(false); } };
  const submit = async (event: FormEvent) => { event.preventDefault(); if (!path.trim()) return; setBusy(true); try { onCreated(await api.createWorkspace({ root_path: path.trim(), name: name.trim() || undefined })); } catch (e) { notify({ kind: 'error', text: message(e) }); } finally { setBusy(false); } };
  return (
  // biome-ignore lint/a11y/noStaticElementInteractions: backdrop-only dismissal is pointer convenience; keyboard users close via Escape and the dialog's own Cancel/close button.
    <div className="dialog-backdrop" role="presentation" onMouseDown={onBackdropMouseDown}><dialog ref={dialogRef} open aria-modal="true" aria-labelledby="add-workspace-title"><form onSubmit={(event) => void submit(event)}><header><h2 id="add-workspace-title">Add workspace</h2><Button variant="ghost" size="icon" type="button" onClick={onClose} aria-label="Close">×</Button></header><p>Choose a project folder. Blunt Code keeps a reference and never changes your source files.</p><label>Folder path<div className="picker-row flex gap-2"><Input ref={pathInputRef} value={path} onChange={(event) => setPath(event.target.value)} placeholder="C:\Projects\my-app" required className="flex-1" /><Button type="button" variant="outline" onClick={() => void pick()} disabled={busy}>Browse…</Button></div></label><label>Workspace name <Input value={name} onChange={(event) => setName(event.target.value)} placeholder="Optional" /></label><footer className="flex justify-end gap-2"><Button type="button" variant="outline" onClick={onClose}>Cancel</Button><Button type="submit" disabled={busy || !path.trim()}>{busy ? 'Adding…' : 'Add workspace'}</Button></footer></form></dialog></div>);
}

export function ConfirmationDialog({ title, description, confirmLabel, busy, onCancel, onConfirm }: { title: string; description: string; confirmLabel: string; busy: boolean; onCancel: () => void; onConfirm: () => void }) {
  const cancelRef = useRef<HTMLButtonElement>(null);
  const { dialogRef, onBackdropMouseDown } = useDialogA11y({ onClose: onCancel, busy, autoFocusRef: cancelRef });
  return (
  // biome-ignore lint/a11y/noStaticElementInteractions: backdrop-only dismissal is pointer convenience; keyboard users close via Escape and the dialog's own Cancel/close button.
    <div className="dialog-backdrop" role="presentation" onMouseDown={onBackdropMouseDown}><dialog ref={dialogRef} open aria-modal="true" aria-labelledby="confirmation-title"><div className="confirmation-dialog"><header><h2 id="confirmation-title">{title}</h2><Button variant="ghost" size="icon" type="button" onClick={onCancel} disabled={busy} aria-label="Close">×</Button></header><p>{description}</p><footer className="flex justify-end gap-2"><Button ref={cancelRef} type="button" variant="outline" onClick={onCancel} disabled={busy}>Cancel</Button><Button type="button" variant="destructive" onClick={onConfirm} disabled={busy}>{busy ? 'Working…' : confirmLabel}</Button></footer></div></dialog></div>);
}

/** Upper bound on the optional suppression note; mirrors the backend's maxSuppressionReasonLength. */
export const SUPPRESSION_REASON_MAX = 500;

/**
 * "Suppress this finding" dialog: records a fingerprint dismissal for the workspace.
 * The reason is optional and capped at 500 characters; confirming posts the
 * suppression and hands control back to the caller (which closes the dialog and
 * refreshes the findings list). Errors keep the dialog open so nothing is lost.
 */
export function SuppressFindingDialog({ workspaceId, finding, onClose, onSuppressed, notify }: { workspaceId: string; finding: Finding; onClose: () => void; onSuppressed: (reason: string) => void; notify: (n: Notice) => void }) {
  const [reason, setReason] = useState('');
  const [busy, setBusy] = useState(false);
  const reasonRef = useRef<HTMLTextAreaElement>(null);
  const { dialogRef, onBackdropMouseDown } = useDialogA11y({ onClose, busy, autoFocusRef: reasonRef });
  const name = finding.title ?? finding.rule_id ?? 'finding';
  const submit = async (event: FormEvent) => {
    event.preventDefault();
    if (!finding.fingerprint) return;
    setBusy(true);
    try {
      await api.addSuppression(workspaceId, finding.fingerprint, reason.trim() || undefined);
      onSuppressed(reason.trim());
    } catch (e) {
      notify({ kind: 'error', text: message(e) });
      setBusy(false);
    }
  };
  return (
  // biome-ignore lint/a11y/noStaticElementInteractions: backdrop-only dismissal is pointer convenience; keyboard users close via Escape and the dialog's own Cancel/close button.
    <div className="dialog-backdrop" role="presentation" onMouseDown={onBackdropMouseDown}><dialog ref={dialogRef} open aria-modal="true" aria-labelledby="suppress-finding-title"><form onSubmit={(event) => void submit(event)}><header><h2 id="suppress-finding-title">Suppress this finding?</h2><button type="button" className="icon-button" onClick={onClose} aria-label="Close">×</button></header><p className="suppress-finding-context"><strong>{name}</strong> <code>{findingLocation(finding)}</code></p><p id="suppress-finding-hint">Suppressing hides this finding from future scans, reports, and the CI gate.</p><label htmlFor="suppress-finding-reason">Reason (optional)<textarea id="suppress-finding-reason" ref={reasonRef} rows={3} maxLength={SUPPRESSION_REASON_MAX} value={reason} onChange={(event) => setReason(event.target.value)} placeholder="Why this finding is being hidden" aria-describedby="suppress-finding-hint suppress-finding-count" /></label><div className="reason-suggestions" role="group" aria-label="Common reasons">{['False positive', 'Safe by design', 'Documented in runbook', 'Third-party code', 'Tracked externally'].map((suggestion) => <button key={suggestion} type="button" className="text-button reason-suggestion" onClick={() => { setReason(suggestion); reasonRef.current?.focus(); }}>{suggestion}</button>)}</div><small id="suppress-finding-count" className="suppress-finding-count">{reason.length}/{SUPPRESSION_REASON_MAX} characters</small><footer><button type="button" className="button secondary" onClick={onClose} disabled={busy}>Cancel</button><button type="submit" className="button danger" disabled={busy || !finding.fingerprint}>{busy ? 'Suppressing…' : 'Suppress finding'}</button></footer></form></dialog></div>);
}

export function FindingPreviewDialog({ scanId, finding, onClose }: { scanId: string; finding: Finding; onClose: () => void }) {
  const preview = useLoad(() => api.findingPreview(scanId, finding.id), [scanId, finding.id]);
  const closeRef = useRef<HTMLButtonElement>(null);
  const { dialogRef, onBackdropMouseDown } = useDialogA11y({ onClose, autoFocusRef: closeRef });
  const [copied, setCopied] = useState(false);
  const [fingerprintCopied, setFingerprintCopied] = useState(false);
  const copyTimer = useRef<number | undefined>(undefined);
  const fingerprintTimer = useRef<number | undefined>(undefined);
  useEffect(() => () => { window.clearTimeout(copyTimer.current); window.clearTimeout(fingerprintTimer.current); }, []);
  /** Copies the file:line:col location using the shared clipboard helper; the label confirms briefly instead of raising a toast. */
  const copyLocation = async () => {
    if (!(await copyToClipboard(findingLocation(finding)))) return;
    setCopied(true);
    window.clearTimeout(copyTimer.current);
    copyTimer.current = window.setTimeout(() => setCopied(false), 2000);
  };
  /** Copies the stable fingerprint so it can be pasted into suppression tooling or bug reports. */
  const copyFingerprint = async () => {
    if (!finding.fingerprint) return;
    if (!(await copyToClipboard(finding.fingerprint))) return;
    setFingerprintCopied(true);
    window.clearTimeout(fingerprintTimer.current);
    fingerprintTimer.current = window.setTimeout(() => setFingerprintCopied(false), 2000);
  };
  const data: SourcePreview | undefined = preview.data;
  return (
  // biome-ignore lint/a11y/noStaticElementInteractions: backdrop-only dismissal is pointer convenience; keyboard users close via Escape and the dialog's own Cancel/close button.
    <div className="dialog-backdrop" role="presentation" onMouseDown={onBackdropMouseDown}><dialog ref={dialogRef} open aria-modal="true" aria-labelledby="source-preview-title" className="code-preview-dialog"><div className="confirmation-dialog"><header><div><h2 id="source-preview-title">Source preview</h2><p className="code-preview-location"><code>{findingLocation(finding)}</code></p></div><button ref={closeRef} type="button" className="icon-button" onClick={onClose} aria-label="Close preview">×</button></header>{preview.loading ? <Loading /> : preview.error ? <ErrorPanel error={preview.error} retry={preview.reload} /> : data ? <><p>{data.note ?? 'Current source near this finding.'}</p><pre className="code-preview">{data.lines.map((line) => <code key={line.number} className={line.number >= (data.highlight_start_line ?? 0) && line.number <= (data.highlight_end_line ?? 0) ? 'highlight' : ''}><span aria-hidden="true">{line.number}</span>{line.text || ' '}</code>)}</pre></> : null}<footer>{finding.fingerprint && <button type="button" className={`button secondary copy-fingerprint${fingerprintCopied ? ' copied' : ''}`} onClick={() => void copyFingerprint()} title="The stable hash used for suppressions and baselines">{fingerprintCopied ? 'Copied' : 'Copy fingerprint'}</button>}<button type="button" className={`button secondary copy-location${copied ? ' copied' : ''}`} onClick={() => void copyLocation()}>{copied ? 'Copied' : 'Copy location'}</button><button type="button" className="button secondary" onClick={onClose}>Close</button></footer></div></dialog></div>);
}
