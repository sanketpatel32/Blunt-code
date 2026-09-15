import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { FileCode, FileJson, FileText, FileBraces, Braces, Palette, Database, Container, Settings2, ScrollText, Code2, FileSpreadsheet, Search, Plus, X, RotateCcw, Save, FolderCog } from 'lucide-react';
import { api } from '../api';
import type { PathOverride, TreeNode } from '../types';
import type { Route } from '../lib/router';
import type { Notice } from '../lib/notice';
import { message } from '../lib/notice';
import { useLoad } from '../hooks/useLoad';
import { useDebouncedValue } from '../hooks/useDebouncedValue';
import { Empty, ErrorPanel } from '../components/ui';
import { MagnifierIcon } from '../components/icons';
import { SkeletonLines } from '../components/skeletons';
import { WorkspaceContextSidebar } from '../components/WorkspaceContext';
import { PageHeader } from '../components/PageHeader';
import { Button } from '../components/ui/button';
import { PathCopy } from '../components/PathCopy';

interface RuleDraft { uid: number; rule_type: 'include' | 'exclude'; pattern: string; enabled?: boolean }

/** Session-wide identity for rule rows, so keys and edits survive reordering and duplicate patterns. */
let ruleUid = 0;
function nextRuleUid() { ruleUid += 1; return ruleUid; }

const LANG_LABELS: Record<string, string> = {
  python: 'Python', javascript: 'JavaScript', typescript: 'TypeScript', go: 'Go', java: 'Java', kotlin: 'Kotlin', csharp: 'C#', c: 'C', cpp: 'C++', ruby: 'Ruby', php: 'PHP', rust: 'Rust', swift: 'Swift', scala: 'Scala', dart: 'Dart', elixir: 'Elixir', 'objective-c': 'ObjC', vue: 'Vue', svelte: 'Svelte', html: 'HTML', css: 'CSS', scss: 'SCSS', less: 'Less', json: 'JSON', yaml: 'YAML', toml: 'TOML', xml: 'XML', sql: 'SQL', graphql: 'GraphQL', shell: 'Shell', powershell: 'PS', batch: 'Batch', markdown: 'MD', dockerfile: 'Dockerfile', env: 'Env', ini: 'INI', properties: 'Properties', terraform: 'Terraform', text: 'Text', certificate: 'Certificate',
};

/** Client-side mirror of the backend's extension→language table
 *  (internal/discovery/discovery.go): same canonical names, same extensions.
 *  The server now sends `language` on tree nodes; this fallback keeps the rail
 *  and filter honest for responses (or cached payloads) that predate it. */
const EXTENSION_LANGUAGES: Record<string, string> = {
  py: 'python', pyi: 'python',
  ts: 'typescript', tsx: 'typescript', mts: 'typescript', cts: 'typescript',
  js: 'javascript', jsx: 'javascript', mjs: 'javascript', cjs: 'javascript',
  go: 'go', java: 'java',
  kt: 'kotlin', kts: 'kotlin',
  cs: 'csharp',
  c: 'c', h: 'c',
  cpp: 'cpp', hpp: 'cpp', cc: 'cpp',
  rb: 'ruby', php: 'php', rs: 'rust', swift: 'swift', scala: 'scala',
  m: 'objective-c', mm: 'objective-c',
  vue: 'vue', svelte: 'svelte',
  tf: 'terraform', tfvars: 'terraform', hcl: 'terraform',
  css: 'css', scss: 'scss', less: 'less',
  html: 'html', htm: 'html',
  json: 'json', jsonc: 'json',
  yaml: 'yaml', yml: 'yaml',
  toml: 'toml', xml: 'xml', sql: 'sql', graphql: 'graphql',
  sh: 'shell', bash: 'shell', zsh: 'shell',
  ps1: 'powershell', bat: 'batch', cmd: 'batch',
  md: 'markdown', markdown: 'markdown', txt: 'text',
  ini: 'ini', cfg: 'ini', conf: 'ini', properties: 'properties',
  env: 'env',
  pem: 'certificate', key: 'certificate', pub: 'certificate',
};

/** Friendly words for the tree's excluded_reason enum (internal/discovery skip
 *  reasons): why the walk kept this path out of scans. Unmapped reasons fall
 *  back to plain-space humanization, like HistoryPage's SKIP_LABELS. */
const EXCLUDED_LABELS: Record<string, string> = {
  excluded_user: 'your rules',
  excluded_default: 'default rules',
  generated_content: 'generated file',
};

