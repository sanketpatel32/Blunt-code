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
  python: 'Python', javascript: 'JavaScript', typescript: 'TypeScript', go: 'Go', java: 'Java', kotlin: 'Kotlin', csharp: 'C#', c: 'C', cpp: 'C++', ruby: 'Ruby', php: 'PHP', rust: 'Rust', swift: 'Swift', scala: 'Scala', dart: 'Dart', elixir: 'Elixir', 'objective-c': 'ObjC', vue: 'Vue', svelte: 'Svelte', html: 'HTML', css: 'CSS', scss: 'SCSS', json: 'JSON', yaml: 'YAML', toml: 'TOML', xml: 'XML', sql: 'SQL', graphql: 'GraphQL', shell: 'Shell', powershell: 'PS', batch: 'Batch', markdown: 'MD', dockerfile: 'Dockerfile', env: 'Env',
};

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
    case 'markdown': return <FileText {...props} />;
    case 'env': return <Settings2 {...props} />;
    default: return <FileCode {...props} />;
  }
}

export function FilesPage({ id, go, notify }: { id: string; go?: (r: Route) => void; notify: (n: Notice) => void }) {
  const workspace = useLoad(() => api.workspace(id), [id]);
  const initialLang = useMemo(() => {
    try { return new URLSearchParams(window.location.search).get('lang') ?? ''; } catch { return ''; }
  }, []);
  const [query, setQuery] = useState('');
  const [lang, setLang] = useState(initialLang);
  /** Typing stays instant while the tree is only re-filtered once input settles; clearing applies immediately. */
  const debouncedQuery = useDebouncedValue(query, query ? 200 : 0);
  const debouncedLang = useDebouncedValue(lang, 0);
  const [nodes, setNodes] = useState<TreeNode[]>([]);
  const [treeError, setTreeError] = useState<string>();
  const [loadingTree, setLoadingTree] = useState(true);
  const [rules, setRules] = useState<{ rules: RuleDraft[] }>({ rules: [] });
  const [overrides, setOverrides] = useState<PathOverride[]>([]);
  const [treeKey, setTreeKey] = useState(0);
  /** Bumped by Collapse all; FileTree watches it and folds every open folder without dropping loaded children. */
  const [collapseSignal, setCollapseSignal] = useState(0);
  /** Languages actually present in the loaded tree (grows as folders expand); the filter rail only offers these. */
  const [loadedMeta, setLoadedMeta] = useState<{ paths: number; langs: Record<string, number> }>({ paths: 0, langs: {} });
  const searchRef = useRef<HTMLInputElement>(null);
  const loadTree = useCallback(async () => { setLoadingTree(true); try { setNodes(await api.tree(id)); setTreeError(undefined); } catch (e) { setTreeError(message(e)); } finally { setLoadingTree(false); } }, [id]);
  useEffect(() => { void loadTree(); void Promise.all([api.rules(id), api.pathOverrides(id)]).then(([savedRules, savedOverrides]) => { setRules({ rules: (savedRules as { rules: Array<Omit<RuleDraft, 'uid'>> }).rules.map((rule) => ({ ...rule, uid: nextRuleUid() })) }); setOverrides(savedOverrides); }).catch((e) => notify({ kind: 'error', text: message(e) })); }, [id, loadTree, notify]);
  /** Empty pattern rows are unfinished drafts, not rules — the server rightly rejects them, so they never leave the client. */
  const save = async () => { try { await api.saveRules(id, rules.rules.filter((rule) => rule.pattern.trim() !== '').map(({ uid: _uid, pattern, ...rule }) => ({ ...rule, pattern: pattern.trim() }))); await api.savePathOverrides(id, overrides); await loadTree(); setTreeKey((value) => value + 1); notify({ kind: 'info', text: 'File selection saved for this workspace.' }); } catch (e) { notify({ kind: 'error', text: message(e) }); } };
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
              <Button variant="ghost" size="sm" onClick={() => { setRules({ rules: [] }); setOverrides([]); }} className="gap-1.5 text-xs">
                <RotateCcw size={14} aria-hidden />Reset
              </Button>
              <Button variant="default" size="sm" onClick={save} className="gap-1.5 text-xs">
                <Save size={14} aria-hidden />Save selection
              </Button>
            </div>
          }
        />
    <section className="file-layout"><div className="tree-panel"><div className="tree-panel-head"><div><p className="eyebrow">Source tree</p><h2 className="tree-panel-title">Workspace files</h2></div><span className="tree-count-badge tabular-nums">{nodes.length ? `${nodes.length} top-level` : '—'}</span></div><div className="tree-panel-controls"><label className="search file-search"><span className="sr-only">Search paths</span><span className="file-search-wrap"><Search size={14} className="file-search-icon" aria-hidden /><input ref={searchRef} value={query} onChange={(event) => setQuery(event.target.value)} onKeyDown={(event) => { if (event.key === 'Escape') { setQuery(''); event.currentTarget.blur(); } }} placeholder="src or package.json" className="file-search-input" /><kbd className="kbd-hint">/</kbd></span></label><fieldset className="tree-toolbar chip-group chip-rail" aria-label="Filter by language">
          <button type="button" className="chip" aria-pressed={lang === ''} onClick={() => setLang('')}>All languages</button>
          {langEntries.map(([l, count]) => {
            const label = LANG_LABELS[l] ?? l;
            return <button key={l} type="button" className="chip" aria-pressed={lang === l} onClick={() => setLang(l)}>{label}<small className="chip-count">{count}</small></button>;
          })}
        {lang && <button type="button" className="text-button" onClick={() => setLang('')}>Clear filter</button>}{!loadingTree && !treeError && <button type="button" className="button ghost tree-collapse-btn" onClick={() => setCollapseSignal((value) => value + 1)}>Collapse all</button>}</fieldset><div className="tree-scroll">{loadingTree ? <SkeletonLines lines={6} /> : treeError ? <ErrorPanel error={treeError} retry={loadTree} /> : <FileTree key={treeKey} nodes={nodes} query={debouncedQuery} lang={debouncedLang} workspaceId={id} overrides={overrides} onOverrides={setOverrides} collapseSignal={collapseSignal} onLoadedMeta={setLoadedMeta} />}</div><div className="tree-summary-bar"><span className="tabular-nums">{selectedCount ? `${selectedCount} paths selected` : `${nodes.length} paths`}</span><span aria-hidden>·</span><span className="tabular-nums">{langDistinct} languages</span></div></div></div><RuleEditor rules={rules.rules} setRules={(items) => setRules({ rules: items })} /></section>
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

