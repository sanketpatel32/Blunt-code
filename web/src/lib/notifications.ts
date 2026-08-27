export type AppNotification = {
  id: string;
  title: string;
  message?: string;
  kind?: 'info' | 'success' | 'warning' | 'critical';
  createdAt: string;
  read: boolean;
};

const STORAGE_KEY = 'bluntcode.notifications';
const MAX_STORED = 50;

function safeRead(): AppNotification[] {
  try {
    const raw = window.localStorage.getItem(STORAGE_KEY);
    if (!raw) return [];
    const parsed = JSON.parse(raw) as AppNotification[];
    return Array.isArray(parsed) ? parsed : [];
  } catch {
    return [];
  }
}

function safeWrite(items: AppNotification[]) {
  try {
    window.localStorage.setItem(STORAGE_KEY, JSON.stringify(items.slice(0, MAX_STORED)));
  } catch { /* quota / private mode */ }
}

export function getNotifications(): AppNotification[] {
  if (typeof window === 'undefined') return [];
  return safeRead();
}

export function pushNotification(input: Omit<AppNotification, 'id' | 'createdAt' | 'read'> & Partial<Pick<AppNotification, 'read'>>): AppNotification {
  const item: AppNotification = {
    id: `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`,
    title: input.title,
    message: input.message,
    kind: input.kind ?? 'info',
    createdAt: new Date().toISOString(),
    read: input.read ?? false,
  };
  const next = [item, ...safeRead()].slice(0, MAX_STORED);
  safeWrite(next);
  try { window.dispatchEvent(new CustomEvent('bluntcode:notifications', { detail: next })); } catch { /* ignore */ }
  return item;
}

export function markAllRead(): AppNotification[] {
  const next = safeRead().map((n) => ({ ...n, read: true }));
  safeWrite(next);
  try { window.dispatchEvent(new CustomEvent('bluntcode:notifications', { detail: next })); } catch { }
  return next;
}

export function markRead(id: string): AppNotification[] {
  const next = safeRead().map((n) => (n.id === id ? { ...n, read: true } : n));
  safeWrite(next);
  try { window.dispatchEvent(new CustomEvent('bluntcode:notifications', { detail: next })); } catch { }
  return next;
}

export function clearNotifications() {
  safeWrite([]);
  try { window.dispatchEvent(new CustomEvent('bluntcode:notifications', { detail: [] })); } catch { }
}

// Optional stubs for scan lifecycle hooks
export function notifyScanCompleted(scanId: string, findings: number) {
  return pushNotification({ title: 'Scan completed', message: `${findings} finding${findings === 1 ? '' : 's'} collected · ${scanId.slice(0, 8)}`, kind: 'success' });
}
export function notifyNewCritical(count: number) {
  return pushNotification({ title: 'New critical finding', message: `${count} critical issue${count === 1 ? '' : 's'} detected`, kind: 'critical' });
}
export function notifyFixReady(ruleId: string) {
  return pushNotification({ title: 'Fix suggestion ready', message: `AI fix available for ${ruleId}`, kind: 'info' });
}