function excludedLabel(reason: string) { return EXCLUDED_LABELS[reason] ?? reason.replaceAll('_', ' '); }

/** Language of a tree node, mirroring discovery's rules: the API's `language`
 *  wins, then the extension, then the basename cases (Dockerfile, .env*,
 *  LICENSE files) whose "extension" is the whole name or absent. Lowercase. */
function resolveLanguage(node: TreeNode): string {
  if (node.language) return node.language.toLowerCase();
  const base = node.name.toLowerCase();
  const dot = base.lastIndexOf('.');
  const ext = dot > 0 ? base.slice(dot + 1) : '';
  if (ext && EXTENSION_LANGUAGES[ext]) return EXTENSION_LANGUAGES[ext];
  if (base === 'dockerfile' || base.startsWith('dockerfile.')) return 'dockerfile';
  if (base === '.env' || base.startsWith('.env.')) return 'env';
  if (/^(?:license|licence|copying|notice)(?:[._-].*)?$/.test(base)) return 'text';
  return '';
}

/** Whether a directory could still hold the language: an unloaded branch counts
 *  as "maybe", so the filter only hides folders whose loaded contents disprove
 *  them — a deep link never lands on an empty tree. */
function subtreeMayHoldLanguage(node: TreeNode, lang: string, children: Record<string, TreeNode[]>): boolean {
  for (const child of children[node.path] ?? node.children ?? []) {
    if (child.type !== 'directory') { if (resolveLanguage(child) === lang) return true; continue; }
    if ((children[child.path] ?? child.children) === undefined) return true;
    if (subtreeMayHoldLanguage(child, lang, children)) return true;
  }
  return false;
}

/** Language-filter visibility for one row: files match by resolved language;
 *  directories stay up while unexplored and, once loaded, only if the language
 *  may live inside them. */
function languageVisible(node: TreeNode, lang: string, children: Record<string, TreeNode[]>): boolean {
  if (node.type !== 'directory') return resolveLanguage(node) === lang;
  return (children[node.path] ?? node.children) === undefined || subtreeMayHoldLanguage(node, lang, children);
}

/** The ?lang= param of a query string, for URL-owned filter state. */
function urlLang(search: string) { try { return new URLSearchParams(search).get('lang') ?? ''; } catch { return ''; } }

function languageIcon(lang?: string) {
  if (!lang) return null;
  const props = { size: 14, className: 'shrink-0 text-[var(--color-ink-faint)]', 'aria-hidden': true } as const;
  switch (lang) {
    case 'python': return <Code2 {...props} />;
    case 'javascript': case 'typescript': return <FileCode {...props} />;
    case 'go': case 'java': case 'kotlin': case 'csharp': case 'c': case 'cpp': case 'rust': case 'swift': case 'scala': case 'dart': case 'elixir': case 'objective-c': return <FileBraces {...props} />;
    case 'ruby': case 'php': return <FileCode {...props} />;
    case 'json': return <FileJson {...props} />;
    case 'yaml': case 'toml': case 'xml': return <FileSpreadsheet {...props} />;
    case 'html': return <Braces {...props} />;
    case 'css': case 'scss': return <Palette {...props} />;
    case 'sql': return <Database {...props} />;
    case 'dockerfile': return <Container {...props} />;
    case 'shell': case 'powershell': case 'batch': return <ScrollText {...props} />;
    case 'markdown': case 'text': return <FileText {...props} />;
    case 'env': return <Settings2 {...props} />;
    default: return <FileCode {...props} />;
  }
}

