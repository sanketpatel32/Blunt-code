import { useEffect, useMemo, useState } from 'react';
import { Bell } from 'lucide-react';
import { Button } from './ui/button';
import { Badge } from './ui/badge';
import { DropdownMenu, DropdownMenuContent, DropdownMenuTrigger, DropdownMenuLabel, DropdownMenuSeparator } from './ui/dropdown-menu';
import { getNotifications, markAllRead, type AppNotification } from '../lib/notifications';
import { relativeTime } from '../lib/format';

function kindDot(kind?: string) {
  if (kind === 'critical') return 'bg-[var(--color-danger)]';
  if (kind === 'success') return 'bg-[var(--color-success)]';
  if (kind === 'warning') return 'bg-[var(--color-warning)]';
  return 'bg-[var(--color-accent)]';
}

export function NotificationsCenter() {
  const [items, setItems] = useState<AppNotification[]>(() => getNotifications());

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

  const handleMarkAll = () => {
    const next = markAllRead();
    setItems(next);
  };

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
              <Badge className="absolute -right-1 -top-1 h-4 min-w-4 justify-center px-1 py-0 text-[10px] leading-none bg-[var(--color-danger)] text-white border-transparent">
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
          {unread > 0 && (
            <button type="button" onClick={handleMarkAll} className="text-xs font-semibold text-[var(--color-accent-strong)] hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-focus)] rounded">
              Mark all read
            </button>
          )}
        </div>
        <DropdownMenuSeparator className="m-0" />
        <div className="max-h-[50vh] overflow-y-auto">
          {items.length === 0 ? (
            <p className="px-3 py-8 text-center text-sm text-[var(--color-ink-faint)]">No notifications yet.</p>
          ) : (
            <ul className="flex flex-col" role="list">
              {items.map((n) => (
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
                </li>
              ))}
            </ul>
          )}
        </div>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
