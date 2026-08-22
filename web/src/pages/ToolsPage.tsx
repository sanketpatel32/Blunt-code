import { useState } from 'react';
import { api } from '../api';
import type { Tool } from '../types';
import type { Notice } from '../lib/notice';
import { message } from '../lib/notice';
import { useLoad } from '../hooks/useLoad';
import { Empty, ErrorPanel } from '../components/ui';
import { WrenchIcon } from '../components/icons';
import { SkeletonTable } from '../components/skeletons';

type ToolOperation = 'install' | 'repair' | 'update';
type BusyAction = { tool: string; operation: ToolOperation };

const operationLabels: Record<ToolOperation, string> = { install: 'Install', repair: 'Repair', update: 'Update' };
const operationBusyLabels: Record<ToolOperation, string> = { install: 'Installing…', repair: 'Repairing…', update: 'Updating…' };
const operationVerbs: Record<ToolOperation, string> = { install: 'installed', repair: 'repaired', update: 'updated' };

export function ToolsPage({ notify }: { notify: (n: Notice) => void }) {
  const tools = useLoad(api.tools, []);
  const [busy, setBusy] = useState<BusyAction>();
  async function action(tool: Tool, operation: ToolOperation) { setBusy({ tool: tool.id, operation }); try { await api.toolAction(tool.id, operation); await tools.reload(); notify({ kind: 'info', text: `${tool.name?.trim() || tool.id}: ${operationVerbs[operation]}.` }); } catch (e) { notify({ kind: 'error', text: message(e) }); } finally { setBusy(undefined); } }
  return <div className="page"><header className="page-heading"><div><p className="eyebrow">Managed tools</p><h1>Analysis tools</h1><p>Blunt Code keeps tool setup private and local. Install or repair only when needed.</p></div></header>{tools.loading ? <SkeletonTable rows={4} cols={5} className="tool-table" /> : tools.error ? <ErrorPanel error={tools.error} retry={tools.reload} /> : !tools.data?.length ? <Empty title="No managed tools" icon={<WrenchIcon />}>Tool status appears here after the backend registers analyzers.</Empty> : <div className="tool-table table-wrap"><table><thead><tr><th scope="col">Tool</th><th scope="col">Version</th><th scope="col">Status</th><th scope="col">Details</th><th scope="col">Actions</th></tr></thead><tbody>{tools.data?.map((tool) => {
    const activeOperation = busy && busy.tool === tool.id ? busy.operation : undefined;
    return <tr key={tool.id}><td><strong>{tool.name ?? tool.id}</strong></td><td><code>{tool.version ?? 'Managed version'}</code></td><td><span className={`state ${tool.ready ? 'ready' : 'not-ready'}`}>{tool.ready ? 'Ready' : 'Not installed'}</span></td><td>{tool.detail ?? 'Ready for local analysis.'}</td><td className="table-actions" aria-busy={activeOperation ? true : undefined}>{(['install', 'repair', 'update'] as const).map((operation) => <button key={operation} className="text-button" disabled={!tool.can_install || activeOperation !== undefined} onClick={() => void action(tool, operation)}>{activeOperation === operation ? <><span className="spinner" aria-hidden="true" />{operationBusyLabels[operation]}</> : operationLabels[operation]}</button>)}</td></tr>;
  })}</tbody></table></div>}</div>;
}
