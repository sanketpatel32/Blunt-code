import { useEffect, useState } from 'react';
import { api, type FindingNote } from '../api';
import { useLoad } from '../hooks/useLoad';
import { message } from '../lib/notice';
import { relativeTime } from '../lib/format';
import { Card, CardHeader, CardTitle, CardContent } from './ui/card';
import { Button } from './ui/button';
import { ConfirmationDialog } from './dialogs';

export type FindingComment = FindingNote & { author?: string };
function legacyNotes(fingerprint: string): FindingComment[] {
  try {
    const value: unknown = JSON.parse(localStorage.getItem(`bluntcode.comments.${fingerprint}`) ?? '[]');
    return Array.isArray(value) ? value.filter((item) => item && typeof item.text === 'string' && typeof item.id === 'string') : [];
  } catch { return []; }
}

export function CommentsPanel({ workspaceId, fingerprint, title }: { workspaceId?: string; fingerprint: string; title?: string }) {
  const notes = useLoad(() => workspaceId ? api.findingNotes(workspaceId, fingerprint) : Promise.resolve([]), [workspaceId, fingerprint]);
  const [text, setText] = useState('');
  const [error, setError] = useState<string>();
  const [busy, setBusy] = useState(false);
  const [deleting, setDeleting] = useState<FindingNote>();
  const [legacy, setLegacy] = useState(() => legacyNotes(fingerprint));
  useEffect(() => { setText(''); setError(undefined); setLegacy(legacyNotes(fingerprint)); }, [workspaceId, fingerprint]);

  async function send() {
    if (!workspaceId || !text.trim() || busy) return;
    setBusy(true); setError(undefined);
    try { await api.addFindingNote(workspaceId, fingerprint, text.trim()); setText(''); await notes.reload(); }
    catch (e) { setError(`${message(e)} Your text has been kept.`); }
    finally { setBusy(false); }
  }
  async function remove() {
    if (!workspaceId || !deleting || busy) return;
    setBusy(true); setError(undefined);
    try { await api.deleteFindingNote(workspaceId, fingerprint, deleting.id); setDeleting(undefined); await notes.reload(); }
    catch (e) { setError(message(e)); }
    finally { setBusy(false); }
  }
  // Import one at a time. A successful item is removed from the browser queue,
  // so retrying a partially failed import does not duplicate saved notes.
  async function importNotes() {
    if (!workspaceId || busy) return;
    setBusy(true); setError(undefined);
    let remaining = [...legacy];
    try {
      while (remaining.length) {
        await api.addFindingNote(workspaceId, fingerprint, remaining[0].text, remaining[0].id);
        remaining = remaining.slice(1);
        setLegacy(remaining);
        localStorage.setItem(`bluntcode.comments.${fingerprint}`, JSON.stringify(remaining));
      }
    } catch (e) { setError(`${message(e)} Saved notes remain in the database; remaining browser notes are shown above.`); }
    finally { await notes.reload(); setBusy(false); }
  }
  return <Card>
    <CardHeader><CardTitle className="text-sm">Finding notes</CardTitle>{title && <p className="text-xs text-[var(--color-ink-soft)]">{title}</p>}
      <p className="text-xs text-[var(--color-ink-soft)]">Saved in the local app database for this workspace and finding. Notes persist across browser sessions and scans with the same fingerprint.</p>
    </CardHeader>
    <CardContent className="space-y-3">
      {!workspaceId && <p role="alert">Workspace context is unavailable. Notes cannot be saved yet.</p>}
      {legacy.length > 0 && <div className="availability-note">{legacy.length} notes remain in this browser from the earlier version. <Button size="sm" variant="outline" disabled={busy || !workspaceId} onClick={() => void importNotes()}>Import browser notes</Button></div>}
      {notes.loading ? <p role="status">Loading notes…</p> : notes.error ? <div role="alert"><p>{notes.error}</p><Button variant="outline" onClick={() => void notes.reload()}>Retry</Button></div> : <div role="log" aria-live="polite" aria-label="Finding notes" className="max-h-[42vh] overflow-y-auto">
        {notes.data?.length ? <ul className="space-y-2">{notes.data.map((note) => <li key={note.id} className="scan-review-row"><header><strong>You</strong><span className="text-xs">{relativeTime(note.createdAt)}</span></header><p className="whitespace-pre-wrap break-words">{note.text}</p><button className="text-button" onClick={() => setDeleting(note)} disabled={busy}>Delete note</button></li>)}</ul> : <p className="muted">No notes yet. Add context for your next review.</p>}
      </div>}
      <label className="block text-xs" htmlFor={`note-${fingerprint}`}>Add a note</label>
      <textarea id={`note-${fingerprint}`} value={text} onChange={(e) => setText(e.target.value)} maxLength={10000} rows={3} className="w-full border border-[var(--color-rule)] bg-[var(--color-surface)] p-3 text-sm" placeholder="Add review context…" onKeyDown={(e) => { if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) { e.preventDefault(); void send(); } }} />
      {error && <p role="alert" className="availability-note">{error}</p>}
      <div className="flex justify-end"><Button size="sm" disabled={busy || !workspaceId || !text.trim()} onClick={() => void send()}>{busy ? 'Saving…' : 'Save note'}</Button></div>
      {deleting && <ConfirmationDialog title="Delete this note?" description="This permanently removes the note from the local app database." confirmLabel="Delete note" busy={busy} onCancel={() => setDeleting(undefined)} onConfirm={() => void remove()} />}
    </CardContent>
  </Card>;
}