function FileTree({ nodes, query, lang, workspaceId, overrides, onOverrides, collapseSignal = 0, onLoadedMeta }: { nodes: TreeNode[]; query: string; lang?: string; workspaceId: string; overrides: PathOverride[]; onOverrides: (items: PathOverride[]) => void; collapseSignal?: number; onLoadedMeta?: (meta: { paths: number; langs: Record<string, number> }) => void }) {
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
    if (langFilter) {
      const walk = (level: TreeNode[]): boolean => level.some((n) => (n.language?.toLowerCase() === langFilter) || walk(children[n.path] ?? []));
      // at top level check if any node matches filter or has matching descendants
      return walk(nodes);
    }
    return nodes.length > 0;
  }, [needle, nodes, matches, ancestors, langFilter, children]);
  const state: TreeState = { overrides, onOverrides, children, expanded, loading, failed, needle, langFilter, matches, ancestors, toggle, retry };
  /** Loaded rows across every fetched folder; the honest size of what search covers. Also feeds the language rail. */
  const loaded = useMemo(() => {
    const langs: Record<string, number> = {};
    let count = 0;
    const walk = (level: TreeNode[]) => { for (const node of level) { count += 1; if (node.type === 'file' && node.language) langs[node.language] = (langs[node.language] ?? 0) + 1; walk(children[node.path] ?? []); } };
    walk(nodes);
    return { count, langs, key: `${count}:${Object.keys(langs).sort().map((l) => `${l}=${langs[l]}`).join(',')}` };
  }, [nodes, children]);
  const reportedKey = useRef('');
  useEffect(() => {
    if (!onLoadedMeta || reportedKey.current === loaded.key) return;
    reportedKey.current = loaded.key;
    onLoadedMeta({ paths: loaded.count, langs: loaded.langs });
  }, [loaded, onLoadedMeta]);
  /** Bulk-action targets: whole top-level subtrees when unfiltered (prefix overrides cover unloaded children too),
   *  exactly the query hits while searching, and the loaded files of one language under a language filter. */
  const bulkTargets = useMemo(() => {
    if (needle) return [...matches].sort();
    if (langFilter) {
      const out: string[] = [];
      const walk = (level: TreeNode[]) => { for (const node of level) { if (node.type === 'file' && node.language?.toLowerCase() === langFilter) out.push(node.path); walk(children[node.path] ?? []); } };
      walk(nodes);
      return out.sort();
    }
    return nodes.map((node) => node.path);
  }, [needle, matches, langFilter, nodes, children]);
  const bulkSelected = useMemo(() => bulkTargets.filter((path) => pathEffective(path, overrides, true)).length, [bulkTargets, overrides]);
  return <>
    <BulkBar targets={bulkTargets} selected={bulkSelected} overrides={overrides} onOverrides={onOverrides} filtered={needle !== '' || langFilter !== ''} />
    <p className="tree-loaded-count">{loaded.count} {loaded.count === 1 ? 'path' : 'paths'} loaded{langFilter ? ` · filtered by ${langFilter}` : ''}</p>
    {needle && <div className="tree-search-meta" aria-live="polite"><p><strong>{matches.size}</strong> {matches.size === 1 ? 'matching path' : 'matching paths'}</p><p>Searching loaded folders — expand more to include their contents</p></div>}
    {anyVisible ? <TreeLevel nodes={nodes} state={state} root /> : <Empty title="No matching paths" icon={<MagnifierIcon />}>Try a shorter search or clear the language filter.</Empty>}
  </>;
}

