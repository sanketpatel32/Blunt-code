import { useMemo, useState } from 'react';
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

export function ComplianceMatrix({ findings, scanId }: { findings: Finding[]; scanId?: string }) {
  const [expanded, setExpanded] = useState<OwaspId>();
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
      const status = count === 0 ? 'No evidence' : topSeverity === 'critical' || topSeverity === 'high' ? 'Review required' : 'Review';
      return { ...o, count, findings: list, topSeverity, pct, status };
    });
  }, [findings]);

  const handleRowClick = (id: OwaspId) => setExpanded((current) => current === id ? undefined : id);
  const selected = rows.find((row) => row.id === expanded);

  return (
    <section aria-label="Compliance matrix" className="rounded-[var(--radius-card)] border border-[var(--color-rule)] bg-[var(--color-surface)] shadow-[var(--shadow-card)] overflow-hidden">
      <div className="p-4 pb-2 flex items-center justify-between">
        <h3 className="font-display text-sm font-semibold tracking-tight">OWASP Top 10 (2021) — finding classification</h3>
        <span className="text-xs text-[var(--color-ink-faint)]">{rows.reduce((sum, row) => sum + row.count, 0)} {findings.length === 1 ? 'finding' : 'findings'} mapped</span>
      </div>
      <p className="availability-note mx-4 mb-3">Heuristic classification of loaded findings, not a compliance assessment. No evidence does not mean a category passed.</p>
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
            <TableRow key={r.id} className="cursor-pointer hover:bg-[var(--color-surface-muted)]" onClick={()=> handleRowClick(r.id as OwaspId)} tabIndex={0} role="button" aria-expanded={expanded === r.id} aria-label={`Show loaded findings for ${r.id} ${r.title}`} onKeyDown={(e)=> { if(e.key==='Enter'||e.key===' ') { e.preventDefault(); handleRowClick(r.id as OwaspId);} }}>
              <TableCell className="font-mono font-semibold">{r.id}</TableCell>
              <TableCell className="max-w-[14rem] truncate" title={r.title}>{r.title}</TableCell>
              <TableCell className="text-xs text-[var(--color-ink-faint)]">{r.cwe.join(', ')}</TableCell>
              <TableCell><Badge variant={r.count===0 ? 'secondary' : severityBadgeVariant(r.topSeverity)}>{r.count===0 ? '—' : r.topSeverity}</Badge></TableCell>
              <TableCell className="tabular-nums font-mono">{r.count}</TableCell>
              <TableCell className="min-w-[8rem]">
                <div className="h-2 w-full rounded-full bg-[var(--color-surface-muted)] overflow-hidden" role="progressbar" aria-valuenow={r.pct} aria-valuemin={0} aria-valuemax={100} aria-label={`${r.id} share of loaded findings ${r.pct}%`}>
                  <div className="h-full bg-[var(--color-accent)] transition-all" style={{ width: `${r.pct}%` }} />
                </div>
                <span className="text-xs text-[var(--color-ink-faint)]">{r.pct}%</span>
              </TableCell>
              <TableCell>
                <Badge variant={r.count === 0 ? 'secondary' : r.status === 'Review required' ? 'danger' : 'warning'}>{r.status}</Badge>
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
      {selected && <section className="p-4 border-t border-[var(--color-rule)]" aria-label={`${selected.id} loaded findings`}>
        <h4>{selected.id} · {selected.title}</h4>
        {selected.findings.length ? <ul className="space-y-2 mt-3">{selected.findings.map((finding) => <li key={finding.id}>
          <a href={`/scans/${encodeURIComponent(scanId ?? '')}?finding=${encodeURIComponent(finding.id)}`} className="text-[var(--color-accent-strong)]">{finding.title || finding.message}</a>
          <p className="text-xs text-[var(--color-ink-soft)]">{finding.severity} · {finding.relative_path || 'Project-level finding'}</p>
        </li>)}</ul> : <p className="muted">No loaded findings mapped to this category. This does not establish compliance.</p>}
      </section>}
    </section>
  );
}