export function FilesPage({ id, go, notify }: { id: string; go?: (r: Route) => void; notify: (n: Notice) => void }) {
  const workspace = useLoad(() => api.workspace(id), [id]);
  // The address bar owns the language filter (?lang=<lang>): the workspace
  // page's donut drills through with it. The search string is re-read every
  // render — the app re-renders this page on pushState navigation and popstate —
  // and mirrored into state by the effect, so Back/Forward move the filter too.
  const routeSearch = window.location.search;
  const [query, setQuery] = useState('');
  const [lang, setLang] = useState(() => urlLang(routeSearch));
  useEffect(() => { setLang(urlLang(routeSearch)); }, [routeSearch]);
  /** Chip clicks drive state and address bar together: replaceState rewrites
   *  just the lang param, so a cleared filter can't silently re-apply on reload
   *  and a chosen one survives it. */
  const setLangFilter = useCallback((next: string) => {
    setLang(next);
    try {
      const url = new URL(window.location.href);
      if (next) url.searchParams.set('lang', next); else url.searchParams.delete('lang');
      window.history.replaceState({}, '', url);
    } catch { /* the filter still applies; only the address bar misses the rewrite */ }
  }, []);
  /** Typing stays instant while the tree is only re-filtered once input settles; clearing applies immediately. */
  const debouncedQuery = useDebouncedValue(query, query ? 200 : 0);
  const debouncedLang = useDebouncedValue(lang, 0);
  const [nodes, setNodes] = useState<TreeNode[]>([]);
  const [treeError, setTreeError] = useState<string>();
  const [loadingTree, setLoadingTree] = useState(true);
  const [rules, setRules] = useState<{ rules: RuleDraft[] }>({ rules: [] });
  const [overrides, setOverrides] = useState<PathOverride[]>([]);
  /** True while the working copy (rules + overrides) differs from the saved one: gates Save and the "Unsaved changes" hint. */
  const [dirty, setDirty] = useState(false);
  const [treeKey, setTreeKey] = useState(0);
  /** Bumped by Collapse all; FileTree watches it and folds every open folder without dropping loaded children. */
  const [collapseSignal, setCollapseSignal] = useState(0);
  /** Languages actually present in the loaded tree (grows as folders expand); the filter rail only offers these. */
  const [loadedMeta, setLoadedMeta] = useState<{ paths: number; langs: Record<string, number> }>({ paths: 0, langs: {} });
  const searchRef = useRef<HTMLInputElement>(null);
  const loadTree = useCallback(async () => { setLoadingTree(true); try { setNodes(await api.tree(id)); setTreeError(undefined); } catch (e) { setTreeError(message(e)); } finally { setLoadingTree(false); } }, [id]);
  /** The workspace probe gates everything else: a bad id fails once here instead
   *  of fanning out into tree/rules/overrides 404s with stacked error surfaces.
   *  The ref pins this to one boot round per workspace id — an identity change
   *  in a callback prop must never re-run it and drop the FileTree's cache. */
  /** One fetch of the saved selection (rules + path overrides). Boot and Reset
   *  share it, so Reset restores the saved state instead of wiping the working
   *  copy to empty — Reset-then-Save can no longer erase a saved selection. */
  const loadSavedSelection = useCallback(async () => {
    const [savedRules, savedOverrides] = await Promise.all([api.rules(id), api.pathOverrides(id)]);
    setRules({ rules: (savedRules as { rules: Array<Omit<RuleDraft, 'uid'>> }).rules.map((rule) => ({ ...rule, uid: nextRuleUid() })) });
    setOverrides(savedOverrides);
    setDirty(false);
  }, [id]);
  /** Working-copy setters mark the selection dirty; only a successful save or a Reset back to the saved state clears it. */
  const setWorkingOverrides = useCallback((items: PathOverride[]) => { setOverrides(items); setDirty(true); }, []);
  const setWorkingRules = useCallback((items: RuleDraft[]) => { setRules({ rules: items }); setDirty(true); }, []);
  const bootedFor = useRef<string | undefined>(undefined);
  useEffect(() => {
    if (workspace.loading || workspace.error || bootedFor.current === id) return;
    bootedFor.current = id;
    void loadTree();
    void loadSavedSelection().catch((e) => notify({ kind: 'error', text: message(e) }));
  }, [id, loadTree, loadSavedSelection, notify, workspace.loading, workspace.error]);
  /** Empty pattern rows are unfinished drafts, not rules — the server rightly rejects them, so they never leave the client. */
  const save = async () => { try { await api.saveRules(id, rules.rules.filter((rule) => rule.pattern.trim() !== '').map(({ uid: _uid, pattern, ...rule }) => ({ ...rule, pattern: pattern.trim() }))); await api.savePathOverrides(id, overrides); await loadTree(); setTreeKey((value) => value + 1); setDirty(false); notify({ kind: 'info', text: 'File selection saved for this workspace.' }); } catch (e) { notify({ kind: 'error', text: message(e) }); } };
  /** "/" jumps to the search box from anywhere on this page — unless the user is already typing in a field. */
  useEffect(() => {
    function jumpToSearch(event: KeyboardEvent) {
      if (event.key !== '/' || event.ctrlKey || event.metaKey || event.altKey) return;
      const active = document.activeElement;
      const tag = active?.tagName.toLowerCase();
      if (tag === 'input' || tag === 'textarea' || tag === 'select' || (active instanceof HTMLElement && active.isContentEditable)) return;
      event.preventDefault();
      searchRef.current?.focus();
    }
    window.addEventListener('keydown', jumpToSearch);
    return () => window.removeEventListener('keydown', jumpToSearch);
  }, []);
  const langEntries = useMemo(() => Object.entries(loadedMeta.langs).sort((a, b) => b[1] - a[1] || a[0].localeCompare(b[0])), [loadedMeta]);
  const selectedCount = overrides.filter((o) => o.mode === 'include').length;
  const langDistinct = langEntries.length;
  /** Save/Reset need a workspace and a tree behind them; a failed load leaves nothing to save. */
  const selectionReady = !workspace.loading && !workspace.error && !loadingTree && !treeError;

  return (
    <div className="page workspace-page">
      {go && <WorkspaceContextSidebar id={id} current={{ page: 'files', id }} onNavigate={go} />}
      <div className="workspace-page-body">
        <PageHeader
          eyebrow="File selection"
          title={workspace.data?.name ? `${workspace.data.name} — Files` : 'Workspace files'}
          description={
            workspace.data?.root_path ? (
              <span className="flex items-center gap-1.5 font-mono text-xs">
                <span className="workspace-root truncate max-w-md sm:max-w-xl text-[var(--color-ink-faint)]" title={workspace.data.root_path}>{workspace.data.root_path}</span>
                <PathCopy path={workspace.data.root_path} />
              </span>
            ) : 'Include or ignore source files and directories for analysis.'
          }
          actions={
            <div className="files-toolbar flex items-center gap-2">
              {dirty && <span role="status" className="text-xs text-[var(--color-ink-faint)]">Unsaved changes</span>}
              <Button variant="ghost" size="sm" disabled={!selectionReady} onClick={() => { void loadSavedSelection().then(() => notify({ kind: 'info', text: 'Restored the saved selection.' })).catch((e) => notify({ kind: 'error', text: message(e) })); }} className="gap-1.5 text-xs">
                <RotateCcw size={14} aria-hidden />Reset
              </Button>
              <Button variant="default" size="sm" disabled={!selectionReady || !dirty} onClick={save} className="gap-1.5 text-xs">
                <Save size={14} aria-hidden />Save selection
              </Button>
            </div>
          }
        />
    <section className="file-layout"><div className="tree-panel"><div className="tree-panel-head"><div><p className="eyebrow">Source tree</p><h2 className="tree-panel-title">Workspace files</h2></div><span className="tree-count-badge tabular-nums">{nodes.length ? `${nodes.length} top-level` : '—'}</span></div><div className="tree-panel-controls"><label className="search file-search"><span className="sr-only">Search paths</span><span className="file-search-wrap"><Search size={14} className="file-search-icon" aria-hidden /><input ref={searchRef} value={query} onChange={(event) => setQuery(event.target.value)} onKeyDown={(event) => { if (event.key === 'Escape') { setQuery(''); event.currentTarget.blur(); } }} placeholder="src or package.json" className="file-search-input" /><kbd className="kbd-hint">/</kbd></span></label><fieldset className="tree-toolbar chip-group chip-rail" aria-label="Filter by language">
          <button type="button" className="chip" aria-pressed={lang === ''} onClick={() => setLangFilter('')}>All languages</button>
          {langEntries.map(([l, count]) => {
            const label = LANG_LABELS[l] ?? l;
            return <button key={l} type="button" className="chip" aria-pressed={lang === l} onClick={() => setLangFilter(l)}>{label}<small className="chip-count">{count}</small></button>;
          })}
        {lang && <button type="button" className="text-button" onClick={() => setLangFilter('')}>Clear filter</button>}{!loadingTree && !treeError && <button type="button" className="button ghost tree-collapse-btn" onClick={() => setCollapseSignal((value) => value + 1)}>Collapse all</button>}</fieldset><div className="tree-scroll">{workspace.error ? <ErrorPanel error={workspace.error} retry={workspace.reload} /> : loadingTree ? <SkeletonLines lines={6} /> : treeError ? <ErrorPanel error={treeError} retry={loadTree} /> : <FileTree key={treeKey} nodes={nodes} query={debouncedQuery} lang={debouncedLang} workspaceId={id} overrides={overrides} onOverrides={setWorkingOverrides} collapseSignal={collapseSignal} onLoadedMeta={setLoadedMeta} hasScans={Boolean(workspace.data?.last_scan_at)} />}</div><div className="tree-summary-bar"><span className="tabular-nums" title="Saved include rules for this workspace">{selectedCount} saved {selectedCount === 1 ? 'rule' : 'rules'}</span><span aria-hidden>·</span><span className="tabular-nums">{langDistinct} languages</span></div></div></div><RuleEditor rules={rules.rules} setRules={setWorkingRules} /></section>
      </div>
    </div>
  );
}

