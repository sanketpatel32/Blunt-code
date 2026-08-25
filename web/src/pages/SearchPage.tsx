import { useEffect, useMemo, useState } from 'react';
import { api } from '../api';
import type { SearchedFinding } from '../types';
import type { Route } from '../lib/router';
import { analyzerName, findingLocation } from '../lib/format';
import { useLoad } from '../hooks/useLoad';
import { useDebouncedValue } from '../hooks/useDebouncedValue';
import { Empty, ErrorPanel } from '../components/ui';
import { MagnifierIcon } from '../components/icons';
import { SkeletonTable } from '../components/skeletons';

const SEARCH_PAGE_SIZE = 25;
const SEARCH_DEBOUNCE_MS = 250;
const SEVERITIES = ['critical', 'high', 'medium', 'low', 'info'] as const;
const ANALYZERS = ['ruff', 'biome', 'semgrep', 'sonarqube', 'secrets', 'todo'] as const;

/**
 * Global findings search: one query across every stored scan on this computer.
 * Results are severity-ranked by the server and link back to the report each
 * finding came from.
 */
export function SearchPage({ go }: { go: (route: Route) => void }) {
  const [query, setQuery] = useState('');
  const [severities, setSeverities] = useState<ReadonlySet<string>>(new Set());
  const [analyzer, setAnalyzer] = useState('');
  const [page, setPage] = useState(1);
  const debouncedQuery = useDebouncedValue(query, query ? SEARCH_DEBOUNCE_MS : 0);

  const params = useMemo(() => {
    const value: Record<string, string> = { page: String(page), page_size: String(SEARCH_PAGE_SIZE) };
    if (debouncedQuery) value.q = debouncedQuery;
    if (analyzer) value.analyzer = analyzer;
    if (severities.size) value.severity = [...severities].join(',');
    return value;
  }, [debouncedQuery, severities, analyzer, page]);

  // Any filter change returns to the first page synchronously so a stale page
  // number can never combine with a new filter window.
  useEffect(() => { setPage(1); }, [debouncedQuery, severities, analyzer]);

  const state = useLoad(() => api.searchFindings(params), [params.q, params.severity, params.analyzer, params.page]);
  const items = state.data?.items ?? [];
  const total = state.data?.total ?? 0;
  const pageSize = state.data?.page_size ?? SEARCH_PAGE_SIZE;
  const first = (page - 1) * pageSize;

  const toggleSeverity = (severity: string) => setSeverities((current) => {
    const next = new Set(current);
    if (next.has(severity)) next.delete(severity); else next.add(severity);
    return next;
  });

  return <div className="page">
    <header className="page-heading">
      <div>
        <p className="eyebrow">Search</p>
        <h1>Find findings everywhere</h1>
        <p>Searches every stored scan on this computer. Suppressed findings stay hidden.</p>
      </div>
    </header>
    <div className="filters" role="search">
      <input
        type="search"
        className="search-input"
        placeholder="Search message, rule, or path…"
        aria-label="Search findings"
        value={query}
        onChange={(event) => setQuery(event.target.value)}
      />
      <div className="severity-pills" role="group" aria-label="Filter by severity">
        {SEVERITIES.map((severity) => (
          <button key={severity} type="button" className={`severity-pill ${severities.has(severity) ? 'selected' : ''}`} aria-pressed={severities.has(severity)} onClick={() => toggleSeverity(severity)}>{severity}</button>
        ))}
      </div>
      <select className="search-analyzer" aria-label="Filter by analyzer" value={analyzer} onChange={(event) => setAnalyzer(event.target.value)}>
        <option value="">All analyzers</option>
        {ANALYZERS.map((id) => <option key={id} value={id}>{analyzerName(id)}</option>)}
      </select>
    </div>
    {state.loading ? <SkeletonTable rows={8} cols={4} />
      : state.error ? <ErrorPanel error={state.error} retry={state.reload} />
        : total === 0 ? <Empty title="No matching findings" icon={<MagnifierIcon />}>Run a scan or loosen the filters — only scans already stored on this computer are searched.</Empty>
          : <>
            <div className="table-wrap"><table className="search-results"><thead><tr><th scope="col">Severity</th><th scope="col">Finding</th><th scope="col">Location</th><th scope="col"><span className="sr-only">Actions</span></th></tr></thead><tbody>
              {items.map((finding: SearchedFinding) => <tr key={`${finding.scan_id}:${finding.id}`}>
                <td><span className={`severity ${finding.severity}`}>{finding.severity}</span></td>
                <td className="finding-summary">{finding.title ? <strong>{finding.title}</strong> : null}<span>{finding.message}</span>{finding.rule_id ? <code className="badge">{finding.rule_id}</code> : null}</td>
                <td><code>{findingLocation(finding)}</code></td>
                <td><a href={`/scans/${finding.scan_id}`} onClick={(event) => { event.preventDefault(); go({ page: 'scan', id: finding.scan_id }); }}>Open report</a></td>
              </tr>)}
            </tbody></table></div>
            <nav className="findings-pagination" aria-label="Search result pagination">
              <span>Showing {total === 0 ? 0 : first + 1}–{Math.min(first + items.length, total)} of {total}</span>
              <div>
                <button type="button" className="button secondary" onClick={() => setPage(page - 1)} disabled={page <= 1}>Previous</button>
                <output aria-live="polite">Page {page}</output>
                <button type="button" className="button secondary" onClick={() => setPage(page + 1)} disabled={!state.data?.has_next}>Next</button>
              </div>
            </nav>
          </>}
  </div>;
}
