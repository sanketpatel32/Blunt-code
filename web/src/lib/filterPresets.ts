import type { FindingFilter, SortState } from '../pages/report/ReportView';

export const VALID_SEVERITIES = ['critical', 'high', 'medium', 'low', 'info'] as const;
export const VALID_STATUSES = ['new', 'persistent', 'suppressed'] as const;
export const VALID_SORT_KEYS = ['severity', 'path', 'analyzer', 'status'] as const;

export function isValidSeverity(v: string) { return (VALID_SEVERITIES as readonly string[]).includes(v); }
export function isValidStatus(v: string) { return (VALID_STATUSES as readonly string[]).includes(v); }

export function serializeFiltersToSearch(filters: FindingFilter, sort: SortState, page: number): string {
  const sp = new URLSearchParams();
  for (const k of ['severity', 'category', 'rule', 'status'] as const) if (filters[k]) sp.set(k, filters[k]);
  if (filters.analyzer) sp.set('analyzer', filters.analyzer);
  if (filters.path) sp.set('path', filters.path);
  if (filters.q) sp.set('q', filters.q);
  if (sort.key !== 'severity' || sort.dir !== 'asc') { sp.set('sort', sort.key); sp.set('order', sort.dir); }
  if (page !== 1) sp.set('page', String(page));
  return sp.toString();
}

export function deserializeFiltersFromSearch(search: string): { filters: FindingFilter; sort: SortState; page: number } {
  const sp = new URLSearchParams(search);
  const filters: FindingFilter = {
    severity: isValidSeverity(sp.get('severity') ?? '') ? sp.get('severity')! : (sp.get('severity') ?? ''),
    category: sp.get('category') ?? '',
    analyzer: sp.get('tool') ?? sp.get('analyzer') ?? '',
    rule: sp.get('rule') ?? '',
    path: sp.get('path') ?? '',
    status: isValidStatus(sp.get('status') ?? '') ? sp.get('status')! : (sp.get('status') ?? ''),
    q: sp.get('q') ?? '',
  };
  const rawSort = sp.get('sort');
  const rawOrder = sp.get('order');
  let sort: SortState = { key: 'severity', dir: 'asc' };
  if (rawSort && (VALID_SORT_KEYS as readonly string[]).includes(rawSort)) {
    const key = rawSort as SortState['key'];
    sort = { key, dir: rawOrder === 'asc' || rawOrder === 'desc' ? rawOrder : (key === 'severity' ? 'desc' : 'asc') };
  }
  let page = 1;
  const rawPage = Number(sp.get('page'));
  if (Number.isFinite(rawPage) && rawPage >= 1) page = Math.floor(rawPage);
  return { filters, sort, page };
}

export function saveLastFilters(pageKey: string, filters: FindingFilter, sort: SortState) {
  try { localStorage.setItem(`bluntcode.filters.${pageKey}`, JSON.stringify({ filters, sort })); } catch { /* private mode */ }
}
export function loadLastFilters(pageKey: string): { filters: FindingFilter; sort: SortState } | null {
  try {
    const raw = localStorage.getItem(`bluntcode.filters.${pageKey}`);
    if (!raw) return null;
    const parsed = JSON.parse(raw);
    if (!parsed?.filters) return null;
    return parsed;
  } catch { return null; }
}