/** Per-row slice of tree state. The top-level FileTree owns all of it so search can walk every loaded level. */
interface TreeState {
  overrides: PathOverride[];
  onOverrides: (items: PathOverride[]) => void;
  children: Record<string, TreeNode[]>;
  expanded: Set<string>;
  loading: Set<string>;
  failed: Set<string>;
  needle: string;
  langFilter: string;
  matches: Set<string>;
  ancestors: Set<string>;
  toggle: (node: TreeNode) => void;
  retry: (node: TreeNode) => void;
}

function FileTree({ nodes, query, lang, workspaceId, overrides, onOverrides, collapseSignal = 0, onLoadedMeta, hasScans = false }: { nodes: TreeNode[]; query: string; lang?: string; workspaceId: string; overrides: PathOverride[]; onOverrides: (items: PathOverride[]) => void; collapseSignal?: number; onLoadedMeta?: (meta: { paths: number; langs: Record<string, number> }) => void; hasScans?: boolean }) {
  const [expanded, setExpanded] = useState<Set<string>>(new Set());
  const [children, setChildren] = useState<Record<string, TreeNode[]>>({});
  const [loading, setLoading] = useState<Set<string>>(new Set());
  const [failed, setFailed] = useState<Set<string>>(new Set());
  const needle = query.trim().toLowerCase();
  const langFilter = (lang ?? '').trim().toLowerCase();
  // Collapse all folds every open folder on demand without unmounting (so
  // already-fetched children stay cached and re-expand instantly).
  useEffect(() => { if (collapseSignal > 0) setExpanded(new Set()); }, [collapseSignal]);
  const loadChildren = useCallback(async (path: string) => {
    setLoading((old) => new Set(old).add(path));
    setFailed((old) => { const next = new Set(old); next.delete(path); return next; });
    try {
      const loaded = await api.tree(workspaceId, path);
      setChildren((old) => ({ ...old, [path]: loaded }));
    } catch {
      setFailed((old) => new Set(old).add(path)); // Surfaced inline on the row with a retry instead of silently dropping the folder.
    } finally {
      setLoading((old) => { const next = new Set(old); next.delete(path); return next; });
    }
  }, [workspaceId]);
  function toggle(node: TreeNode) {
    if (node.type !== 'directory') return;
    setExpanded((old) => { const next = new Set(old); if (next.has(node.path)) next.delete(node.path); else next.add(node.path); return next; });
    if (!expanded.has(node.path) && children[node.path] === undefined) void loadChildren(node.path);
  }
  const retry = useCallback((node: TreeNode) => { setExpanded((old) => new Set(old).add(node.path)); void loadChildren(node.path); }, [loadChildren]);
  /** One recursive walk over every loaded node: `matches` hits the query, `ancestors` lead to a hit. */
  const { matches, ancestors } = useMemo(() => {
    const result = { matches: new Set<string>(), ancestors: new Set<string>() };
    if (!needle) return result;
    const walk = (level: TreeNode[]): boolean => {
      let hit = false;
      for (const node of level) {
        const descendant = walk(children[node.path] ?? []);
        if (node.path.toLowerCase().includes(needle)) { result.matches.add(node.path); hit = true; }
        if (descendant) { result.ancestors.add(node.path); hit = true; }
      }
      return hit;
    };
    walk(nodes);
    return result;
  }, [needle, nodes, children]);
  const anyVisible = useMemo(() => {
    if (needle) return nodes.some((node) => matches.has(node.path) || ancestors.has(node.path));
    if (langFilter) return nodes.some((node) => languageVisible(node, langFilter, children));
    return nodes.length > 0;
  }, [needle, nodes, matches, ancestors, langFilter, children]);
  const state: TreeState = { overrides, onOverrides, children, expanded, loading, failed, needle, langFilter, matches, ancestors, toggle, retry };
  /** Loaded rows across every fetched folder; the honest size of what search covers. Also feeds the language rail. */
  const loaded = useMemo(() => {
    const langs: Record<string, number> = {};
    let count = 0;
    const walk = (level: TreeNode[]) => { for (const node of level) { count += 1; if (node.type === 'file') { const resolved = resolveLanguage(node); if (resolved) langs[resolved] = (langs[resolved] ?? 0) + 1; } walk(children[node.path] ?? []); } };
    walk(nodes);
    return { count, langs, key: `${count}:${Object.keys(langs).sort().map((l) => `${l}=${langs[l]}`).join(',')}` };
  }, [nodes, children]);
  const reportedKey = useRef('');
  useEffect(() => {
    if (!onLoadedMeta || reportedKey.current === loaded.key) return;
    reportedKey.current = loaded.key;
    onLoadedMeta({ paths: loaded.count, langs: loaded.langs });
  }, [loaded, onLoadedMeta]);
  /** A language filter can only prune what has loaded, so engaging it fans out
   *  one level: top-level folders expand and fetch, then visibility rules hide
   *  every branch the loaded contents prove empty. The attempted-ref guard keeps
   *  this from re-firing: loadChildren bumps the loading/failed set identities
   *  on every call, and those sets feed this effect's deps. */
  const fanoutAttempted = useRef<Set<string>>(new Set());
  useEffect(() => {
    if (!langFilter) return;
    const pending = nodes.filter((node) => node.type === 'directory' && children[node.path] === undefined && !failed.has(node.path) && !fanoutAttempted.current.has(node.path));
    if (!pending.length) return;
    for (const node of pending) fanoutAttempted.current.add(node.path);
    setExpanded((old) => { const next = new Set(old); for (const node of pending) next.add(node.path); return next; });
    for (const node of pending) void loadChildren(node.path);
  }, [langFilter, nodes, children, failed, loadChildren]);
  /** Bulk-action targets: whole top-level subtrees when unfiltered (prefix overrides cover unloaded children too),
   *  exactly the query hits while searching, and the loaded files of one language under a language filter. */
  const bulkTargets = useMemo(() => {
    if (needle) return [...matches].sort();
    if (langFilter) {
      const out: string[] = [];
      const walk = (level: TreeNode[]) => { for (const node of level) { if (node.type === 'file' && resolveLanguage(node) === langFilter) out.push(node.path); walk(children[node.path] ?? []); } };
      walk(nodes);
      return out.sort();
    }
    return nodes.map((node) => node.path);
  }, [needle, matches, langFilter, nodes, children]);
  const bulkSelected = useMemo(() => bulkTargets.filter((path) => pathEffective(path, overrides, true)).length, [bulkTargets, overrides]);
  return <>
    <BulkBar targets={bulkTargets} selected={bulkSelected} overrides={overrides} onOverrides={onOverrides} filtered={needle !== '' || langFilter !== ''} />
    <p className="tree-loaded-count">{loaded.count} {loaded.count === 1 ? 'path' : 'paths'} loaded{langFilter ? ` · filtered by ${LANG_LABELS[langFilter] ?? langFilter}` : ''}</p>
    {needle && <div className="tree-search-meta" aria-live="polite"><p><strong>{matches.size}</strong> {matches.size === 1 ? 'matching path' : 'matching paths'}</p><p>Searching loaded folders — expand more to include their contents</p></div>}
    {anyVisible ? <TreeLevel nodes={nodes} state={state} root /> : !hasScans && nodes.length === 0 && !needle && !langFilter ? <Empty title="No files discovered yet" icon={<MagnifierIcon />}>Run a scan to populate the file tree.</Empty> : <Empty title="No matching paths" icon={<MagnifierIcon />}>Try a shorter search or clear the language filter.</Empty>}
  </>;
}

