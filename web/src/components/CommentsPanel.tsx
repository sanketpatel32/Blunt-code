import { useEffect, useMemo, useState } from 'react';
import { Card, CardHeader, CardTitle, CardContent } from './ui/card';
import { Button } from './ui/button';
import { relativeTime } from '../lib/format';

export type FindingComment = {
  id: string;
  author: string;
  text: string;
  createdAt: string;
};

function storageKey(fingerprint: string) {
  return `bluntcode.comments.${fingerprint}`;
}

function loadComments(fingerprint: string): FindingComment[] {
  try {
    const raw = window.localStorage.getItem(storageKey(fingerprint));
    if (!raw) return [];
    const parsed = JSON.parse(raw) as FindingComment[];
    return Array.isArray(parsed) ? parsed : [];
  } catch {
    return [];
  }
}

function saveComments(fingerprint: string, items: FindingComment[]) {
  try {
    window.localStorage.setItem(storageKey(fingerprint), JSON.stringify(items));
  } catch { /* quota */ }
}

function avatarInitial(author: string) {
  const t = author.trim();
  return t ? t[0]!.toUpperCase() : '?';
}

export function CommentsPanel({ fingerprint, title }: { fingerprint: string; title?: string }) {
  const [comments, setComments] = useState<FindingComment[]>(() => loadComments(fingerprint));
  const [text, setText] = useState('');
  const [tick, setTick] = useState(0);

  // refresh relative time every 30s
  useEffect(() => {
    const id = window.setInterval(() => setTick((n) => n + 1), 30_000);
    return () => window.clearInterval(id);
  }, []);

  // reload when fingerprint changes
  useEffect(() => {
    setComments(loadComments(fingerprint));
  }, [fingerprint]);

  // keep tick used
  void tick;

  const canSend = useMemo(() => text.trim().length > 0, [text]);

  const handleSend = () => {
    const trimmed = text.trim();
    if (!trimmed) return;
    const next: FindingComment = {
      id: `${Date.now()}-${Math.random().toString(36).slice(2, 6)}`,
      author: 'You',
      text: trimmed,
      createdAt: new Date().toISOString(),
    };
    const updated = [...comments, next];
    setComments(updated);
    saveComments(fingerprint, updated);
    setText('');
  };

  const onKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) {
      e.preventDefault();
      if (canSend) handleSend();
    }
  };

  return (
    <Card className="border-[var(--color-rule)] shadow-none">
      <CardHeader className="pb-3">
        <CardTitle className="text-sm font-semibold">Comments</CardTitle>
        {title && <p className="text-xs text-[var(--color-ink-soft)] truncate">{title}</p>}
      </CardHeader>
      <CardContent className="flex flex-col gap-3 pt-0">
        <div
          role="log"
          aria-live="polite"
          aria-label="Comments"
          className="max-h-[42vh] overflow-y-auto rounded-[var(--radius-md)] border border-[var(--color-rule-faint)] bg-[var(--color-surface-muted)]/40 p-2"
          style={{ scrollbarWidth: 'thin' }}
        >
          {comments.length === 0 ? (
            <p className="py-6 text-center text-sm text-[var(--color-ink-faint)]">No comments yet. Start the discussion.</p>
          ) : (
            <ul className="flex flex-col gap-2">
              {comments.map((c) => (
                <li key={c.id} className="flex gap-2 rounded-[var(--radius-md)] bg-[var(--color-surface)] border border-[var(--color-rule-faint)] px-3 py-2 shadow-[var(--shadow-xs)]">
                  <span
                    aria-hidden="true"
                    className="mt-0.5 flex h-7 w-7 shrink-0 items-center justify-center rounded-full bg-[var(--color-accent-soft)] text-xs font-bold text-[var(--color-accent-strong)]"
                  >
                    {avatarInitial(c.author)}
                  </span>
                  <span className="min-w-0 flex-1">
                    <span className="flex flex-wrap items-baseline gap-1.5">
                      <strong className="text-xs font-semibold text-[var(--color-ink)]">{c.author}</strong>
                      <span className="text-[11px] text-[var(--color-ink-faint)]">{relativeTime(c.createdAt)}</span>
                    </span>
                    <span className="mt-0.5 block whitespace-pre-wrap break-words text-sm leading-5 text-[var(--color-ink)]">{c.text}</span>
                  </span>
                </li>
              ))}
            </ul>
          )}
        </div>

        <label className="flex flex-col gap-1.5" htmlFor={`comment-${fingerprint}`}>
          <span className="sr-only">Add a comment</span>
          <textarea
            id={`comment-${fingerprint}`}
            value={text}
            onChange={(e) => setText(e.target.value)}
            onKeyDown={onKeyDown}
            placeholder="Add a comment… (⌘+Enter to send)"
            rows={3}
            className="min-h-[72px] w-full resize-y rounded-[var(--radius-md)] border border-[var(--color-rule-strong)] bg-[var(--color-surface)] px-3 py-2 text-sm placeholder:text-[var(--color-ink-faint)] focus-visible:outline-none focus-visible:border-[var(--color-accent)] focus-visible:ring-2 focus-visible:ring-[var(--color-accent-glow)]"
          />
        </label>
        <div className="flex justify-end">
          <Button size="sm" disabled={!canSend} onClick={handleSend} aria-label="Send comment">
            Send
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}
