import '../css/search.css';
import { Fragment, useEffect, useMemo, useRef, useState, type KeyboardEvent as ReactKeyboardEvent, type MouseEvent as ReactMouseEvent } from 'react';
import { api } from '../api';
import type { SearchedFinding, Workspace } from '../types';
import type { Route } from '../lib/router';
import { analyzerName, findingLocation, friendlyFindingTitle, shortFindingLocation } from '../lib/format';
import { useLoad } from '../hooks/useLoad';
import { useDebouncedValue } from '../hooks/useDebouncedValue';
import { Empty, ErrorPanel } from '../components/ui';
import { MagnifierIcon } from '../components/icons';
import { SkeletonTable } from '../components/skeletons';
import { SavedFilters } from '../components/SavedFilters';
import type { FindingFilter } from './report/ReportView';
import { QueryBuilder } from '../components/QueryBuilder';
import { filterToQueryGroup, type QueryGroup } from '../lib/queryBuilder';
import { Sheet, SheetContent, SheetHeader, SheetTitle, SheetDescription } from '../components/ui/sheet';
import { PageHeader } from '../components/PageHeader';
import { Badge } from '../components/ui/badge';
import { Button } from '../components/ui/button';
import { Card } from '../components/ui/card';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '../components/ui/select';
import {
  Search,
  Filter,
  SlidersHorizontal,
  ChevronDown,
  ShieldAlert,
  Zap,
  KeyRound,
  LayoutList,
  LayoutGrid,
  X,
  ExternalLink,
  RotateCcw,
  Bookmark,
  Plus,
} from 'lucide-react';
import { analyzerMeta } from '../lib/analyzerCatalog';

const SEARCH_PAGE_SIZE = 25;
const SEARCH_DEBOUNCE_MS = 250;
const SEVERITIES = ['critical', 'high', 'medium', 'low', 'info'] as const;
const ANALYZERS = [
  'pentest',
  'semgrep',
  'secrets',
  'gitleaks-secrets',
  'osv-dependencies',
  'container-trivy',
  'iac-checkov',
  'sonarqube',
  'ruff',
  'biome',
  'license-scan',
  'todo',
] as const;

const SEARCH_COLUMNS = [
  ['finding', 'Finding'],
  ['location', 'Location'],
  ['severity', 'Severity'],
  ['actions', 'Actions'],
] as const;

/** Accessible name for the clickable result rows/cards — mirrors ReportView's row labeling. */
function findingActionLabel(finding: SearchedFinding) {
  const name = finding.title ?? finding.rule_id ?? 'finding';
  return `${finding.severity} ${name}${finding.relative_path ? ` in ${finding.relative_path}` : ''} — open quick look`;
}

/**
 * Rule engines echo their own id as the message lead ("aws-access-token: Identified…")
 * while the title already shows the id — rendering both gives "aws-access-token:
 * aws-access-token: …". The page-local strip (lib/format is shared and frozen for
 * this wave) drops the "${lead}: " prefix so the id never appears twice.
 */
function stripMessagePrefix(finding: Pick<SearchedFinding, 'title' | 'rule_id' | 'message'>): string {
  const lead = finding.title || finding.rule_id;
  const message = finding.message ?? '';
  if (!lead) return message;
  const prefix = `${lead}: `;
  return message.startsWith(prefix) ? message.slice(prefix.length) : message;
}

/** The heading a result deserves: a real backend title wins; a title that merely
 *  echoes the rule id is upgraded to the first clause of the prefix-stripped
 *  message via the shared formatter, so an opaque id never leads a row. */
function findingDisplayTitle(finding: Pick<SearchedFinding, 'title' | 'rule_id' | 'message'>): string {
  const realTitle = finding.title && finding.title !== finding.rule_id ? finding.title : undefined;
  return friendlyFindingTitle({ title: realTitle, rule_id: finding.rule_id, message: stripMessagePrefix(finding) });
}

/** Grouping key: consecutive results sharing rule + file collapse into one group. */
function resultGroupKey(finding: SearchedFinding) {
  return `${finding.rule_id ?? ''}\u0000${finding.relative_path ?? ''}`;
}

type SearchRow =
  | { kind: 'single'; finding: SearchedFinding }
  | { kind: 'group'; key: string; occurrences: SearchedFinding[] };

/** Collapse consecutive same rule+file results into groups (client-side; the API
 *  window is untouched). With 80k findings, six identical consecutive rows are
 *  unusable — one header with an occurrence count plus deduped line lines reads. */
function buildSearchRows(items: SearchedFinding[], grouped: boolean): SearchRow[] {
  const rows: SearchRow[] = [];
  for (const finding of items) {
    const last = rows[rows.length - 1];
    const key = resultGroupKey(finding);
    if (grouped && last && (last.kind === 'group' ? last.key : resultGroupKey(last.finding)) === key) {
      if (last.kind === 'group') last.occurrences.push(finding);
      else rows[rows.length - 1] = { kind: 'group', key, occurrences: [last.finding, finding] };
    } else {
      rows.push({ kind: 'single', finding });
    }
  }
  return rows;
}

function useSavedSearches() {
  const key = 'bluntcode.savedSearches';
  const load = (): string[] => {
    try {
      const r = localStorage.getItem(key);
      return r ? JSON.parse(r) : [];
    } catch {
      return [];
    }
  };
  const [list, setList] = useState<string[]>(load);
  const add = (q: string) => {
    const t = q.trim();
    if (!t || list.includes(t)) return;
    const nxt = [t, ...list].slice(0, 10);
    setList(nxt);
    try {
      localStorage.setItem(key, JSON.stringify(nxt));
    } catch {}
  };
  const remove = (q: string) => {
    const nxt = list.filter((x) => x !== q);
    setList(nxt);
    try {
      localStorage.setItem(key, JSON.stringify(nxt));
    } catch {}
  };
  return { list, add, remove };
}