function TreeLevel({ nodes, state, root = false }: { nodes: TreeNode[]; state: TreeState; root?: boolean }) {
  const visible = useMemo(() => {
    let out = nodes;
    if (state.needle) out = out.filter((node) => state.matches.has(node.path) || state.ancestors.has(node.path));
    if (state.langFilter) out = out.filter((node) => languageVisible(node, state.langFilter, state.children));
    return out;
  }, [nodes, state]);
  return <ul className="file-tree" aria-label={root ? 'Workspace file tree' : undefined}>{visible.map((node, index) => {
    const open = state.expanded.has(node.path) || (state.needle !== '' && state.ancestors.has(node.path));
    const nodeLang = node.type === 'file' ? resolveLanguage(node) : '';
    // Stagger is capped so a 30-child folder doesn't hold its tail invisible for ~0.6s on every expand.
    return <li key={node.path} className="tree-item" style={{ animationDelay: `${Math.min(index, 10) * 20}ms` }}><div className="tree-row"><button type="button" className="tree-toggle" aria-label={open ? `Collapse ${node.name}` : `Expand ${node.name}`} disabled={node.type !== 'directory'} onClick={() => state.toggle(node)}>{state.loading.has(node.path) ? <span className="spinner" aria-hidden="true" /> : node.type === 'directory' ? (open ? '−' : '+') : '·'}</button><input type="checkbox" checked={nodeIncluded(node, state.overrides)} ref={(input) => { if (input) input.indeterminate = isPartial(node, state); }} onChange={() => toggleNode(node, state.overrides, state.onOverrides)} aria-label={`Include ${node.path}`} /><span className="flex items-center gap-1.5 min-w-0"><span aria-hidden="true">{nodeLang ? languageIcon(nodeLang) : null}</span><span className="tree-name"><HighlightedName name={node.name} needle={state.needle} />{nodeLang && <small className="tree-lang-badge">{LANG_LABELS[nodeLang] ?? nodeLang}</small>}</span>{state.needle && !node.name.toLowerCase().includes(state.needle) && <PathMatchHint path={node.path} needle={state.needle} />}</span>{node.excluded_reason && <small className="tree-excluded">Excluded: {excludedLabel(node.excluded_reason)}</small>}{state.failed.has(node.path) && <span className="tree-load-error">Could not load<button type="button" className="text-button" onClick={() => state.retry(node)}>Retry</button></span>}</div>{open && <div className="tree-children"><TreeLevel nodes={state.children[node.path] ?? []} state={state} /></div>}</li>;
  })}</ul>;
}

