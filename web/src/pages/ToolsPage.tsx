import { useEffect, useMemo, useRef, useState } from 'react';
import '../css/tools.css';
import { api } from '../api';
import type { Tool } from '../types';
import type { Notice } from '../lib/notice';
import { message } from '../lib/notice';
import { useLoad } from '../hooks/useLoad';
import { Empty, ErrorPanel } from '../components/ui';
import { WrenchIcon } from '../components/icons';
import { SkeletonTable } from '../components/skeletons';
import { ANALYZER_CATALOG, analyzerMeta, categoryColor, CATEGORY_LABELS, type AnalyzerCategory } from '../lib/analyzerCatalog';
import { LanguageCoverage } from '../components/LanguageCoverage';
import { PentestSection } from './PentestPage';
import type { Route } from '../lib/router';
import { PageHeader } from '../components/PageHeader';
import { Wrench, Shield, Bug, KeyRound, Boxes, Container, FileCog, Scale, Palette, Zap, Crosshair, Radar, Package, ListTodo, Gauge, Download, WrenchIcon as RepairIcon, RefreshCw, MoreHorizontal } from 'lucide-react';

type ToolOperation = 'install' | 'repair' | 'update';
type BusyAction = { tool: string; operation: ToolOperation };

const operationLabels: Record<ToolOperation, string> = { install: 'Install', repair: 'Repair', update: 'Update' };
const operationBusyLabels: Record<ToolOperation, string> = { install: 'Installing…', repair: 'Repairing…', update: 'Updating…' };
const operationVerbs: Record<ToolOperation, string> = { install: 'installed', repair: 'repaired', update: 'updated' };

// In-process analyzers bundled with the app. They are not managed downloads,
// so the tools API never lists them — presenting them as "coming soon" hides
// two analyzers that already run on every standard/deep scan.
const BUILT_IN_IDS = new Set(['secrets', 'todo', 'license-scan']);

const categoryIcons: Record<string, React.ElementType> = {
  lint: Wrench, style: Palette, security: Shield, pentest: Zap, secrets: KeyRound, maintainability: Gauge,
  dependencies: Package, container: Container, iac: FileCog, license: Scale,
};

function ReadinessStrip({ tools, busy }: { tools: Tool[]; busy?: BusyAction }) {
  const ready = tools.filter((tool) => tool.ready).length;
  return <div className="tools-readiness" role="status"><span className={`badge${ready === tools.length ? ' tools-all-ready' : ''}`}>{ready} of {tools.length} ready</span>{tools.filter((tool) => !tool.ready).map((tool) => {
    const active = busy !== undefined && busy.tool === tool.id ? busy.operation : undefined;
    const name = tool.name?.trim() || tool.id;
    return <span key={tool.id} className="badge">{active ? <span className="spinner" aria-hidden="true" /> : <i className="dot not-ready" aria-hidden="true" />}{name} {active ? operationBusyLabels[active].toLowerCase() : 'not installed'}</span>;
  })}</div>;
}

function ToolActions({ tool, activeOperation, onAction }: { tool: Tool; activeOperation?: ToolOperation; onAction: (t: Tool, op: ToolOperation)=>void }) {
  const [open,setOpen]=useState(false);
  const ref=useRef<HTMLDivElement>(null);
  const ops: Array<{op: ToolOperation; icon: React.ElementType; label:string}> = [{op:'install', icon: Download, label:'Install'},{op:'repair', icon: RepairIcon, label:'Repair'},{op:'update', icon: RefreshCw, label:'Update'}];
  
  // close on outside
  const handleOpen=()=> setOpen(v=>!v); useEffect(()=>{ if(!open) return; const onDown=(e: MouseEvent)=>{ if(!ref.current?.contains(e.target as Node)) setOpen(false); }; const onEsc=(e: KeyboardEvent)=>{ if(e.key==="Escape") setOpen(false); }; document.addEventListener("mousedown",onDown); document.addEventListener("keydown",onEsc); return()=>{ document.removeEventListener("mousedown",onDown); document.removeEventListener("keydown",onEsc); }; },[open]);
  return <div ref={ref} className="relative inline-block">
    <button type="button" aria-haspopup="menu" aria-expanded={open} onClick={handleOpen} disabled={activeOperation!==undefined} className="inline-flex h-8 items-center gap-1 rounded-[var(--radius-button)] border border-[var(--color-rule)] bg-[var(--color-surface)] px-3 text-xs font-semibold shadow-[var(--shadow-card)] disabled:opacity-50"><MoreHorizontal className="h-3.5 w-3.5" />Actions</button>
    {activeOperation && <span className="ml-1 inline-flex items-center gap-1 text-xs"><span className="spinner" aria-hidden="true" />{operationBusyLabels[activeOperation]}</span>}
    {open && <div role="menu" className="absolute right-0 z-10 mt-1 grid min-w-[10rem] gap-0.5 rounded-[var(--radius-lg)] border border-[var(--color-rule)] bg-[var(--color-surface)] p-1 shadow-[var(--shadow-lg)]">{ops.map(({op,icon:Icon,label})=> <button key={op} role="menuitem" type="button" disabled={!tool.can_install} onClick={()=>{ setOpen(false); void onAction(tool,op); }} className="flex items-center gap-2 rounded-[var(--radius-button)] px-2 py-1.5 text-left text-sm hover:bg-[var(--color-surface-muted)] disabled:opacity-50"><Icon className="h-3.5 w-3.5" />{label}</button>)}</div>}
  </div>;
}

