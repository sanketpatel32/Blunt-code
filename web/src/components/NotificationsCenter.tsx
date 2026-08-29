import { useEffect, useMemo, useState } from 'react';
import { Bell, Check, Trash2, Volume2, VolumeX } from 'lucide-react';
import { Button } from './ui/button';
import { Badge } from './ui/badge';
import { DropdownMenu, DropdownMenuContent, DropdownMenuTrigger, DropdownMenuLabel, DropdownMenuSeparator } from './ui/dropdown-menu';
import { getNotifications, markAllRead, markRead, clearNotifications, type AppNotification } from '../lib/notifications';
import { relativeTime } from '../lib/format';

function kindDot(kind?: string) {
  if (kind === 'critical') return 'bg-[var(--color-danger)]';
  if (kind === 'success') return 'bg-[var(--color-success)]';
  if (kind === 'warning') return 'bg-[var(--color-warning)]';
  return 'bg-[var(--color-accent)]';
}

function dayLabel(iso: string): string {
  const d = new Date(iso);
  const now = new Date();
  const diffMs = now.getTime() - d.getTime();
  const days = Math.floor(diffMs / 86_400_000);
  if (days === 0) return 'Today';
  if (days === 1) return 'Yesterday';
  // group older by date string
  return d.toLocaleDateString(undefined, { month: 'short', day: 'numeric', year: d.getFullYear() !== now.getFullYear() ? 'numeric' : undefined });
}

function groupByDay(items: AppNotification[]): Map<string, AppNotification[]> {
  const m = new Map<string, AppNotification[]>();
  for (const n of items) {
    const k = dayLabel(n.createdAt);
    const arr = m.get(k);
    if (arr) arr.push(n);
    else m.set(k, [n]);
  }
  return m;
}

type Tab = 'all' | 'unread' | 'scans';