/** Tri-state for a directory: mixed effective states across the loaded subtree read as partial.
 *  An explicit override on the node itself is a decision, never partial; unloaded depths keep the server flag. */
function isPartial(node: TreeNode, state: TreeState): boolean {
  if (node.type !== 'directory') return false;
  if (state.overrides.some((item) => node.path === item.relative_path)) return false;
  const kids = state.children[node.path] ?? node.children ?? [];
  if (kids.length) {
    const seen = new Set<boolean>();
    const walk = (current: TreeNode) => {
      seen.add(nodeIncluded(current, state.overrides));
      for (const child of state.children[current.path] ?? current.children ?? []) walk(child);
    };
    walk(node);
    if (seen.size > 1) return true;
    return Boolean(node.partial); // uniform locally — unloaded depths still report through the server flag
  }
  return Boolean(node.partial);
}

function HighlightedName({ name, needle }: { name: string; needle: string }) {
  if (!needle) return <>{name}</>;
  const index = name.toLowerCase().indexOf(needle);
  if (index < 0) return <>{name}</>;
  return <>{name.slice(0, index)}<mark className="tree-hit">{name.slice(index, index + needle.length)}</mark>{name.slice(index + needle.length)}</>;
}

/** Search matches the whole path while names highlight alone; when the hit lives
 *  outside the name, echo the path tail around it (capped window, full path on
 *  hover) so the reason for the match is visible on the row itself. */