function CategoryAccordion({ category, tools, busy, onAction, tableClassName = "tool-table" }: { category: AnalyzerCategory; tools: Tool[]; busy?: BusyAction; onAction: (tool: Tool, op: ToolOperation) => void; tableClassName?: string }) {
  const [open, setOpen] = useState(true);
  const meta = ANALYZER_CATALOG.filter((a) => a.category === category);
  const ready = tools.filter((t) => t.ready).length;
  const Icon = categoryIcons[category] ?? Wrench;
  return (
    <details open={open} onToggle={(e) => setOpen((e.target as HTMLDetailsElement).open)} className="tool-category border rounded-lg bg-[var(--color-surface)] mb-3">
      <summary className="flex items-center gap-2 px-4 py-3 cursor-pointer list-none">
        <span className="flex h-7 w-7 items-center justify-center rounded-md border" style={{ background: `color-mix(in oklch, ${categoryColor(category)} 16%, var(--color-surface))`, color: categoryColor(category), borderColor: `color-mix(in oklch, ${categoryColor(category)} 34%, var(--color-rule))` }}><Icon className="h-4 w-4" /></span>
        <strong>{CATEGORY_LABELS[category]}</strong>
        <span className="badge ml-2">{ready} of {tools.length} ready</span>
        <span className="ml-auto text-xs text-[var(--color-ink-faint)]">{open ? 'Hide' : 'Show'}</span>
      </summary>
      <div className="px-2 pb-3">
        <div className={`${tableClassName} table-wrap`}><table><thead><tr><th scope="col">Tool</th><th scope="col">Version</th><th scope="col">Status</th><th scope="col">Languages</th><th scope="col">Details</th><th scope="col">Actions</th></tr></thead><tbody>{tools.map((tool) => {
          const activeOperation = busy && busy.tool === tool.id ? busy.operation : undefined;
          const metaEntry = analyzerMeta(tool.id);
          const detail = tool.detail ?? (tool.ready ? 'Ready for local analysis.' : 'Coming soon — managed install');
          return <tr key={tool.id}><td><strong className="flex items-center gap-1.5">{tool.name ?? tool.id}{metaEntry && <span className="badge text-[10px]" style={{ borderColor: categoryColor(metaEntry.category), color: categoryColor(metaEntry.category) }}>{metaEntry.category}</span>}</strong></td><td><span className="badge tool-version">{tool.version ? `v${tool.version}` : 'Managed version'}</span></td><td><span className={`state ${tool.ready ? 'ready' : 'not-ready'}`}>{tool.ready ? 'Ready' : 'Not installed'}</span></td><td><span className="flex flex-wrap gap-1">{(metaEntry?.languages.slice(0, 3) ?? []).map((l) => <span key={l} className="badge text-[10px]">{l}</span>)}</span></td><td className="text-xs max-w-[18rem] truncate" title={detail}>{detail}</td><td className="table-actions" aria-busy={activeOperation ? true : undefined}>
            <span className="flex gap-1">{(['install', 'repair', 'update'] as const).map((operation) => <button type="button" key={operation} className="text-button" disabled={!tool.can_install || activeOperation !== undefined} onClick={() => void onAction(tool, operation)}>{activeOperation === operation ? <><span className="spinner" aria-hidden="true" />{operationBusyLabels[operation]}</> : operationLabels[operation]}</button>)}</span>
          </td></tr>;
        })}</tbody></table></div>
      </div>
    </details>
  );
}