export function NotificationsCenter() {
  const [items, setItems] = useState<AppNotification[]>(() => getNotifications());
  const [tab, setTab] = useState<Tab>('all');
  const [soundOn, setSoundOn] = useState(false);

  useEffect(() => {
    const refresh = () => setItems(getNotifications());
    const onCustom = (e: Event) => {
      const detail = (e as CustomEvent<AppNotification[]>).detail;
      if (Array.isArray(detail)) setItems(detail);
      else refresh();
    };
    window.addEventListener('storage', refresh);
    window.addEventListener('bluntcode:notifications' as never, onCustom as never);
    return () => {
      window.removeEventListener('storage', refresh);
      window.removeEventListener('bluntcode:notifications' as never, onCustom as never);
    };
  }, []);

  const unread = useMemo(() => items.filter((n) => !n.read).length, [items]);

  const filtered = useMemo(() => {
    if (tab === 'unread') return items.filter((n) => !n.read);
    if (tab === 'scans') return items.filter((n) => n.title.toLowerCase().includes('scan') || n.kind === 'success' || n.message?.toLowerCase().includes('scan'));
    return items;
  }, [items, tab]);

  const grouped = useMemo(() => groupByDay(filtered), [filtered]);

  const handleMarkAll = () => {
    const next = markAllRead();
    setItems(next);
  };
  const handleMarkOne = (id: string) => {
    const next = markRead(id);
    setItems(next);
  };
  const handleClearAll = () => {
    clearNotifications();
    setItems([]);
  };
  const toggleSound = () => setSoundOn((v) => !v);

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          variant="outline"
          size="icon"
          className="relative rounded-[var(--radius-button)] h-[2.15rem] w-[2.15rem] border-[var(--color-rule)] hover:border-[var(--color-rule-strong)]"
          aria-label={unread ? `Notifications, ${unread} unread` : 'Notifications'}
        >
          <Bell className="h-4 w-4" aria-hidden="true" />
          {unread > 0 && (
            <>
              <Badge className="absolute -right-1 -top-1 h-4 min-w-4 justify-center px-1 py-0 text-[10px] leading-none bg-[var(--color-danger-strong)] text-[var(--color-on-accent)] border-transparent">
                {unread > 99 ? '99+' : String(unread)}
              </Badge>
              <span className="absolute right-1 top-1 h-2 w-2 rounded-full bg-[var(--color-danger)] ring-2 ring-[var(--color-surface)] motion-reduce:hidden" aria-hidden="true" />
            </>
          )}
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-80 p-0 shadow-xl rounded-[var(--radius-lg)] overflow-hidden">
        <div className="flex items-center justify-between px-3 py-2">
          <DropdownMenuLabel className="p-0 text-sm font-semibold">Notifications</DropdownMenuLabel>
          <div className="flex items-center gap-1.5">
            <button type="button" onClick={toggleSound} aria-label={soundOn ? 'Sound on' : 'Sound off'} aria-pressed={soundOn} className="grid h-7 w-7 place-items-center rounded-[var(--radius-sm)] text-[var(--color-ink-faint)] hover:bg-[var(--color-surface-muted)] hover:text-[var(--color-ink)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-focus)]">
              {soundOn ? <Volume2 className="h-3.5 w-3.5" /> : <VolumeX className="h-3.5 w-3.5" />}
            </button>
            {unread > 0 && (
              <button type="button" onClick={handleMarkAll} className="text-xs font-semibold text-[var(--color-accent-strong)] hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-focus)] rounded">
                Mark all read
              </button>
            )}
          </div>
        </div>
        {/* Filter tabs */}
        <div className="flex items-center gap-1 px-3 pb-2" role="tablist" aria-label="Notification filters">
          {(['all', 'unread', 'scans'] as const).map((t) => (
            <button
              key={t}
              type="button"
              role="tab"
              aria-selected={tab === t}
              onClick={() => setTab(t)}
              className={`rounded-[var(--radius-pill)] px-2.5 py-1 text-xs font-semibold font-mono border transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-focus)] ${tab === t ? 'bg-[var(--color-ink)] text-[var(--color-paper)] border-[var(--color-ink)]' : 'bg-[var(--color-surface-muted)] text-[var(--color-ink-faint)] border-[var(--color-rule)] hover:border-[var(--color-rule-strong)] hover:text-[var(--color-ink)]'}`}
            >
              {t === 'all' ? 'All' : t === 'unread' ? `Unread${unread ? ` · ${unread}` : ''}` : 'Scans'}
            </button>
          ))}
          {filtered.length > 0 && (
            <button type="button" onClick={handleClearAll} className="ml-auto inline-flex items-center gap-1 text-xs font-semibold text-[var(--color-ink-faint)] hover:text-[var(--color-danger)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-focus)] rounded px-1.5 py-1">
              <Trash2 className="h-3 w-3" /> Clear all
            </button>
          )}
        </div>
        <DropdownMenuSeparator className="m-0" />
        <div className="max-h-[50vh] overflow-y-auto">
          {filtered.length === 0 ? (
            <p className="px-3 py-8 text-center text-sm text-[var(--color-ink-faint)]">No notifications.</p>
          ) : (
            <div className="flex flex-col">
              {Array.from(grouped.entries()).map(([day, list]) => (
                <div key={day}>
                  <div className="sticky top-0 z-10 bg-[var(--color-surface-muted)] px-3 py-1 text-[10px] font-mono font-semibold uppercase tracking-widest text-[var(--color-ink-faint)] border-b border-[var(--color-rule-faint)]">{day}</div>
                  <ul className="flex flex-col" role="list" aria-label={day}>
                    {list.map((n) => (
                      <li
                        key={n.id}
                        className={`flex gap-2.5 px-3 py-2.5 border-b border-[var(--color-rule-faint)] last:border-0 ${!n.read ? 'bg-[var(--color-accent-soft)]/35' : 'bg-transparent'}`}
                      >
                        <span className={`mt-1 h-2 w-2 shrink-0 rounded-full ${kindDot(n.kind)} ${!n.read ? '' : 'opacity-60'}`} aria-hidden="true" />
                        <span className="min-w-0 flex-1">
                          <span className="flex items-start justify-between gap-2">
                            <strong className="text-sm font-semibold leading-5 text-[var(--color-ink)]">{n.title}</strong>
                            {!n.read && <span className="mt-1 h-1.5 w-1.5 shrink-0 rounded-full bg-[var(--color-accent)]" aria-label="Unread" />}
                          </span>
                          {n.message && <span className="block text-xs leading-4 text-[var(--color-ink-soft)]">{n.message}</span>}
                          <span className="mt-0.5 block text-[11px] text-[var(--color-ink-faint)]">{relativeTime(n.createdAt)}</span>
                        </span>
                        <span className="flex shrink-0 flex-col gap-1 self-start">
                          {!n.read && (
                            <button type="button" onClick={() => handleMarkOne(n.id)} aria-label={`Mark ${n.title} as read`} className="grid h-6 w-6 place-items-center rounded-[var(--radius-sm)] text-[var(--color-ink-faint)] hover:bg-[var(--color-surface-muted)] hover:text-[var(--color-ink)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-focus)]">
                              <Check className="h-3.5 w-3.5" />
                            </button>
                          )}
                        </span>
                      </li>
                    ))}
                  </ul>
                </div>
              ))}
            </div>
          )}
        </div>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