export function SearchPage({ go }: { go: (route: Route) => void }) {
  const [query, setQuery] = useState(() => new URLSearchParams(window.location.search).get('q') ?? '');
  const [severities, setSeverities] = useState<ReadonlySet<string>>(() => {
    const v = new URLSearchParams(window.location.search).get('severity');
    return new Set(v ? v.split(',').filter(Boolean) : []);
  });
  const [analyzer, setAnalyzer] = useState(() => new URLSearchParams(window.location.search).get('analyzer') ?? '');
  const [workspace, setWorkspace] = useState(() => new URLSearchParams(window.location.search).get('workspace') ?? '');
  const [visibleCols, setVisibleCols] = useState({ finding: true, location: true, severity: true, actions: true });
  const [viewMode, setViewMode] = useState<'cards' | 'table'>('table');
  const [selectedFinding, setSelectedFinding] = useState<SearchedFinding | null>(null);

  const [page, setPage] = useState(() => {
    const p = Number(new URLSearchParams(window.location.search).get('page'));
    return Number.isFinite(p) && p >= 1 ? Math.floor(p) : 1;
  });
  const [pageSize] = useState(SEARCH_PAGE_SIZE);
  const [facetsOpen, setFacetsOpen] = useState(false);
  const [advancedOpen, setAdvancedOpen] = useState(false);
  const [queryGroup, setQueryGroup] = useState<QueryGroup>(() =>
    filterToQueryGroup({
      severity: [...(new URLSearchParams(window.location.search).get('severity')?.split(',').filter(Boolean) ?? [])].join(','),
      category: '',
      analyzer: new URLSearchParams(window.location.search).get('analyzer') ?? '',
      rule: '',
      path: new URLSearchParams(window.location.search).get('workspace') ?? '',
      status: '',
      q: new URLSearchParams(window.location.search).get('q') ?? '',
    } as FindingFilter)
  );

  const debouncedQuery = useDebouncedValue(query, query ? SEARCH_DEBOUNCE_MS : 0);
  const workspacesList = useLoad(api.workspaces, []);
  const workspaceOptions = workspacesList.data ?? [];

  const params = useMemo(() => {
    const value: Record<string, string> = { page: String(page), page_size: String(pageSize) };
    if (debouncedQuery) value.q = debouncedQuery;
    if (analyzer) value.analyzer = analyzer;
    if (severities.size) value.severity = [...severities].join(',');
    // The server scopes by workspace_id; the address bar keeps the shorter `workspace` key.
    if (workspace) value.workspace_id = workspace;
    return value;
  }, [debouncedQuery, severities, analyzer, workspace, page, pageSize]);

  // Reset to page 1 whenever a filter changes — but skip the very first run so a
  // deep link like /search?q=error&page=2 keeps its URL-initialized page.
  const didMount = useRef(false);
  useEffect(() => {
    if (!didMount.current) {
      didMount.current = true;
      return;
    }
    setPage(1);
  }, [debouncedQuery, severities, analyzer, workspace]);

  // URL sync
  useEffect(() => {
    const sp = new URLSearchParams();
    if (debouncedQuery) sp.set('q', debouncedQuery);
    if (analyzer) sp.set('analyzer', analyzer);
    if (severities.size) sp.set('severity', [...severities].join(','));
    if (workspace) sp.set('workspace', workspace);
    if (page !== 1) sp.set('page', String(page));
    const qs = sp.toString();
    const cur = window.location.search.replace(/^\?/, '');
    if (qs === cur) return;
    const next = qs ? `${window.location.pathname}?${qs}${window.location.hash}` : `${window.location.pathname}${window.location.hash}`;
    window.history.replaceState(null, '', next);
  }, [debouncedQuery, analyzer, severities, workspace, page]);

  useEffect(() => {
    const onPop = () => {
      const sp = new URLSearchParams(window.location.search);
      setQuery(sp.get('q') ?? '');
      setAnalyzer(sp.get('analyzer') ?? '');
      setWorkspace(sp.get('workspace') ?? '');
      const v = sp.get('severity');
      setSeverities(new Set(v ? v.split(',').filter(Boolean) : []));
      const p = Number(sp.get('page'));
      setPage(Number.isFinite(p) && p >= 1 ? Math.floor(p) : 1);
      setQueryGroup(
        filterToQueryGroup({
          severity: v ?? '',
          category: '',
          analyzer: sp.get('analyzer') ?? '',
          rule: '',
          path: sp.get('workspace') ?? '',
          status: '',
          q: sp.get('q') ?? '',
        } as FindingFilter)
      );
    };
    window.addEventListener('popstate', onPop);
    return () => window.removeEventListener('popstate', onPop);
  }, []);

  useEffect(() => {
    setQueryGroup(
      filterToQueryGroup({
        severity: [...severities].join(','),
        category: '',
        analyzer,
        rule: '',
        path: workspace,
        status: '',
        q: query,
      } as FindingFilter)
    );
  }, [severities, analyzer, workspace, query]);

  const state = useLoad(() => api.searchFindings(params), [params.q, params.severity, params.analyzer, params.page, params.page_size, params.workspace_id]);
  const items = state.data?.items ?? [];
  const total = state.data?.total ?? 0;
  const actualPageSize = state.data?.page_size ?? pageSize;
  const first = (page - 1) * actualPageSize;
  const saved = useSavedSearches();

  // Snap out-of-range pages (deep links like ?page=999999, or a result set that
  // shrank since the page was picked) back to the last page instead of issuing a
  // huge wasted OFFSET query. An empty result set (total 0) snaps to page 1.
  useEffect(() => {
    if (!state.data) return;
    const pageCount = Math.max(1, Math.ceil(state.data.total / (state.data.page_size || pageSize)));
    if (page > pageCount) setPage(pageCount);
  }, [state.data, page, pageSize]);

  const toggleSeverity = (severity: string) =>
    setSeverities((current) => {
      const next = new Set(current);
      if (next.has(severity)) next.delete(severity);
      else next.add(severity);
      return next;
    });

  /** Result rows and cards open the quick-look drawer on click; Enter/Space get the same behavior. */
  const openFromKeyboard = (event: ReactKeyboardEvent, finding: SearchedFinding) => {
    if (event.key !== 'Enter' && event.key !== ' ') return;
    event.preventDefault();
    setSelectedFinding(finding);
  };

  /** Navigate to the finding's originating scan report (the one visible row action). */
  const openReport = (event: ReactMouseEvent, finding: SearchedFinding) => {
    event.preventDefault();
    event.stopPropagation();
    go({ page: 'scan', id: finding.scan_id });
  };

  // Severity facet counts: the API's `severity_counts` totals every matched
  // finding across the whole result set. Legacy payloads omit the field —
  // counting only this page's rows would read as authoritative ("High 0" against
  // 77k matches), so the badges are omitted instead.
  const severityCounts = state.data?.severity_counts as Record<string, number> | undefined;

  // Analyzer facet counts have no server facet; the served page is the only
  // honest source, so chips carry page-local counts and say so in the facet
  // title. Zero only ever means "not on this page" — never "no matches" — so
  // zero chips dim but stay clickable.
  const analyzerPageCounts = useMemo(() => {
    const counts: Record<string, number> = {};
    for (const finding of items) counts[finding.analyzer_id] = (counts[finding.analyzer_id] ?? 0) + 1;
    return counts;
  }, [items]);

  // Rule+file grouping: defaults on when this page holds consecutive duplicates;
  // null means "follow the default" so a fresh result set re-decides.
  const [grouping, setGrouping] = useState<boolean | null>(null);
  const hasDuplicates = useMemo(() => {
    for (let i = 1; i < items.length; i++) {
      if (resultGroupKey(items[i]) === resultGroupKey(items[i - 1])) return true;
    }
    return false;
  }, [items]);
  const grouped = grouping ?? hasDuplicates;
  const rows = useMemo(() => buildSearchRows(items, grouped), [items, grouped]);

  const activeFiltersCount = useMemo(() => {
    let count = 0;
    if (query.trim()) count++;
    if (severities.size > 0) count += severities.size;
    if (analyzer) count++;
    if (workspace) count++;
    return count;
  }, [query, severities, analyzer, workspace]);

  const clearAll = () => {
    setQuery('');
    setSeverities(new Set());
    setAnalyzer('');
    setWorkspace('');
    setPage(1);
  };

  const applyPreset = (preset: 'critical_high' | 'security' | 'secrets') => {
    clearAll();
    switch (preset) {
      case 'critical_high':
        setSeverities(new Set(['critical', 'high']));
        break;
      case 'security':
        setAnalyzer('pentest');
        setSeverities(new Set(['critical', 'high', 'medium']));
        break;
      case 'secrets':
        setAnalyzer('secrets');
        setSeverities(new Set(['critical', 'high']));
        break;
    }
  };

  const facets = (
    <div className="search-facets space-y-5">
      {/* Severities */}
      <div className="facet-section">
        <div className="flex items-center justify-between">
          <p className="facet-title text-xs font-semibold uppercase tracking-wider text-[var(--color-ink-faint)]">Severity</p>
          {severities.size > 0 && (
            <button type="button" onClick={() => setSeverities(new Set())} className="text-[11px] text-[var(--color-accent-strong)] hover:underline">
              Reset
            </button>
          )}
        </div>
        <fieldset className="severity-pills mt-2 flex flex-col gap-1.5" aria-label="Filter by severity">
          {SEVERITIES.map((sev) => {
            const isSelected = severities.has(sev);
            return (
              <button
                key={sev}
                type="button"
                className={`severity-pill flex items-center justify-between rounded-[var(--radius-button)] px-2.5 py-1.5 text-xs font-medium transition-all ${
                  isSelected
                    ? 'bg-[var(--color-surface-subtle)] border border-[var(--color-accent)] text-[var(--color-ink)] font-semibold shadow-xs'
                    : 'border border-transparent text-[var(--color-ink-soft)] hover:bg-[var(--color-surface-muted)] hover:text-[var(--color-ink)]'
                }`}
                aria-pressed={isSelected}
                onClick={() => toggleSeverity(sev)}
              >
                <div className="flex items-center gap-2">
                  <span
                    className="h-2 w-2 rounded-full severity-dot"
                    style={{
                      background:
                        sev === 'critical'
                          ? 'var(--color-danger)'
                          : sev === 'high'
                          ? 'var(--color-danger)'
                          : sev === 'medium'
                          ? 'var(--color-warning)'
                          : 'var(--color-ink-faint)',
                    }}
                  />
                  <span className="capitalize">{sev}</span>
                </div>
                {severityCounts && (
                  <Badge variant={isSelected ? 'outline' : 'secondary'} className="text-[10px] tabular-nums px-1.5 py-0">
                    {severityCounts[sev] ?? 0}
                  </Badge>
                )}
              </button>
            );
          })}
        </fieldset>
      </div>

      {/* Workspaces */}
      <div className="facet-section">
        <p className="facet-title text-xs font-semibold uppercase tracking-wider text-[var(--color-ink-faint)]">Workspace Scope</p>
        <div className="mt-2 space-y-2">
          {/* Names, not raw ids: the URL keeps persisting the workspace id, the
              select displays the human name for it. A deep-linked id stays an
              honest option until (and unless) the list can resolve its name. */}
          <Select value={workspace || 'all'} onValueChange={(val) => setWorkspace(val === 'all' ? '' : val)}>
            <SelectTrigger className="w-full text-xs h-8" aria-label="Filter by workspace">
              <SelectValue placeholder="All workspaces" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">All workspaces</SelectItem>
              {workspaceOptions.map((w: Workspace) => (
                <SelectItem key={w.id} value={w.id}>
                  {w.name || w.id}
                </SelectItem>
              ))}
              {workspace && !workspaceOptions.some((w) => w.id === workspace) && (
                <SelectItem value={workspace}>{workspace}</SelectItem>
              )}
            </SelectContent>
          </Select>
        </div>
      </div>

      {/* Analyzer engines — shared .chip styling (state via aria-pressed), page-local
          counts. Zero-count engines are omitted, not dimmed: a page-local "0" chip is
          an unusable filter presented as noise; the facet hint keeps counts honest. */}
      <div className="facet-section">
        <div className="flex items-center justify-between">
          <p className="facet-title text-xs font-semibold uppercase tracking-wider text-[var(--color-ink-faint)]">
            Engine <span className="search-facet-hint">· this page</span>
          </p>
          {analyzer && (
            <button type="button" onClick={() => setAnalyzer('')} className="text-[11px] text-[var(--color-accent-strong)] hover:underline">
              All
            </button>
          )}
        </div>
        <fieldset className="chip-group mt-2" aria-label="Filter by analyzer">
          <button type="button" className="chip" aria-pressed={analyzer === ''} onClick={() => setAnalyzer('')}>
            All engines
          </button>
          {/* The active selection stays visible even at zero so the current filter
              is always legible. */}
          {ANALYZERS.filter((id) => (analyzerPageCounts[id] ?? 0) > 0 || analyzer === id).map((id) => {
            const meta = analyzerMeta(id);
            const count = analyzerPageCounts[id] ?? 0;
            return (
              <button
                key={id}
                type="button"
                className="chip search-engine-chip"
                aria-pressed={analyzer === id}
                onClick={() => setAnalyzer(id)}
              >
                {meta?.displayName ?? analyzerName(id)}
                <small className="chip-count">{count}</small>
              </button>
            );
          })}
        </fieldset>
      </div>

      {/* Saved Searches */}
      <div className="facet-saved pt-3 border-t border-[var(--color-rule-faint)]">
        <div className="flex items-center justify-between">
          <p className="text-xs font-semibold uppercase tracking-wider text-[var(--color-ink-faint)] flex items-center gap-1.5">
            <Bookmark className="h-3 w-3" /> Saved Searches
          </p>
          {query.trim() && (
            <button
              type="button"
              className="text-[11px] text-[var(--color-accent-strong)] hover:underline flex items-center gap-0.5"
              onClick={() => saved.add(query)}
            >
              <Plus className="h-3 w-3" /> Save current
            </button>
          )}
        </div>
        <div className="mt-2 space-y-1">
          {saved.list.length === 0 ? (
            <span className="text-xs text-[var(--color-ink-faint)]">No saved search presets yet.</span>
          ) : (
            saved.list.map((q) => (
              <div key={q} className="saved-preset-row flex items-center justify-between rounded-[var(--radius-sm)] px-2 py-1 bg-[var(--color-surface-muted)] text-xs text-[var(--color-ink)] hover:bg-[var(--color-surface-subtle)]">
                <button type="button" className="truncate text-left flex-1" onClick={() => setQuery(q)}>
                  {q}
                </button>
                <button type="button" aria-label={`Delete ${q}`} onClick={() => saved.remove(q)} className="text-[var(--color-ink-faint)] hover:text-[var(--color-danger)] ml-1">
                  ×
                </button>
              </div>
            ))
          )}
        </div>
      </div>

      <SavedFilters
        filters={
          {
            severity: [...severities].join(','),
            analyzer,
            q: query,
            status: '',
            category: '',
            rule: '',
            path: workspace,
          } as FindingFilter
        }
        onLoad={(f) => {
          setSeverities(new Set(f.severity ? f.severity.split(',') : []));
          setAnalyzer(f.analyzer);
          setQuery(f.q);
          setWorkspace(f.path);
        }}
      />
    </div>
  );

  /** Rule tag next to a display title — skipped when the title already is the
   *  rule id (or leads with it), so one row never renders the id twice. */
  const ruleTagFor = (finding: SearchedFinding, title: string) =>
    finding.rule_id && finding.rule_id !== title ? (
      <code className="tag" key="rule">
        <span>{finding.rule_id}</span>
      </code>
    ) : null;

  const visibleColCount = Math.max(1, Object.values(visibleCols).filter(Boolean).length);

  /** One visible action per row: "Open" jumps to the finding's scan report.
   *  Everything else (quick look) stays on the row click / keyboard. */
  const openLink = (finding: SearchedFinding) => (
    <a href={`/scans/${finding.scan_id}`} className="search-row-open" onClick={(event) => openReport(event, finding)}>
      Open
    </a>
  );

  return (
    <div className="page page-search search-page space-y-3">
      <PageHeader
        eyebrow="Findings Search"
        title="Search findings"
        description="Searches every stored scan on this computer. Suppressed findings stay hidden."
        actions={
          <>
            <Button variant="outline" size="sm" onClick={() => setAdvancedOpen(true)} aria-expanded={advancedOpen} aria-controls="search-advanced" className="gap-1.5 text-xs h-7 px-2.5">
              <SlidersHorizontal className="h-3 w-3" /> Advanced
            </Button>
            <Button variant="outline" size="sm" onClick={() => setFacetsOpen((v) => !v)} className="md:hidden gap-1.5 text-xs h-7 px-2.5">
              <Filter className="h-3 w-3" /> Filters {activeFiltersCount > 0 && `(${activeFiltersCount})`}
            </Button>
            {activeFiltersCount > 0 && (
              <Button variant="ghost" size="sm" onClick={clearAll} className="gap-1.5 text-xs h-7 px-2 text-[var(--color-ink-soft)] hover:text-[var(--color-danger)]">
                <RotateCcw className="h-3 w-3" /> Clear
              </Button>
            )}
          </>
        }
      />

      {/* Omnisearch Bar & Quick Presets */}
      <div className="space-y-2">
        <div className="search-toolbar relative flex items-center shadow-xs" role="search">
          <Search className="pointer-events-none absolute left-3 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-[var(--color-ink-faint)]" aria-hidden="true" />
          <input
            type="search"
            className="search-input w-full rounded-[var(--radius-md)] border border-[var(--color-rule)] bg-[var(--color-surface)] py-2 pl-9 pr-10 text-xs sm:text-sm text-[var(--color-ink)] shadow-xs transition-all placeholder:text-[var(--color-ink-faint)] focus:border-[var(--color-accent)] focus:outline-none focus:ring-1 focus:ring-[var(--color-focus)]"
            placeholder="Search message text, vulnerability rules (e.g. sqli, xss, cwe), file paths, or CWE IDs…"
            aria-label="Search findings"
            value={query}
            onChange={(event) => setQuery(event.target.value)}
          />
          {query && (
            <button
              type="button"
              onClick={() => setQuery('')}
              className="absolute right-3.5 top-1/2 -translate-y-1/2 text-[var(--color-ink-faint)] hover:text-[var(--color-ink)] p-1 rounded-full"
              aria-label="Clear search input"
            >
              <X className="h-4 w-4" />
            </button>
          )}
        </div>

        {/* Quick Filter Chips */}
        <div className="flex flex-wrap items-center gap-1.5 text-xs">
          <span className="text-[var(--color-ink-faint)] font-medium mr-1">Quick Filters:</span>
          <button
            type="button"
            onClick={() => applyPreset('critical_high')}
            className="inline-flex items-center gap-1 px-2.5 py-1 rounded-full border border-[var(--color-rule-faint)] bg-[var(--color-surface-muted)] text-[var(--color-danger-text)] hover:bg-[var(--color-danger-soft)] transition-colors font-medium"
          >
            <ShieldAlert className="h-3 w-3" /> Critical &amp; High
          </button>
          <button
            type="button"
            onClick={() => applyPreset('security')}
            className="inline-flex items-center gap-1 px-2.5 py-1 rounded-full border border-[var(--color-rule-faint)] bg-[var(--color-surface-muted)] text-[var(--color-accent-strong)] hover:bg-[var(--color-accent-soft)] transition-colors font-medium"
          >
            <Zap className="h-3 w-3" /> Pentest &amp; OWASP
          </button>
          <button
            type="button"
            onClick={() => applyPreset('secrets')}
            className="inline-flex items-center gap-1 px-2.5 py-1 rounded-full border border-[var(--color-rule-faint)] bg-[var(--color-surface-muted)] text-[var(--color-warning-text)] hover:bg-[var(--color-warning-soft)] transition-colors font-medium"
          >
            <KeyRound className="h-3 w-3" /> Secrets &amp; Keys
          </button>
        </div>
      </div>

      {/* Advanced Query Builder Sheet */}
      <Sheet open={advancedOpen} onOpenChange={setAdvancedOpen}>
        <SheetContent side="right" className="w-full max-w-[460px] sm:max-w-[460px] overflow-y-auto" aria-label="Global search query builder" id="search-advanced">
          <SheetHeader>
            <SheetTitle>Advanced Query Builder</SheetTitle>
            <SheetDescription>Construct compound filters across workspaces, severity levels, rule IDs, and analyzers.</SheetDescription>
          </SheetHeader>
          <div className="mt-4">
            <QueryBuilder
              group={queryGroup}
              onChange={setQueryGroup}
              onApply={(f) => {
                setSeverities(new Set(f.severity ? f.severity.split(',') : []));
                setAnalyzer(f.analyzer);
                setQuery(f.q);
                setWorkspace(f.path);
                setAdvancedOpen(false);
              }}
              facetCounts={severityCounts ? { severity: severityCounts } : undefined}
              analyzers={[...ANALYZERS]}
            />
          </div>
        </SheetContent>
      </Sheet>

      {/* Main Search Layout */}
      <div className="search-layout grid grid-cols-1 md:grid-cols-[260px_1fr] gap-6 items-start">
        {/* Desktop Sidebar Facets */}
        <aside className="search-sidebar desktop-only hidden md:block p-4 rounded-[var(--radius-card)] border border-[var(--color-rule-faint)] bg-[var(--color-surface)] shadow-xs">
          {facets}
        </aside>

        {/* Mobile Filter Drawer */}
        {facetsOpen && (
          <div
            className="filter-drawer-backdrop fixed inset-0 z-50 bg-black/50 backdrop-blur-xs md:hidden"
            role="presentation"
            onMouseDown={(e) => {
              if (e.target === e.currentTarget) setFacetsOpen(false);
            }}
          >
            <aside role="dialog" aria-modal="true" aria-label="Filters" className="filter-drawer absolute right-0 top-0 bottom-0 w-full max-w-xs bg-[var(--color-surface)] p-5 overflow-y-auto shadow-xl space-y-4">
              <div className="flex items-center justify-between pb-3 border-b border-[var(--color-rule-faint)]">
                <span className="font-semibold text-sm text-[var(--color-ink)]">Filter Results</span>
                <Button variant="ghost" size="sm" onClick={() => setFacetsOpen(false)}>
                  Close
                </Button>
              </div>
              {facets}
            </aside>
          </div>
        )}

        {/* Results Area */}
        <div className="search-main space-y-4 min-w-0">
          {/* Result toolbar: filters left, the single grounded count right. The
              matched total renders exactly once on this screen — here. */}
          <div className="toolbar-row">
            <div className="toolbar-filters">
              {total > 0 && (
                <button
                  type="button"
                  className="chip"
                  aria-pressed={grouped}
                  title="Collapse consecutive results that share a rule and file"
                  onClick={() => setGrouping(!grouped)}
                >
                  Group by rule + file
                </button>
              )}
              <div className="segmented inline-flex items-center rounded-[var(--radius-button)] border border-[var(--color-rule)] bg-[var(--color-surface-muted)] p-0.5" role="group" aria-label="View mode">
                <button
                  type="button"
                  onClick={() => setViewMode('table')}
                  aria-pressed={viewMode === 'table'}
                  className={`p-1 rounded-[calc(var(--radius-button)-2px)] transition-colors ${
                    viewMode === 'table' ? 'bg-[var(--color-surface)] text-[var(--color-ink)] shadow-xs' : 'text-[var(--color-ink-faint)] hover:text-[var(--color-ink)]'
                  }`}
                  title="Dense table view"
                >
                  <LayoutList className="h-3.5 w-3.5" />
                </button>
                <button
                  type="button"
                  onClick={() => setViewMode('cards')}
                  aria-pressed={viewMode === 'cards'}
                  className={`p-1 rounded-[calc(var(--radius-button)-2px)] transition-colors ${
                    viewMode === 'cards' ? 'bg-[var(--color-surface)] text-[var(--color-ink)] shadow-xs' : 'text-[var(--color-ink-faint)] hover:text-[var(--color-ink)]'
                  }`}
                  title="Card list view"
                >
                  <LayoutGrid className="h-3.5 w-3.5" />
                </button>
              </div>

              {/* Column picker */}
              {viewMode === 'table' && (
                <details className="search-columns relative inline-block text-xs">
                  <summary className="search-columns-toggle flex items-center gap-1 px-2.5 py-1 rounded-[var(--radius-button)] border border-[var(--color-rule-faint)] bg-[var(--color-surface)] cursor-pointer text-[var(--color-ink-soft)] hover:text-[var(--color-ink)]">
                    <ChevronDown className="h-3.5 w-3.5" aria-hidden="true" />
                    <span>Columns</span>
                  </summary>
                  <div className="search-columns-panel absolute right-0 mt-1.5 w-44 rounded-[var(--radius-md)] border border-[var(--color-rule)] bg-[var(--color-surface)] p-2 shadow-lg z-20 space-y-1">
                    {SEARCH_COLUMNS.map(([key, label]) => (
                      <label key={key} className="search-column-toggle flex items-center gap-2 px-1.5 py-1 rounded text-xs text-[var(--color-ink-soft)] hover:bg-[var(--color-surface-muted)] cursor-pointer">
                        <input
                          type="checkbox"
                          checked={visibleCols[key as keyof typeof visibleCols]}
                          onChange={() =>
                            setVisibleCols((v) => ({
                              ...v,
                              [key]: !v[key as keyof typeof visibleCols],
                            }))
                          }
                          className="h-3.5 w-3.5 rounded border-[var(--color-rule-strong)] text-[var(--color-accent)]"
                        />
                        <span>{label}</span>
                      </label>
                    ))}
                  </div>
                </details>
              )}
            </div>

            <span className="toolbar-meta tabular-nums" aria-live="polite">
              {state.loading ? 'Searching…' : total > 0 ? `Showing ${first + 1}–${Math.min(first + items.length, total)} of ${total.toLocaleString()}` : 'No matches'}
            </span>
          </div>

          {/* Results List / Table */}
          {state.loading ? (
            <SkeletonTable rows={8} cols={4} />
          ) : state.error ? (
            <ErrorPanel error={state.error} retry={state.reload} />
          ) : total === 0 ? (
            workspacesList.data && workspacesList.data.length === 0 && activeFiltersCount === 0 ? (
              // Cold-start dead end: zero workspaces means zero scans, so the
              // "no matches" copy would mislead. The add-workspace dialog is
              // owned by App (no onAdd callback reaches this page), so route
              // to Workspaces where that flow lives.
              <Empty
                title="Nothing to search yet"
                icon={<MagnifierIcon />}
                action={
                  <Button variant="outline" size="sm" onClick={() => go({ page: 'workspaces' })} className="gap-1.5">
                    Add workspace
                  </Button>
                }
              >
                Add a workspace and run your first scan — findings from every scan land here.
              </Empty>
            ) : (
              <Empty title="No matching findings" icon={<MagnifierIcon />}>
                No findings match your current query or filters. Try clearing some filters or running a fresh scan across your workspaces.
                {activeFiltersCount > 0 && (
                  <Button variant="outline" size="sm" onClick={clearAll} className="mt-4 gap-1.5">
                    <RotateCcw className="h-3.5 w-3.5" /> Reset all filters
                  </Button>
                )}
              </Empty>
            )
          ) : viewMode === 'cards' ? (
            /* Cards View — title leads, message de-duplicated, one quiet action. */
            <div className="grid gap-2.5">
              {items.map((finding: SearchedFinding) => {
                const meta = analyzerMeta(finding.analyzer_id);
                const title = findingDisplayTitle(finding);
                const message = stripMessagePrefix(finding);
                return (
                  <Card
                    key={`${finding.scan_id}:${finding.id}`}
                    className="p-3.5 hover:border-[var(--color-rule)] transition-all cursor-pointer group"
                    role="button"
                    tabIndex={0}
                    aria-label={findingActionLabel(finding)}
                    onClick={() => setSelectedFinding(finding)}
                    onKeyDown={(event) => openFromKeyboard(event, finding)}
                  >
                    <div className="flex items-start justify-between gap-3">
                      <div className="space-y-1.5 min-w-0 flex-1">
                        <div className="flex flex-wrap items-center gap-2">
                          <span className={`severity ${finding.severity} text-[10px] uppercase font-mono px-2 py-0.5 rounded font-semibold`}>
                            {finding.severity}
                          </span>
                          <span className="tag">
                            <span title={findingLocation(finding)}>{shortFindingLocation(finding)}</span>
                          </span>
                          {ruleTagFor(finding, title)}
                          <span className="tag">
                            <span>{meta?.displayName ?? analyzerName(finding.analyzer_id)}</span>
                          </span>
                        </div>
                        <strong className="block text-sm font-semibold text-[var(--color-ink)] truncate" title={message}>
                          {title}
                        </strong>
                        {message && message !== title && (
                          <p className="text-xs text-[var(--color-ink-soft)] leading-relaxed line-clamp-2">{message}</p>
                        )}
                        {finding.remediation && (
                          <p className="text-[11px] text-[var(--color-ink-soft)] bg-[var(--color-surface-muted)] p-2 rounded-[var(--radius-sm)] font-mono truncate">
                            <span className="font-semibold text-[var(--color-ink)]">Fix: </span>
                            {finding.remediation}
                          </p>
                        )}
                      </div>

                      <div className="flex flex-col items-end gap-2 shrink-0">{openLink(finding)}</div>
                    </div>
                  </Card>
                );
              })}
            </div>
          ) : (
            /* Table View — dense single-line rows; consecutive same rule+file
               results collapse into a group header with deduped occurrence lines. */
            <div className="table-wrap overflow-x-auto rounded-[var(--radius-md)] border border-[var(--color-rule-faint)] bg-[var(--color-surface)]">
              <table className="search-results table-dense w-full text-left">
                <caption className="sr-only">Global search results</caption>
                <colgroup>
                  {visibleCols.finding && <col />}
                  {visibleCols.location && <col className="search-col-location" />}
                  {visibleCols.severity && <col className="search-col-severity" />}
                  {visibleCols.actions && <col className="search-col-actions" />}
                </colgroup>
                <thead>
                  <tr>
                    {visibleCols.finding && <th scope="col">Finding</th>}
                    {visibleCols.location && <th scope="col">Location</th>}
                    {visibleCols.severity && <th scope="col" className="search-cell-severity">Severity</th>}
                    {visibleCols.actions && (
                      <th scope="col" className="search-cell-action">
                        <span className="sr-only">Actions</span>
                      </th>
                    )}
                  </tr>
                </thead>
                <tbody>
                  {rows.map((row) => {
                    if (row.kind === 'single') {
                      const finding = row.finding;
                      const title = findingDisplayTitle(finding);
                      return (
                        <tr
                          key={`${finding.scan_id}:${finding.id}`}
                          className="search-row"
                          tabIndex={0}
                          aria-label={findingActionLabel(finding)}
                          title={stripMessagePrefix(finding)}
                          onClick={() => setSelectedFinding(finding)}
                          onKeyDown={(event) => openFromKeyboard(event, finding)}
                        >
                          {visibleCols.finding && (
                            <td className="search-cell-finding">
                              <span className="search-finding-line">
                                <span className="search-finding-title">{title}</span>
                                {ruleTagFor(finding, title)}
                              </span>
                            </td>
                          )}
                          {visibleCols.location && (
                            <td className="search-cell-location">
                              <span className="tag">
                                <span title={findingLocation(finding)}>{shortFindingLocation(finding)}</span>
                              </span>
                            </td>
                          )}
                          {visibleCols.severity && (
                            <td className="search-cell-severity">
                              <span className={`severity ${finding.severity} text-[10px] uppercase font-mono px-2 py-0.5 rounded font-semibold`}>
                                {finding.severity}
                              </span>
                            </td>
                          )}
                          {visibleCols.actions && <td className="search-cell-action">{openLink(finding)}</td>}
                        </tr>
                      );
                    }
                    const head = row.occurrences[0];
                    const title = findingDisplayTitle(head);
                    // Occurrences of one rule in one file differ only by line; show
                    // each line once, in order, instead of N near-identical rows.
                    const seenLines = new Set<string>();
                    const lines = row.occurrences.filter((f) => {
                      const label = f.start_line ? `Line ${f.start_line}` : findingLocation(f);
                      if (seenLines.has(label)) return false;
                      seenLines.add(label);
                      return true;
                    });
                    return (
                      <Fragment key={`group:${row.key}:${head.scan_id}:${head.id}`}>
                        <tr
                          className="search-group-row"
                          tabIndex={0}
                          aria-label={`${title} — ${row.occurrences.length} occurrences in ${head.relative_path ?? 'project'} — open quick look`}
                          onClick={() => setSelectedFinding(head)}
                          onKeyDown={(event) => openFromKeyboard(event, head)}
                        >
                          <td colSpan={visibleColCount}>
                            <span className="search-group-line">
                              <span className={`search-group-dot sev-${head.severity}`} aria-hidden="true" />
                              <span className="search-group-title">{title}</span>
                              {ruleTagFor(head, title)}
                              <span className="tag">
                                <span title={head.relative_path ?? undefined}>{head.relative_path ?? 'Project-level'}</span>
                              </span>
                              <span className="search-group-count">
                                {row.occurrences.length} occurrence{row.occurrences.length === 1 ? '' : 's'}
                              </span>
                              {openLink(head)}
                            </span>
                          </td>
                        </tr>
                        {lines.map((finding) => (
                          <tr
                            key={`${finding.scan_id}:${finding.id}`}
                            className="search-occurrence-row"
                            tabIndex={0}
                            aria-label={findingActionLabel(finding)}
                            title={stripMessagePrefix(finding)}
                            onClick={() => setSelectedFinding(finding)}
                            onKeyDown={(event) => openFromKeyboard(event, finding)}
                          >
                            <td colSpan={visibleColCount}>
                              <span className="search-occurrence">{finding.start_line ? `Line ${finding.start_line}` : findingLocation(finding)}</span>
                            </td>
                          </tr>
                        ))}
                      </Fragment>
                    );
                  })}
                </tbody>
              </table>
            </div>
          )}

          {/* Pagination — the window range lives in the toolbar meta; the nav
              carries only movement, so the matched total is never rendered twice. */}
          {total > 0 && (
            <nav className="findings-pagination flex flex-wrap items-center justify-end gap-3 pt-3 border-t border-[var(--color-rule-faint)]" aria-label="Search result pagination">
              <div className="flex items-center gap-2">
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => setPage(page - 1)}
                  disabled={page <= 1}
                  className="h-8 text-xs px-3"
                >
                  Previous
                </Button>
                <output aria-live="polite" className="text-xs font-semibold px-2 tabular-nums">
                  Page {page}
                </output>
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => setPage(page + 1)}
                  disabled={!state.data?.has_next}
                  className="h-8 text-xs px-3"
                >
                  Next
                </Button>
              </div>
            </nav>
          )}
        </div>
      </div>

      {/* Quick Finding Detail Drawer */}
      <Sheet open={Boolean(selectedFinding)} onOpenChange={(open) => !open && setSelectedFinding(null)}>
        <SheetContent side="right" className="w-full sm:max-w-md overflow-y-auto space-y-4" aria-label="Finding detail preview">
          {selectedFinding && (
            <>
              <SheetHeader>
                <div className="flex items-center gap-2">
                  <span className={`severity ${selectedFinding.severity} text-[10px] uppercase font-mono px-2 py-0.5 rounded font-semibold`}>
                    {selectedFinding.severity}
                  </span>
                  {selectedFinding.rule_id && selectedFinding.rule_id !== findingDisplayTitle(selectedFinding) && (
                    <Badge variant="outline" className="text-[10px] font-mono">
                      {selectedFinding.rule_id}
                    </Badge>
                  )}
                </div>
                <SheetTitle className="text-base mt-2">{findingDisplayTitle(selectedFinding)}</SheetTitle>
                <SheetDescription className="font-mono text-xs text-[var(--color-ink-soft)]">
                  {findingLocation(selectedFinding)}
                </SheetDescription>
              </SheetHeader>

              <div className="space-y-3 pt-2">
                <div>
                  <h4 className="text-xs font-semibold uppercase tracking-wider text-[var(--color-ink-faint)]">Diagnostic Message</h4>
                  <p className="mt-1 text-xs text-[var(--color-ink)] leading-relaxed bg-[var(--color-surface-muted)] p-3 rounded-[var(--radius-sm)]">
                    {stripMessagePrefix(selectedFinding)}
                  </p>
                </div>

                {selectedFinding.remediation && (
                  <div>
                    <h4 className="text-xs font-semibold uppercase tracking-wider text-[var(--color-ink-faint)]">Remediation Guidance</h4>
                    <p className="mt-1 text-xs text-[var(--color-accent-strong)] leading-relaxed bg-[var(--color-accent-soft)] p-3 rounded-[var(--radius-sm)] font-mono">
                      {selectedFinding.remediation}
                    </p>
                  </div>
                )}

                <div className="pt-2 flex flex-col gap-2">
                  <Button
                    onClick={() => {
                      const sid = selectedFinding.scan_id;
                      setSelectedFinding(null);
                      go({ page: 'scan', id: sid });
                    }}
                    className="w-full gap-2 bg-[var(--color-accent)] text-[var(--color-accent-ink)]"
                  >
                    <ExternalLink className="h-4 w-4" /> Open Full Scan Report
                  </Button>
                </div>
              </div>
            </>
          )}
        </SheetContent>
      </Sheet>
    </div>
  );
}
