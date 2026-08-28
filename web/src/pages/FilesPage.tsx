import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { FileCode, FileJson, FileText, FileBraces, Braces, Palette, Database, Container, Settings2, ScrollText, Code2, FileSpreadsheet } from 'lucide-react';
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
import { ALL_LANGUAGES } from '../lib/analyzerCatalog';

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
  const searchRef = useRef<HTMLInputElement>(null);
  const loadTree = useCallback(async () => { setLoadingTree(true); try { setNodes(await api.tree(id)); setTreeError(undefined); } catch (e) { setTreeError(message(e)); } finally { setLoadingTree(false); } }, [id]);
  useEffect(() => { void loadTree(); void Promise.all([api.rules(id), api.pathOverrides(id)]).then(([savedRules, savedOverrides]) => { setRules({ rules: (savedRules as { rules: Array<Omit<RuleDraft, 'uid'>> }).rules.map((rule) => ({ ...rule, uid: nextRuleUid() })) }); setOverrides(savedOverrides); }).catch((e) => notify({ kind: 'error', text: message(e) })); }, [id, loadTree, notify]);
  const save = async () => { try { await api.saveRules(id, rules.rules.map(({ uid: _uid, ...rule }) => rule)); await api.savePathOverrides(id, overrides); await loadTree(); setTreeKey((value) => value + 1); notify({ kind: 'info', text: 'File selection saved for this workspace.' }); } catch (e) { notify({ kind: 'error', text: message(e) }); } };
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
  const langCounts = useMemo(() => {
    const counts: Record<string, number> = {};
    function walk(level: TreeNode[]) { for (const n of level) { if (n.language) counts[n.language] = (counts[n.language] ?? 0) + 1; if (n.type === 'directory' && n.children) walk(n.children); } }
    // count from loaded nodes - approximate via nodes array (children loaded separately not counted here, but FileTree provides deeper counts via loaded rendering)
    for (const n of nodes) if (n.language) counts[n.language] = (counts[n.language] ?? 0) + 1;
    return counts;
  }, [nodes]);
  return <div className="page workspace-page">{go && <WorkspaceContextSidebar id={id} current={{ page: 'files', id }} onNavigate={go} />}<div className="workspace-page-body"><header className="page-heading"><div><p className="eyebrow">File selection</p><h1>{workspace.data?.name ?? 'Workspace files'}</h1><code className="workspace-root" title={workspace.data?.root_path}>{workspace.data?.root_path}</code><p>Choose source paths to analyze. Default exclusions protect dependencies and build output.</p></div><div className="action-row"><button type="button" className="button secondary" onClick={() => { setRules({ rules: [] }); setOverrides([]); }}>Reset to defaults</button><button type="button" className="button primary" onClick={save}>Save selection</button></div></header>
    <section className="file-layout"><div className="tree-panel"><label className="search"><span>Search paths<kbd className="kbd-hint">/</kbd></span><input ref={searchRef} value={query} onChange={(event) => setQuery(event.target.value)} onKeyDown={(event) => { if (event.key === 'Escape') { setQuery(''); event.currentTarget.blur(); } }} placeholder="src or package.json" /></label><div className="tree-toolbar" style={{display:'flex',gap:'.5rem',alignItems:'center',flexWrap:'wrap'}}><select aria-label="Filter by language" value={lang} onChange={e=>setLang(e.target.value)} className="border rounded-[var(--radius-button)] px-2 py-1 text-sm max-w-[14rem]">
          <option value="">All languages</option>
          {(ALL_LANGUAGES as readonly string[]).map((l) => {
            const count = langCounts[l] ?? 0;
            const label = LANG_LABELS[l] ?? l;
            return <option key={l} value={l}>{label}{count ? ` (${count})` : ''}</option>;
          })}
        </select>{lang && <button type="button" className="text-button" onClick={() => setLang('')}>Clear filter</button>}{!loadingTree && !treeError && <button type="button" className="text-button" onClick={() => setCollapseSignal((value) => value + 1)}>Collapse all</button>}</div>{loadingTree ? <SkeletonLines lines={6} /> : treeError ? <ErrorPanel error={treeError} retry={loadTree} /> : <FileTree key={treeKey} nodes={nodes} query={debouncedQuery} lang={debouncedLang} workspaceId={id} overrides={overrides} onOverrides={setOverrides} collapseSignal={collapseSignal} />}</div><RuleEditor rules={rules.rules} setRules={(items) => setRules({ rules: items })} /></section>
  </div></div>;
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

