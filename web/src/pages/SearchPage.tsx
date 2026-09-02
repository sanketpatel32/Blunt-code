import { useEffect, useMemo, useState } from 'react';
import { api } from '../api';
import type { SearchedFinding, Workspace } from '../types';
import type { Route } from '../lib/router';
import { analyzerName, findingLocation } from '../lib/format';
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
  ChevronRight,
  ShieldAlert,
  Zap,
  KeyRound,
  LayoutList,
  LayoutGrid,
  Sparkles,
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

const STATUSES = ['new', 'persistent'] as const;

const SEARCH_COLUMNS = [
  ['severity', 'Severity'],
  ['finding', 'Finding & Rule'],
  ['location', 'Location'],
  ['actions', 'Actions'],
] as const;

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
  const [status, setStatus] = useState(() => new URLSearchParams(window.location.search).get('status') ?? '');
  const [workspace, setWorkspace] = useState(() => new URLSearchParams(window.location.search).get('workspace') ?? '');
  const [pathPrefix, setPathPrefix] = useState(() => new URLSearchParams(window.location.search).get('path') ?? '');
  const [visibleCols, setVisibleCols] = useState({ severity: true, finding: true, location: true, actions: true });
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
      status: new URLSearchParams(window.location.search).get('status') ?? '',
      q: new URLSearchParams(window.location.search).get('q') ?? '',
    } as FindingFilter)
  );

  const debouncedQuery = useDebouncedValue(query, query ? SEARCH_DEBOUNCE_MS : 0);
  const workspacesList = useLoad(api.workspaces, []);

  const params = useMemo(() => {
    const value: Record<string, string> = { page: String(page), page_size: String(pageSize) };
    if (debouncedQuery) value.q = debouncedQuery;
    if (analyzer) value.analyzer = analyzer;
    if (severities.size) value.severity = [...severities].join(',');
    if (status) value.status = status;
    if (workspace) value.workspace = workspace;
    if (pathPrefix) value.path = pathPrefix;
    return value;
  }, [debouncedQuery, severities, analyzer, status, workspace, pathPrefix, page, pageSize]);

  useEffect(() => {
    setPage(1);
  }, [debouncedQuery, severities, analyzer, status, workspace, pathPrefix]);

  // URL sync
  useEffect(() => {
    const sp = new URLSearchParams();
    if (debouncedQuery) sp.set('q', debouncedQuery);
    if (analyzer) sp.set('analyzer', analyzer);
    if (severities.size) sp.set('severity', [...severities].join(','));
    if (status) sp.set('status', status);
    if (workspace) sp.set('workspace', workspace);
    if (pathPrefix) sp.set('path', pathPrefix);
    if (page !== 1) sp.set('page', String(page));
    const qs = sp.toString();
    const cur = window.location.search.replace(/^\?/, '');
    if (qs === cur) return;
    const next = qs ? `${window.location.pathname}?${qs}${window.location.hash}` : `${window.location.pathname}${window.location.hash}`;
    window.history.replaceState(null, '', next);
  }, [debouncedQuery, analyzer, severities, status, workspace, pathPrefix, page]);

  useEffect(() => {
    const onPop = () => {
      const sp = new URLSearchParams(window.location.search);
      setQuery(sp.get('q') ?? '');
      setAnalyzer(sp.get('analyzer') ?? '');
      setStatus(sp.get('status') ?? '');
      setWorkspace(sp.get('workspace') ?? '');
      setPathPrefix(sp.get('path') ?? '');
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
          status: sp.get('status') ?? '',
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
        status,
        q: query,
      } as FindingFilter)
    );
  }, [severities, analyzer, status, workspace, query]);

  const state = useLoad(() => api.searchFindings(params), [params.q, params.severity, params.analyzer, params.page, params.page_size, params.status, params.workspace, params.path]);
  const hiddenColumnCount = SEARCH_COLUMNS.filter(([key]) => !visibleCols[key as keyof typeof visibleCols]).length;
  const items = state.data?.items ?? [];
  const total = state.data?.total ?? 0;
  const actualPageSize = state.data?.page_size ?? pageSize;
  const first = (page - 1) * actualPageSize;
  const saved = useSavedSearches();

  const toggleSeverity = (severity: string) =>
    setSeverities((current) => {
      const next = new Set(current);
      if (next.has(severity)) next.delete(severity);
      else next.add(severity);
      return next;
    });

  // Severity counts from current result set
  const severityCounts = useMemo(() => {
    const c: Record<string, number> = { critical: 0, high: 0, medium: 0, low: 0, info: 0 };
    for (const f of items) c[f.severity] = (c[f.severity] ?? 0) + 1;
    return c;
  }, [items]);

  const activeFiltersCount = useMemo(() => {
    let count = 0;
    if (query.trim()) count++;
    if (severities.size > 0) count += severities.size;
    if (analyzer) count++;
    if (status) count++;
    if (workspace) count++;
    if (pathPrefix) count++;
    return count;
  }, [query, severities, analyzer, status, workspace, pathPrefix]);

  const clearAll = () => {
    setQuery('');
    setSeverities(new Set());
    setAnalyzer('');
    setStatus('');
    setWorkspace('');
    setPathPrefix('');
    setPage(1);
  };

  const applyPreset = (preset: 'critical_high' | 'security' | 'secrets' | 'new_only') => {
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
      case 'new_only':
        setStatus('new');
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
                <Badge variant={isSelected ? 'outline' : 'secondary'} className="text-[10px] tabular-nums px-1.5 py-0">
                  {severityCounts[sev] ?? 0}
                </Badge>
              </button>
            );
          })}
        </fieldset>
      </div>

      {/* Workspaces */}
      <div className="facet-section">
        <p className="facet-title text-xs font-semibold uppercase tracking-wider text-[var(--color-ink-faint)]">Workspace Scope</p>
        <div className="mt-2 space-y-2">
          {workspacesList.data && workspacesList.data.length > 0 ? (
            <Select value={workspace} onValueChange={(val) => setWorkspace(val === 'all' ? '' : val)}>
              <SelectTrigger className="w-full text-xs h-8">
                <SelectValue placeholder="All Workspaces" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">All Workspaces</SelectItem>
                {workspacesList.data.map((w: Workspace) => (
                  <SelectItem key={w.id} value={w.id}>
                    {w.name || w.id}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          ) : (
            <input
              value={workspace}
              onChange={(e) => setWorkspace(e.target.value)}
              placeholder="Filter by workspace ID…"
              aria-label="Filter by workspace"
              className="w-full rounded-[var(--radius-button)] border border-[var(--color-rule-faint)] bg-[var(--color-surface)] px-2.5 py-1.5 text-xs text-[var(--color-ink)] placeholder:text-[var(--color-ink-faint)] focus:border-[var(--color-accent)] focus:outline-none"
            />
          )}
        </div>
      </div>

      {/* Analyzer Engines */}
      <div className="facet-section">
        <div className="flex items-center justify-between">
          <p className="facet-title text-xs font-semibold uppercase tracking-wider text-[var(--color-ink-faint)]">Engine / Analyzer</p>
          {analyzer && (
            <button type="button" onClick={() => setAnalyzer('')} className="text-[11px] text-[var(--color-accent-strong)] hover:underline">
              All
            </button>
          )}
        </div>
        <fieldset className="chip-group mt-2 flex flex-wrap gap-1.5" aria-label="Filter by analyzer">
          <button
            type="button"
            className={`chip text-[11px] px-2.5 py-1 rounded-full border transition-all ${
              analyzer === ''
                ? 'bg-[var(--color-accent)] text-[var(--color-accent-ink)] border-[var(--color-accent)] font-semibold'
                : 'border-[var(--color-rule-faint)] bg-[var(--color-surface-muted)] text-[var(--color-ink-soft)] hover:text-[var(--color-ink)]'
            }`}
            aria-pressed={analyzer === ''}
            onClick={() => setAnalyzer('')}
          >
            All engines
          </button>
          {ANALYZERS.map((id) => {
            const meta = analyzerMeta(id);
            const isSelected = analyzer === id;
            return (
              <button
                key={id}
                type="button"
                className={`chip text-[11px] px-2.5 py-1 rounded-full border transition-all ${
                  isSelected
                    ? 'bg-[var(--color-accent)] text-[var(--color-accent-ink)] border-[var(--color-accent)] font-semibold'
                    : 'border-[var(--color-rule-faint)] bg-[var(--color-surface-muted)] text-[var(--color-ink-soft)] hover:text-[var(--color-ink)]'
                }`}
                aria-pressed={isSelected}
                onClick={() => setAnalyzer(id)}
              >
                {meta?.displayName ?? analyzerName(id)}
              </button>
            );
          })}
        </fieldset>
      </div>

      {/* Finding Status */}
      <div className="facet-section">
        <p className="facet-title text-xs font-semibold uppercase tracking-wider text-[var(--color-ink-faint)]">Finding Status</p>
        <fieldset className="chip-group mt-2 flex flex-wrap gap-1.5" aria-label="Filter by status">
          <button
            type="button"
            className={`chip text-[11px] px-2.5 py-1 rounded-full border transition-all ${
              status === ''
                ? 'bg-[var(--color-surface-subtle)] border-[var(--color-rule-strong)] text-[var(--color-ink)] font-semibold'
                : 'border-[var(--color-rule-faint)] bg-[var(--color-surface-muted)] text-[var(--color-ink-soft)]'
            }`}
            aria-pressed={status === ''}
            onClick={() => setStatus('')}
          >
            All
          </button>
          {STATUSES.map((s) => (
            <button
              key={s}
              type="button"
              className={`chip text-[11px] px-2.5 py-1 rounded-full border capitalize transition-all ${
                status === s
                  ? 'bg-[var(--color-surface-subtle)] border-[var(--color-rule-strong)] text-[var(--color-ink)] font-semibold'
                  : 'border-[var(--color-rule-faint)] bg-[var(--color-surface-muted)] text-[var(--color-ink-soft)]'
              }`}
              aria-pressed={status === s}
              onClick={() => setStatus(s)}
            >
              {s}
            </button>
          ))}
        </fieldset>
      </div>

      {/* Path Prefix Filter */}
      <div className="facet-section">
        <p className="facet-title text-xs font-semibold uppercase tracking-wider text-[var(--color-ink-faint)]">File Path Prefix</p>
        <input
          value={pathPrefix}
          onChange={(e) => setPathPrefix(e.target.value)}
          placeholder="e.g. src/auth or internal/api"
          aria-label="Filter by file path"
          className="mt-2 w-full rounded-[var(--radius-button)] border border-[var(--color-rule-faint)] bg-[var(--color-surface)] px-2.5 py-1.5 text-xs text-[var(--color-ink)] placeholder:text-[var(--color-ink-faint)] focus:border-[var(--color-accent)] focus:outline-none"
        />
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
            status,
            category: '',
            rule: '',
            path: workspace,
          } as FindingFilter
        }
        onLoad={(f) => {
          setSeverities(new Set(f.severity ? f.severity.split(',') : []));
          setAnalyzer(f.analyzer);
          setQuery(f.q);
          setStatus(f.status);
          setWorkspace(f.path);
        }}
      />
    </div>
  );

  return (
    <div className="page search-page space-y-3">
      {/* Compact Header */}
      <PageHeader
        eyebrow="Findings Search"
        title="Find findings everywhere"
        badge={total > 0 ? <Badge variant="secondary" className="text-xs font-mono tabular-nums">{total.toLocaleString()} matches</Badge> : undefined}
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
            className="inline-flex items-center gap-1 px-2.5 py-1 rounded-full border border-[var(--color-rule-faint)] bg-[var(--color-surface-muted)] text-[var(--color-danger)] hover:bg-[var(--color-danger-soft)] transition-colors font-medium"
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
            className="inline-flex items-center gap-1 px-2.5 py-1 rounded-full border border-[var(--color-rule-faint)] bg-[var(--color-surface-muted)] text-[var(--color-warning)] hover:bg-[var(--color-warning-soft)] transition-colors font-medium"
          >
            <KeyRound className="h-3 w-3" /> Secrets &amp; Keys
          </button>
          <button
            type="button"
            onClick={() => applyPreset('new_only')}
            className="inline-flex items-center gap-1 px-2.5 py-1 rounded-full border border-[var(--color-rule-faint)] bg-[var(--color-surface-muted)] text-[var(--color-ink-soft)] hover:bg-[var(--color-surface-subtle)] transition-colors font-medium"
          >
            <Sparkles className="h-3 w-3" /> New in Last Scan
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
                setStatus(f.status);
                setWorkspace(f.path);
                setAdvancedOpen(false);
              }}
              facetCounts={{ severity: severityCounts as Record<string, number> }}
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
          {/* Controls Bar */}
          <div className="flex flex-wrap items-center justify-between gap-3 pb-2 border-b border-[var(--color-rule-faint)]">
            <div className="flex items-center gap-2">
              <span className="text-xs font-semibold text-[var(--color-ink)] tabular-nums">
                {total} {total === 1 ? 'finding match' : 'findings matched'}
              </span>
              {total > 0 && (
                <span className="text-xs text-[var(--color-ink-faint)]">
                  (Showing {first + 1}–{Math.min(first + items.length, total)})
                </span>
              )}
            </div>

            <div className="flex items-center gap-2">
              {/* View Switcher */}
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
                    {hiddenColumnCount > 0 && <small className="text-[var(--color-warning)]">({hiddenColumnCount} hidden)</small>}
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
          </div>

          {/* Results List / Table */}
          {state.loading ? (
            <SkeletonTable rows={8} cols={4} />
          ) : state.error ? (
            <ErrorPanel error={state.error} retry={state.reload} />
          ) : total === 0 ? (
            <Empty title="No matching findings" icon={<MagnifierIcon />}>
              No findings match your current query or filters. Try clearing some filters or running a fresh scan across your workspaces.
              {activeFiltersCount > 0 && (
                <Button variant="outline" size="sm" onClick={clearAll} className="mt-4 gap-1.5">
                  <RotateCcw className="h-3.5 w-3.5" /> Reset all filters
                </Button>
              )}
            </Empty>
          ) : viewMode === 'cards' ? (
            /* Cards View */
            <div className="grid gap-2.5">
              {items.map((finding: SearchedFinding) => {
                const meta = analyzerMeta(finding.analyzer_id);
                return (
                  <Card
                    key={`${finding.scan_id}:${finding.id}`}
                    className="p-3.5 hover:border-[var(--color-rule)] transition-all cursor-pointer group"
                    onClick={() => setSelectedFinding(finding)}
                  >
                    <div className="flex items-start justify-between gap-3">
                      <div className="space-y-1.5 min-w-0 flex-1">
                        <div className="flex flex-wrap items-center gap-2">
                          <span className={`severity ${finding.severity} text-[10px] uppercase font-mono px-2 py-0.5 rounded font-semibold`}>
                            {finding.severity}
                          </span>
                          <span className="font-mono text-xs font-semibold text-[var(--color-ink)] truncate">
                            {findingLocation(finding)}
                          </span>
                          {finding.rule_id && (
                            <Badge variant="outline" className="text-[10px] font-mono">
                              {finding.rule_id}
                            </Badge>
                          )}
                          <Badge variant="secondary" className="text-[10px]">
                            {meta?.displayName ?? analyzerName(finding.analyzer_id)}
                          </Badge>
                        </div>
                        <p className="text-xs text-[var(--color-ink)] leading-relaxed">
                          {finding.title && <strong className="block text-sm font-semibold">{finding.title}</strong>}
                          {finding.message}
                        </p>
                        {finding.remediation && (
                          <p className="text-[11px] text-[var(--color-ink-soft)] bg-[var(--color-surface-muted)] p-2 rounded-[var(--radius-sm)] font-mono truncate">
                            <span className="font-semibold text-[var(--color-ink)]">Fix: </span>
                            {finding.remediation}
                          </p>
                        )}
                      </div>

                      <div className="flex flex-col items-end gap-2 shrink-0">
                        <a
                          href={`/scans/${finding.scan_id}`}
                          onClick={(event) => {
                            event.preventDefault();
                            event.stopPropagation();
                            go({ page: 'scan', id: finding.scan_id });
                          }}
                          className="button primary text-xs h-7 px-2.5 gap-1"
                        >
                          Open report <ChevronRight className="h-3 w-3" />
                        </a>
                      </div>
                    </div>
                  </Card>
                );
              })}
            </div>
          ) : (
            /* Table View */
            <div className="table-wrap overflow-x-auto rounded-[var(--radius-md)] border border-[var(--color-rule-faint)] bg-[var(--color-surface)]">
              <table className="search-results w-full text-left text-xs">
                <caption className="sr-only">Global search results</caption>
                <thead className="sticky top-0 z-[1] bg-[var(--color-surface-muted)] border-b border-[var(--color-rule)]">
                  <tr>
                    {visibleCols.severity && <th scope="col" className="py-2.5 px-3 font-semibold text-[var(--color-ink)]">Severity</th>}
                    {visibleCols.finding && <th scope="col" className="py-2.5 px-3 font-semibold text-[var(--color-ink)]">Finding &amp; Rule</th>}
                    {visibleCols.location && <th scope="col" className="py-2.5 px-3 font-semibold text-[var(--color-ink)]">Location</th>}
                    {visibleCols.actions && (
                      <th scope="col" className="py-2.5 px-3 font-semibold text-[var(--color-ink)] text-right">
                        <span className="sr-only">Actions</span>
                      </th>
                    )}
                  </tr>
                </thead>
                <tbody className="divide-y divide-[var(--color-rule-faint)]">
                  {items.map((finding: SearchedFinding) => (
                    <tr
                      key={`${finding.scan_id}:${finding.id}`}
                      className="hover:bg-[var(--color-surface-muted)] transition-colors cursor-pointer"
                      onClick={() => setSelectedFinding(finding)}
                    >
                      {visibleCols.severity && (
                        <td className="py-2.5 px-3 align-top">
                          <span className={`severity ${finding.severity} text-[10px] uppercase font-mono px-2 py-0.5 rounded font-semibold`}>
                            {finding.severity}
                          </span>
                        </td>
                      )}
                      {visibleCols.finding && (
                        <td className="finding-summary py-2.5 px-3 align-top space-y-1 max-w-md">
                          {finding.title ? <strong className="block font-semibold text-[var(--color-ink)]">{finding.title}</strong> : null}
                          <span className="text-[var(--color-ink)] line-clamp-2">{finding.message}</span>
                          <div className="flex items-center gap-1.5 pt-0.5">
                            {finding.rule_id ? <code className="badge text-[10px]">{finding.rule_id}</code> : null}
                            <span className="text-[10px] text-[var(--color-ink-faint)]">
                              via {analyzerName(finding.analyzer_id)}
                            </span>
                          </div>
                        </td>
                      )}
                      {visibleCols.location && (
                        <td className="py-2.5 px-3 align-top font-mono text-[11px] text-[var(--color-ink-soft)]">
                          <code>{findingLocation(finding)}</code>
                        </td>
                      )}
                      {visibleCols.actions && (
                        <td className="py-2.5 px-3 align-top text-right">
                          <a
                            href={`/scans/${finding.scan_id}`}
                            onClick={(event) => {
                              event.preventDefault();
                              event.stopPropagation();
                              go({ page: 'scan', id: finding.scan_id });
                            }}
                            className="inline-flex items-center gap-1 text-xs font-medium text-[var(--color-accent-strong)] hover:underline whitespace-nowrap"
                          >
                            Open report
                          </a>
                        </td>
                      )}
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}

          {/* Pagination */}
          {total > 0 && (
            <nav className="findings-pagination flex flex-wrap items-center justify-between gap-3 pt-3 border-t border-[var(--color-rule-faint)]" aria-label="Search result pagination">
              <span className="text-xs text-[var(--color-ink-soft)]">
                Showing {total === 0 ? 0 : first + 1}–{Math.min(first + items.length, total)} of {total}
              </span>
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
                <output aria-live="polite" className="text-xs font-semibold px-2">
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
                  <Badge variant="outline" className="text-[10px] font-mono">
                    {selectedFinding.rule_id}
                  </Badge>
                </div>
                <SheetTitle className="text-base mt-2">
                  {selectedFinding.title || selectedFinding.rule_id || 'Finding Details'}
                </SheetTitle>
                <SheetDescription className="font-mono text-xs text-[var(--color-ink-soft)]">
                  {findingLocation(selectedFinding)}
                </SheetDescription>
              </SheetHeader>

              <div className="space-y-3 pt-2">
                <div>
                  <h4 className="text-xs font-semibold uppercase tracking-wider text-[var(--color-ink-faint)]">Diagnostic Message</h4>
                  <p className="mt-1 text-xs text-[var(--color-ink)] leading-relaxed bg-[var(--color-surface-muted)] p-3 rounded-[var(--radius-sm)]">
                    {selectedFinding.message}
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