function TreeLevel({ nodes, state, root = false }: { nodes: TreeNode[]; state: TreeState; root?: boolean }) {
  const visible = useMemo(() => {
    let out = nodes;
    if (state.needle) out = out.filter((node) => state.matches.has(node.path) || state.ancestors.has(node.path));
    if (state.langFilter) {
      out = out.filter((node) => {
        if (node.type === 'directory') {
          const descendants = state.children[node.path] ?? node.children ?? [];
          const hasLangDescendant = descendants.some((c) => c.language?.toLowerCase() === state.langFilter) || node.path.toLowerCase().includes(state.langFilter);
          // keep directories if they contain matching files deeper (checked via children cache)
          if (hasLangDescendant) return true;
          // also keep empty dirs when filter active? hide if no match
          return false;
        }
        return (node.language?.toLowerCase() === state.langFilter);
      });
    }
    return out;
  }, [nodes, state]);
  return <ul className="file-tree" aria-label={root ? 'Workspace file tree' : undefined}>{visible.map((node, index) => {
    const open = state.expanded.has(node.path) || (state.needle !== '' && state.ancestors.has(node.path));
    return <li key={node.path} className="tree-item" style={{ animationDelay: `${index * 20}ms` }}><div className="tree-row"><button type="button" className="tree-toggle" aria-label={open ? `Collapse ${node.name}` : `Expand ${node.name}`} disabled={node.type !== 'directory'} onClick={() => state.toggle(node)}>{state.loading.has(node.path) ? <span className="spinner" aria-hidden="true" /> : node.type === 'directory' ? (open ? '−' : '+') : '·'}</button><input type="checkbox" checked={nodeIncluded(node, state.overrides)} ref={(input) => { if (input) input.indeterminate = isPartial(node, state); }} onChange={() => toggleNode(node, state.overrides, state.onOverrides)} aria-label={`Include ${node.path}`} /><span className="flex items-center gap-1.5 min-w-0"><span aria-hidden="true">{node.type === 'file' ? languageIcon(node.language) : null}</span><span className="tree-name"><HighlightedName name={node.name} needle={state.needle} />{node.language && node.type === 'file' && <small className="tree-lang-badge">{LANG_LABELS[node.language] ?? node.language}</small>}</span></span>{node.excluded_reason && <small className="tree-excluded">Excluded: {node.excluded_reason}</small>}{state.failed.has(node.path) && <span className="tree-load-error">Could not load<button type="button" className="text-button" onClick={() => state.retry(node)}>Retry</button></span>}</div>{open && <div className="tree-children"><TreeLevel nodes={state.children[node.path] ?? []} state={state} /></div>}</li>;
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
    <span className="tree-bulkbar-count tabular-nums" aria-live="polite">{selected} of {targets.length} shown selected{filtered ? '' : ' — folders cover their whole subtree'}</span>
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