function FileTree({ nodes, query, lang, workspaceId, overrides, onOverrides, collapseSignal = 0 }: { nodes: TreeNode[]; query: string; lang?: string; workspaceId: string; overrides: PathOverride[]; onOverrides: (items: PathOverride[]) => void; collapseSignal?: number }) {
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
  /** Loaded rows across every fetched folder; the honest size of what search covers. */
  const loadedCount = useMemo(() => {
    let count = 0;
    const walk = (level: TreeNode[]) => { for (const node of level) { count += 1; walk(children[node.path] ?? []); } };
    walk(nodes);
    return count;
  }, [nodes, children]);
  return <>
    <p className="tree-loaded-count">{loadedCount} {loadedCount === 1 ? 'path' : 'paths'} loaded{langFilter ? ` · filtered by ${langFilter}` : ''}</p>
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
  return <ul className="file-tree" aria-label={root ? 'Workspace file tree' : undefined}>{visible.map((node) => {
    // While a query is active, folders leading to a match render expanded; clearing the query falls back to the manual set.
    const open = state.expanded.has(node.path) || (state.needle !== '' && state.ancestors.has(node.path));
    return <li key={node.path}><div className="tree-row"><button type="button" className="tree-toggle" aria-label={open ? `Collapse ${node.name}` : `Expand ${node.name}`} disabled={node.type !== 'directory'} onClick={() => state.toggle(node)}>{state.loading.has(node.path) ? <span className="spinner" aria-hidden="true" /> : node.type === 'directory' ? (open ? '−' : '+') : '·'}</button><input type="checkbox" checked={nodeIncluded(node, state.overrides)} ref={(input) => { if (input) input.indeterminate = Boolean(node.partial && !state.overrides.some((item) => node.path === item.relative_path)); }} onChange={() => toggleNode(node, state.overrides, state.onOverrides)} aria-label={`Include ${node.path}`} /><span className="flex items-center gap-1.5 min-w-0"><span aria-hidden="true">{node.type === 'file' ? languageIcon(node.language) : null}</span><span className="tree-name"><HighlightedName name={node.name} needle={state.needle} />{node.language && node.type === 'file' && <small className="ml-1 rounded bg-[var(--color-surface-muted)] px-1 py-0.5 text-[10px] text-[var(--color-ink-faint)]">{LANG_LABELS[node.language] ?? node.language}</small>}</span></span>{node.excluded_reason && <small>Excluded: {node.excluded_reason}</small>}{state.failed.has(node.path) && <span className="tree-load-error">Could not load<button type="button" className="text-button" onClick={() => state.retry(node)}>Retry</button></span>}</div>{open && <div className="tree-children" style={{overflow:'hidden', transition: 'height 200ms var(--ease-out)'}}><TreeLevel nodes={state.children[node.path] ?? []} state={state} /></div>}</li>;
  })}</ul>;
}

/** Wraps the first case-insensitive hit in the visible name so it is obvious why a row matched. */
function HighlightedName({ name, needle }: { name: string; needle: string }) {
  if (!needle) return <>{name}</>;
  const index = name.toLowerCase().indexOf(needle);
  if (index < 0) return <>{name}</>;
  return <>{name.slice(0, index)}<mark className="tree-hit" style={{background:'var(--color-accent-soft)', borderRadius:'2px'}}>{name.slice(index, index + needle.length)}</mark>{name.slice(index + needle.length)}</>;
}

function nodeIncluded(node: TreeNode, overrides: PathOverride[]) { const matching = overrides.filter((item) => node.path === item.relative_path || node.path.startsWith(`${item.relative_path}/`)).sort((a, b) => b.relative_path.length - a.relative_path.length)[0]; return matching ? matching.mode === 'include' : node.included ?? !node.excluded_reason; }

function toggleNode(node: TreeNode, overrides: PathOverride[], onOverrides: (items: PathOverride[]) => void) { const next = !nodeIncluded(node, overrides); onOverrides([...overrides.filter((item) => item.relative_path !== node.path), { relative_path: node.path, mode: next ? 'include' : 'exclude' }]); }

function RuleEditor({ rules, setRules }: { rules: RuleDraft[]; setRules: (next: RuleDraft[]) => void }) {
  function edit(uid: number, patch: Partial<RuleDraft>) { setRules(rules.map((rule) => rule.uid === uid ? { ...rule, ...patch } : rule)); }
  return <aside className="rule-editor"><h2>Rules</h2><p className="muted">Rules apply to new files too.</p>{rules.map((rule) => <div className="rule" key={rule.uid}><select value={rule.rule_type} onChange={(event) => edit(rule.uid, { rule_type: event.target.value as 'include' | 'exclude' })}><option value="include">Include</option><option value="exclude">Exclude</option></select><input value={rule.pattern} onChange={(event) => edit(rule.uid, { pattern: event.target.value })} aria-label="Rule pattern" /><button type="button" className="icon-button" onClick={() => setRules(rules.filter((item) => item.uid !== rule.uid))} aria-label="Remove rule">×</button></div>)}<button type="button" className="text-button" onClick={() => setRules([...rules, { uid: nextRuleUid(), rule_type: 'exclude', pattern: '' }])}>+ Add rule</button></aside>;
}