function PathMatchHint({ path, needle }: { path: string; needle: string }) {
  const index = path.toLowerCase().indexOf(needle);
  if (index < 0) return null;
  const end = index + needle.length;
  const start = Math.max(0, index - 12);
  const stop = Math.max(end, Math.min(path.length, start + 48));
  return (
    <small className="shrink-0 font-mono text-xs text-[var(--color-ink-faint)]" title={path}>
      {start > 0 ? '…' : ''}{path.slice(start, index)}<mark className="tree-hit">{path.slice(index, end)}</mark>{path.slice(end, stop)}{stop < path.length ? '…' : ''}
    </small>
  );
}

function nodeIncluded(node: TreeNode, overrides: PathOverride[]) { return pathEffective(node.path, overrides, node.included ?? !node.excluded_reason); }

/** Effective state of any path: longest matching override wins, otherwise the server/default fallback. */
function pathEffective(path: string, overrides: PathOverride[], fallback: boolean) {
  const matching = overrides.filter((item) => path === item.relative_path || path.startsWith(`${item.relative_path}/`)).sort((a, b) => b.relative_path.length - a.relative_path.length)[0];
  return matching ? matching.mode === 'include' : fallback;
}

/** Drop exact duplicates (last wins) and entries a same-mode ancestor already covers — keeps the override list shippable. */
function pruneOverrides(items: PathOverride[]): PathOverride[] {
  const byPath = new Map<string, PathOverride>();
  for (const item of items) byPath.set(item.relative_path, item);
  return [...byPath.values()].filter((item) => {
    const parts = item.relative_path.split('/');
    for (let i = parts.length - 1; i >= 1; i -= 1) {
      if (byPath.get(parts.slice(0, i).join('/'))?.mode === item.mode) return false;
    }
    return true;
  });
}