export function ToolsPage({ notify, go }: { notify: (n: Notice) => void; go?: (r: Route) => void }) {
  const tools = useLoad(api.tools, []);
  const [busy, setBusy] = useState<BusyAction>();
  async function action(tool: Tool, operation: ToolOperation) { setBusy({ tool: tool.id, operation }); try { await api.toolAction(tool.id, operation); await tools.reload(); notify({ kind: 'info', text: `${tool.name?.trim() || tool.id}: ${operationVerbs[operation]}.` }); } catch (e) { notify({ kind: 'error', text: message(e) }); } finally { setBusy(undefined); } }

  const backend = tools.data ?? [];

  const grouped = useMemo(() => {
    const map = new Map<AnalyzerCategory, Tool[]>();
    for (const tool of backend) {
      const cat = analyzerMeta(tool.id)?.category ?? 'security' as AnalyzerCategory;
      const arr = map.get(cat) ?? [];
      arr.push(tool);
      map.set(cat, arr);
    }
    return map;
  }, [backend]);

  const builtIns = useMemo(() => ANALYZER_CATALOG.filter((a) => BUILT_IN_IDS.has(a.id)), []);

  const placeholderGrouped = useMemo(() => {
    const map = new Map<AnalyzerCategory, Tool[]>();
    const ids = new Set(backend.map((t) => t.id));
    for (const a of ANALYZER_CATALOG.filter((x) => !ids.has(x.id) && !BUILT_IN_IDS.has(x.id))) {
      const tool: Tool = { id: a.id, name: a.displayName, ready: false, can_install: false, detail: 'Coming soon — managed install' };
      const cat = a.category;
      const arr = map.get(cat) ?? [];
      arr.push(tool);
      map.set(cat, arr);
    }
    return map;
  }, [backend]);

  return (
    <div className="page">
      <PageHeader
        eyebrow="Managed tools"
        title="Analysis tools"
        description="Blunt Code keeps tool setup private and local. Install or repair only when needed."
      />
      {tools.loading ? <SkeletonTable rows={4} cols={5} className="tool-table" /> : tools.error ? <ErrorPanel error={tools.error} retry={tools.reload} /> : !backend.length ? <Empty title="No managed tools" icon={<WrenchIcon />}>Tool status appears here after the backend registers analyzers.</Empty> : <><ReadinessStrip tools={backend} busy={busy} />
    {[...grouped.entries()].map(([cat, list]) => <CategoryAccordion key={cat} category={cat} tools={list} busy={busy} onAction={action} />)}
    {builtIns.length > 0 && <section className="builtin-analyzers" aria-label="Built-in analyzers"><h3 className="text-sm font-semibold mt-4">Built-in analyzers</h3><p className="text-xs text-[var(--color-ink-soft)]">Bundled in-process — nothing to install. They run on standard and deep scans (skipped in offline mode).</p>
      <div className="builtin-table table-wrap"><table><thead><tr><th scope="col">Tool</th><th scope="col">Status</th><th scope="col">Languages</th><th scope="col">Details</th></tr></thead><tbody>{builtIns.map((a) => <tr key={a.id}><td><strong className="flex items-center gap-1.5">{a.displayName}<span className="badge text-[10px]" style={{ borderColor: categoryColor(a.category), color: categoryColor(a.category) }}>{a.category}</span></strong></td><td><span className="state ready">Built-in</span></td><td><span className="flex flex-wrap gap-1">{a.languages.slice(0, 3).map((l) => <span key={l} className="badge text-[10px]">{l}</span>)}<span className="badge text-[10px]">+{a.languages.length - 3} more</span></span></td><td className="text-xs max-w-[18rem] truncate" title={a.description}>{a.description}</td></tr>)}</tbody></table></div>
    </section>}
      <LanguageCoverage />
      <PentestSection go={go} />
    </>}
  </div>
  );
}
