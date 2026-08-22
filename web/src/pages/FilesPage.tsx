import { useCallback, useEffect, useState } from 'react';
import { api } from '../api';
import type { PathOverride, TreeNode } from '../types';
import type { Notice } from '../lib/notice';
import { message } from '../lib/notice';
import { useLoad } from '../hooks/useLoad';
import { Empty, ErrorPanel } from '../components/ui';
import { SkeletonLines } from '../components/skeletons';

export function FilesPage({ id, notify }: { id: string; notify: (n: Notice) => void }) {
  const workspace = useLoad(() => api.workspace(id), [id]);
  const [query, setQuery] = useState('');
  const [nodes, setNodes] = useState<TreeNode[]>([]);
  const [treeError, setTreeError] = useState<string>();
  const [loadingTree, setLoadingTree] = useState(true);
  const [rules, setRules] = useState<{ rules: Array<{ rule_type: 'include' | 'exclude'; pattern: string; enabled?: boolean }> }>({ rules: [] });
  const [overrides, setOverrides] = useState<PathOverride[]>([]);
  const [treeKey, setTreeKey] = useState(0);
  const loadTree = useCallback(async () => { setLoadingTree(true); try { setNodes(await api.tree(id)); setTreeError(undefined); } catch (e) { setTreeError(message(e)); } finally { setLoadingTree(false); } }, [id]);
  useEffect(() => { void loadTree(); void Promise.all([api.rules(id), api.pathOverrides(id)]).then(([savedRules, savedOverrides]) => { setRules(savedRules as typeof rules); setOverrides(savedOverrides); }).catch((e) => notify({ kind: 'error', text: message(e) })); }, [id, loadTree]);
  const save = async () => { try { await api.saveRules(id, rules); await api.savePathOverrides(id, overrides); await loadTree(); setTreeKey((value) => value + 1); notify({ kind: 'info', text: 'File selection saved for this workspace.' }); } catch (e) { notify({ kind: 'error', text: message(e) }); } };
  const filtered = query ? nodes.filter((node) => node.path.toLowerCase().includes(query.toLowerCase())) : nodes;
  return <div className="page"><header className="page-heading"><div><p className="eyebrow">File selection</p><h1>{workspace.data?.name ?? 'Workspace files'}</h1><p>Choose source paths to analyze. Default exclusions protect dependencies and build output.</p></div><div className="action-row"><button className="button secondary" onClick={() => { setRules({ rules: [] }); setOverrides([]); }}>Reset to defaults</button><button className="button primary" onClick={save}>Save selection</button></div></header>
    <section className="file-layout"><div className="tree-panel"><label className="search"><span>Search paths</span><input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="src or package.json" /></label>{loadingTree ? <SkeletonLines lines={6} /> : treeError ? <ErrorPanel error={treeError} retry={loadTree} /> : filtered.length ? <FileTree key={treeKey} nodes={filtered} workspaceId={id} overrides={overrides} onOverrides={setOverrides} /> : <Empty title="No matching paths">Try a shorter search.</Empty>}</div><RuleEditor rules={rules.rules} setRules={(items) => setRules({ rules: items })} /></section>
  </div>;
}

function FileTree({ nodes, workspaceId, overrides, onOverrides }: { nodes: TreeNode[]; workspaceId: string; overrides: PathOverride[]; onOverrides: (items: PathOverride[]) => void }) {
  const [expanded, setExpanded] = useState<Set<string>>(new Set());
  const [children, setChildren] = useState<Record<string, TreeNode[]>>({});
  async function expand(node: TreeNode) { if (node.type !== 'directory') return; const next = new Set(expanded); if (next.has(node.path)) { next.delete(node.path); setExpanded(next); return; } next.add(node.path); setExpanded(next); if (!children[node.path]) { try { const loaded = await api.tree(workspaceId, node.path); setChildren((old) => ({ ...old, [node.path]: loaded })); } catch { /* Root remains usable; API errors are surfaced on subsequent retry. */ } } }
  function included(node: TreeNode) { const matching = overrides.filter((item) => node.path === item.relative_path || node.path.startsWith(`${item.relative_path}/`)).sort((a, b) => b.relative_path.length - a.relative_path.length)[0]; return matching ? matching.mode === 'include' : node.included ?? !node.excluded_reason; }
  function toggle(node: TreeNode) { const next = !included(node); onOverrides([...overrides.filter((item) => item.relative_path !== node.path), { relative_path: node.path, mode: next ? 'include' : 'exclude' }]); }
  return <ul className="file-tree" aria-label="Workspace file tree">{nodes.map((node) => <li key={node.path}><div className="tree-row"><button className="tree-toggle" aria-label={expanded.has(node.path) ? `Collapse ${node.name}` : `Expand ${node.name}`} disabled={node.type !== 'directory'} onClick={() => void expand(node)}>{node.type === 'directory' ? (expanded.has(node.path) ? '−' : '+') : '·'}</button><input type="checkbox" checked={included(node)} ref={(input) => { if (input) input.indeterminate = Boolean(node.partial && !overrides.some((item) => node.path === item.relative_path)); }} onChange={() => toggle(node)} aria-label={`Include ${node.path}`} /><span className="tree-name">{node.name}</span>{node.excluded_reason && <small>Excluded: {node.excluded_reason}</small>}</div>{expanded.has(node.path) && <div className="tree-children"><FileTree nodes={children[node.path] ?? []} workspaceId={workspaceId} overrides={overrides} onOverrides={onOverrides} /></div>}</li>)}</ul>;
}

function RuleEditor({ rules, setRules }: { rules: Array<{ rule_type: 'include' | 'exclude'; pattern: string; enabled?: boolean }>; setRules: (next: Array<{ rule_type: 'include' | 'exclude'; pattern: string; enabled?: boolean }>) => void }) {
  function edit(index: number, patch: Partial<(typeof rules)[number]>) { setRules(rules.map((rule, i) => i === index ? { ...rule, ...patch } : rule)); }
  return <aside className="rule-editor"><h2>Rules</h2><p className="muted">Rules apply to new files too.</p>{rules.map((rule, index) => <div className="rule" key={`${rule.pattern}-${index}`}><select value={rule.rule_type} onChange={(event) => edit(index, { rule_type: event.target.value as 'include' | 'exclude' })}><option value="include">Include</option><option value="exclude">Exclude</option></select><input value={rule.pattern} onChange={(event) => edit(index, { pattern: event.target.value })} aria-label="Rule pattern" /><button className="icon-button" onClick={() => setRules(rules.filter((_, i) => i !== index))} aria-label="Remove rule">×</button></div>)}<button className="text-button" onClick={() => setRules([...rules, { rule_type: 'exclude', pattern: '' }])}>+ Add rule</button></aside>;
}