function toggleNode(node: TreeNode, overrides: PathOverride[], onOverrides: (items: PathOverride[]) => void) { const next = !nodeIncluded(node, overrides); onOverrides(pruneOverrides([...overrides.filter((item) => item.relative_path !== node.path), { relative_path: node.path, mode: next ? 'include' : 'exclude' }])); }

/** Mass select/unselect for the currently shown paths: one click instead of one checkbox per row. */
function BulkBar({ targets, selected, overrides, onOverrides, filtered }: { targets: string[]; selected: number; overrides: PathOverride[]; onOverrides: (items: PathOverride[]) => void; filtered: boolean }) {
  if (!targets.length) return null;
  const allSelected = selected === targets.length;
  const apply = (mode: 'include' | 'exclude') => {
    const kept = overrides.filter((item) => !targets.includes(item.relative_path));
    onOverrides(pruneOverrides([...kept, ...targets.map((relative_path) => ({ relative_path, mode } as PathOverride))]));
  };
  const invert = () => {
    const kept = overrides.filter((item) => !targets.includes(item.relative_path));
    onOverrides(pruneOverrides([...kept, ...targets.map((relative_path) => ({ relative_path, mode: pathEffective(relative_path, overrides, true) ? 'exclude' : 'include' } as PathOverride))]));
  };
  return <div className="tree-bulkbar" role="toolbar" aria-label="Bulk selection">
    <span className="tree-bulkbar-count tabular-nums" aria-live="polite">{selected} of {targets.length} shown included{filtered ? '' : ' — folders cover their whole subtree'}</span>
    <span className="tree-bulkbar-actions">
      <button type="button" className="button secondary tree-bulkbar-btn" disabled={allSelected} onClick={() => apply('include')}>Select all</button>
      <button type="button" className="button secondary tree-bulkbar-btn" disabled={selected === 0} onClick={() => apply('exclude')}>Exclude all</button>
      <button type="button" className="button ghost tree-bulkbar-btn" onClick={invert}>Invert</button>
    </span>
  </div>;
}

function RuleEditor({ rules, setRules }: { rules: RuleDraft[]; setRules: (next: RuleDraft[]) => void }) {
  function edit(uid: number, patch: Partial<RuleDraft>) { setRules(rules.map((rule) => rule.uid === uid ? { ...rule, ...patch } : rule)); }
  return <aside className="rule-editor"><header className="rule-editor-head"><div><p className="eyebrow">Overrides</p><h2>Rules</h2></div></header><p className="muted rule-editor-desc">Rules apply to new files too. Include or exclude paths beyond defaults.</p>{rules.length === 0 ? <div className="rule-empty-illustration"><FolderCog size={40} className="rule-empty-icon" aria-hidden /><p className="muted">No custom rules yet. Add one to include or exclude paths beyond defaults.</p></div> : <div className="rule-list">{rules.map((rule) => <div className="rule" key={rule.uid}><div className="segmented" role="group" aria-label="Rule type"><button type="button" className={`segmented-btn${rule.rule_type === 'include' ? ' active' : ''}`} aria-pressed={rule.rule_type === 'include'} onClick={() => edit(rule.uid, { rule_type: 'include' })}>Include</button><button type="button" className={`segmented-btn${rule.rule_type === 'exclude' ? ' active' : ''}`} aria-pressed={rule.rule_type === 'exclude'} onClick={() => edit(rule.uid, { rule_type: 'exclude' })}>Exclude</button></div><input className="rule-pattern" value={rule.pattern} onChange={(event) => edit(rule.uid, { pattern: event.target.value })} placeholder="**/generated/**" aria-label="Rule pattern" /><button type="button" className="icon-button rule-remove" onClick={() => setRules(rules.filter((item) => item.uid !== rule.uid))} aria-label="Remove rule"><X size={14} /></button></div>)}</div>}<button type="button" className="rule-add" onClick={() => setRules([...rules, { uid: nextRuleUid(), rule_type: 'exclude', pattern: '' }])}><Plus size={14} aria-hidden /> Add rule</button><p className="rule-help">Use glob patterns like <code>**/generated/**</code> or <code>src/legacy/**</code>. Include wins over exclude for the matched path.</p></aside>;
}
