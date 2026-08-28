import { useMemo } from 'react';
import type { Finding } from '../types';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from './ui/table';
import { Badge } from './ui/badge';

export type OwaspId = `A0${number}` | 'A10';

export const OWASP_TOP10: Array<{ id: OwaspId; title: string; cwe: string[]; categoryHints: string[] }> = [
  { id: 'A01', title: 'Broken Access Control', cwe: ['CWE-284','CWE-862'], categoryHints: ['access','authz','idor','rbac'] },
  { id: 'A02', title: 'Cryptographic Failures', cwe: ['CWE-310','CWE-327'], categoryHints: ['crypto','crypt','secrets','hardcoded'] },
  { id: 'A03', title: 'Injection', cwe: ['CWE-20','CWE-89','CWE-78'], categoryHints: ['injection','sqli','xss','command'] },
  { id: 'A04', title: 'Insecure Design', cwe: ['CWE-657','CWE-799'], categoryHints: ['design','threat'] },
  { id: 'A05', title: 'Security Misconfiguration', cwe: ['CWE-16','CWE-611'], categoryHints: ['misconfig','config','dockerfile','yaml'] },
  { id: 'A06', title: 'Vulnerable and Outdated Components', cwe: ['CWE-937','CWE-1104'], categoryHints: ['dependencies','vulnerable','outdated'] },
  { id: 'A07', title: 'Identification and Authentication Failures', cwe: ['CWE-287','CWE-384'], categoryHints: ['auth','authentication','session'] },
  { id: 'A08', title: 'Software and Data Integrity Failures', cwe: ['CWE-829','CWE-502'], categoryHints: ['integrity','deserialization','supply'] },
  { id: 'A09', title: 'Security Logging and Monitoring Failures', cwe: ['CWE-117','CWE-223'], categoryHints: ['logging','monitoring','todo'] },
  { id: 'A10', title: 'Server-Side Request Forgery (SSRF)', cwe: ['CWE-918'], categoryHints: ['ssrf','request'] },
];

// heuristic mapping finding -> owasp id
function mapFindingToOwasp(f: Finding): OwaspId | null {
  const hay = `${f.category} ${f.rule_id ?? ''} ${f.title ?? ''} ${f.message}`.toLowerCase();
  for (const o of OWASP_TOP10) {
    if (o.categoryHints.some((h) => hay.includes(h))) return o.id as OwaspId;
  }
  // fallback by analyzer/category
  if (f.category === 'secrets' || hay.includes('secret') || hay.includes('hardcoded')) return 'A02';
  if (f.category === 'dependencies' || hay.includes('depend')) return 'A06';
  if (f.category === 'container' || f.category === 'iac') return 'A05';
  if (hay.includes('injection') || hay.includes('xss') || hay.includes('sqli')) return 'A03';
  if (hay.includes('auth')) return 'A07';
  // no match
  return null;
}

function severityRank(s: string): number {
  const m: Record<string,number> = { critical: 4, high: 3, medium: 2, low: 1, info: 0 };
  return m[s] ?? -1;
}
function severityBadgeVariant(s: string) {
  if (s === 'critical' || s === 'high') return 'danger' as const;
  if (s === 'medium') return 'warning' as const;
  if (s === 'low') return 'secondary' as const;
  return 'secondary' as const;
}

export function ComplianceMatrix({ findings, scanId, onFilterOwasp }: { findings: Finding[]; scanId?: string; onFilterOwasp?: (id: OwaspId) => void }) {
  const rows = useMemo(() => {
    const byOwasp = new Map<string, Finding[]>();
    for (const o of OWASP_TOP10) byOwasp.set(o.id, []);
    const unmapped: Finding[] = [];
    for (const f of findings) {
      const ow = mapFindingToOwasp(f);
      if (ow) byOwasp.get(ow)!.push(f);
      else unmapped.push(f);
    }
    const total = findings.length || 1;
    return OWASP_TOP10.map((o) => {
      const list = byOwasp.get(o.id)!;
      const count = list.length;
      const topSeverity = list.length ? [...list].sort((a,b)=> severityRank(b.severity)-severityRank(a.severity))[0].severity : 'info';
      const pct = Math.round((count/total)*100);
      const status = count === 0 ? 'Pass' : topSeverity === 'critical' || topSeverity === 'high' ? 'Fail' : count > 0 ? 'Review' : 'Pass';
      return { ...o, count, findings: list, topSeverity, pct, status };
    });
  }, [findings]);

  const handleRowClick = (id: OwaspId) => {
    if (onFilterOwasp) { onFilterOwasp(id); return; }
    if (scanId) {
      const ow = OWASP_TOP10.find((o)=> o.id===id);
      const q = ow ? ow.categoryHints[0] : id;
      const url = `/scans/${scanId}?q=${encodeURIComponent(q)}`;
      window.history.pushState(null,'',url);
      window.dispatchEvent(new PopStateEvent('popstate'));
    }
  };

  return (
    <section aria-label="Compliance matrix" className="rounded-[var(--radius-card)] border border-[var(--color-rule)] bg-[var(--color-surface)] shadow-[var(--shadow-card)] overflow-hidden">
      <div className="p-4 pb-2 flex items-center justify-between">
        <h3 className="font-display text-sm font-semibold tracking-tight">Compliance — OWASP Top 10 (2021) + CWE Top 25</h3>
        <span className="text-xs text-[var(--color-ink-faint)]">{findings.length} findings mapped</span>
      </div>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>OWASP</TableHead>
            <TableHead>Title</TableHead>
            <TableHead>CWE</TableHead>
            <TableHead>Severity</TableHead>
            <TableHead>Count</TableHead>
            <TableHead>Findings</TableHead>
            <TableHead>Status</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {rows.map((r) => (
            <TableRow key={r.id} className="cursor-pointer hover:bg-[var(--color-surface-muted)]" onClick={()=> handleRowClick(r.id as OwaspId)} tabIndex={0} role="button" aria-label={`Filter report by ${r.id} ${r.title}`} onKeyDown={(e)=> { if(e.key==='Enter'||e.key===' ') { e.preventDefault(); handleRowClick(r.id as OwaspId);} }}>
              <TableCell className="font-mono font-semibold">{r.id}</TableCell>
              <TableCell className="max-w-[14rem] truncate" title={r.title}>{r.title}</TableCell>
              <TableCell className="text-xs text-[var(--color-ink-faint)]">{r.cwe.join(', ')}</TableCell>
              <TableCell><Badge variant={r.count===0 ? 'secondary' : severityBadgeVariant(r.topSeverity)}>{r.count===0 ? '—' : r.topSeverity}</Badge></TableCell>
              <TableCell className="tabular-nums font-mono">{r.count}</TableCell>
              <TableCell className="min-w-[8rem]">
                <div className="h-2 w-full rounded-full bg-[var(--color-surface-muted)] overflow-hidden" role="progressbar" aria-valuenow={r.pct} aria-valuemin={0} aria-valuemax={100} aria-label={`${r.id} coverage ${r.pct}%`}>
                  <div className="h-full bg-[var(--color-accent)] transition-all" style={{ width: `${r.pct}%` }} />
                </div>
                <span className="text-[10px] text-[var(--color-ink-faint)]">{r.pct}%</span>
              </TableCell>
              <TableCell>
                <Badge variant={r.status==='Pass' ? 'success' : r.status==='Fail' ? 'danger' : 'warning'}>{r.status}</Badge>
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </section>
  );
}
