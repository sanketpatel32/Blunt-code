import type { FindingFilter, SortState } from '../pages/report/ReportView';

export type QueryField = 'severity' | 'analyzer' | 'rule' | 'path' | 'status' | 'category';
export type QueryOp = '=' | '!=' | 'contains' | 'startsWith';
export type QueryLogic = 'AND' | 'OR';
export type QueryRow = { id: string; field: QueryField; op: QueryOp; value: string };
export type QueryGroup = { logic: QueryLogic; rows: QueryRow[] };

export const QUERY_FIELDS: QueryField[] = ['severity', 'analyzer', 'rule', 'path', 'status', 'category'];
export const QUERY_OPS: QueryOp[] = ['=', '!=', 'contains', 'startsWith'];
export const FIELD_LABELS: Record<QueryField, string> = {
  severity: 'Severity',
  analyzer: 'Analyzer',
  rule: 'Rule',
  path: 'Path',
  status: 'Status',
  category: 'Category',
};
export const OP_LABELS: Record<QueryOp, string> = {
  '=': '=',
  '!=': '≠',
  contains: 'contains',
  startsWith: 'starts with',
};

let idSeq = 0;
export function nextRowId(): string {
  idSeq += 1;
  return `qr-${Date.now()}-${idSeq}`;
}

export function emptyQueryGroup(): QueryGroup {
  return { logic: 'AND', rows: [{ id: nextRowId(), field: 'severity', op: '=', value: '' }] };
}

export function filterToQueryGroup(filter: FindingFilter): QueryGroup {
  const rows: QueryRow[] = [];
  const push = (field: QueryField, value: string) => {
    if (!value) return;
    // split comma for severity multi? keep as multiple rows with OR? use single row for now
    if (field === 'severity' && value.includes(',')) {
      for (const v of value.split(',').map((s) => s.trim()).filter(Boolean)) {
        rows.push({ id: nextRowId(), field, op: '=', value: v });
      }
      return;
    }
    rows.push({ id: nextRowId(), field, op: '=', value });
  };
  push('severity', filter.severity);
  push('category', filter.category);
  push('analyzer', filter.analyzer);
  push('rule', filter.rule);
  push('path', filter.path);
  push('status', filter.status);
  if (filter.q) rows.push({ id: nextRowId(), field: 'path', op: 'contains', value: filter.q });
  if (!rows.length) return { logic: 'AND', rows: [{ id: nextRowId(), field: 'severity', op: '=', value: '' }] };
  return { logic: 'AND', rows };
}

export function queryGroupToFilter(group: QueryGroup): FindingFilter {
  const f: FindingFilter = { severity: '', category: '', analyzer: '', rule: '', path: '', status: '', q: '' };
  const severities: string[] = [];
  for (const row of group.rows) {
    if (!row.value.trim()) continue;
    // only '=' with AND maps cleanly to FindingFilter
    if (row.op !== '=') {
      // contains/startsWith on path or generic => fold into q or path
      if (row.field === 'path' && (row.op === 'contains' || row.op === 'startsWith')) {
        if (!f.path) f.path = row.value;
        else f.q = row.value;
      } else {
        // non-equality => use q as fallback
        f.q = row.value;
      }
      continue;
    }
    switch (row.field) {
      case 'severity':
        severities.push(row.value);
        break;
      case 'category':
        if (!f.category) f.category = row.value;
        break;
      case 'analyzer':
        if (!f.analyzer) f.analyzer = row.value;
        break;
      case 'rule':
        if (!f.rule) f.rule = row.value;
        break;
      case 'path':
        if (!f.path) f.path = row.value;
        break;
      case 'status':
        if (!f.status) f.status = row.value;
        break;
    }
  }
  if (severities.length) f.severity = severities.join(',');
  // != is not representable; drop
  return f;
}

export function buildUrlSearchFromFilter(filters: FindingFilter, sort?: SortState, page?: number): string {
  const sp = new URLSearchParams();
  for (const k of ['severity', 'category', 'rule', 'status'] as const) if (filters[k]) sp.set(k, filters[k]);
  if (filters.analyzer) sp.set('analyzer', filters.analyzer);
  if (filters.path) sp.set('path', filters.path);
  if (filters.q) sp.set('q', filters.q);
  if (sort && (sort.key !== 'severity' || sort.dir !== 'asc')) {
    sp.set('sort', sort.key);
    sp.set('order', sort.dir);
  }
  if (page && page !== 1) sp.set('page', String(page));
  return sp.toString();
}

export function buildUrlSearch(group: QueryGroup, sort?: SortState, page?: number): string {
  return buildUrlSearchFromFilter(queryGroupToFilter(group), sort, page);
}

export function urlSearchToQueryGroup(search: string): QueryGroup {
  const sp = new URLSearchParams(search.startsWith('?') ? search.slice(1) : search);
  const f: FindingFilter = {
    severity: sp.get('severity') ?? '',
    category: sp.get('category') ?? '',
    analyzer: sp.get('tool') ?? sp.get('analyzer') ?? '',
    rule: sp.get('rule') ?? '',
    path: sp.get('path') ?? '',
    status: sp.get('status') ?? '',
    q: sp.get('q') ?? '',
  };
  return filterToQueryGroup(f);
}

export function previewSql(group: QueryGroup): string {
  if (!group.rows.length) return '— no conditions —';
  const parts = group.rows.map((r) => {
    const v = r.value ? `'${r.value.replace(/'/g, "\\'")}'` : "''";
    if (r.op === '=') return `${r.field} = ${v}`;
    if (r.op === '!=') return `${r.field} != ${v}`;
    if (r.op === 'contains') return `${r.field} CONTAINS ${v}`;
    return `${r.field} STARTS_WITH ${v}`;
  });
  return parts.join(` ${group.logic} `);
}

// Legacy alias for share button consumers
export { buildUrlSearch as buildUrlSearchFromGroup };
