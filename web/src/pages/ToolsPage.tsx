import { useState } from 'react';
import { api } from '../api';
import type { Tool } from '../types';
import type { Notice } from '../lib/notice';
import { message } from '../lib/notice';
import { useLoad } from '../hooks/useLoad';
import { ErrorPanel, Loading } from '../components/ui';

export function ToolsPage({ notify }: { notify: (n: Notice) => void }) {
  const tools = useLoad(api.tools, []); const [busy, setBusy] = useState<string>();
  async function action(tool: Tool, operation: 'install' | 'repair' | 'update') { setBusy(`${tool.id}-${operation}`); try { await api.toolAction(tool.id, operation); await tools.reload(); const verb = operation === 'install' ? 'installed' : operation === 'repair' ? 'repaired' : 'updated'; notify({ kind: 'info', text: `${tool.name?.trim() || tool.id}: ${verb}.` }); } catch (e) { notify({ kind: 'error', text: message(e) }); } finally { setBusy(undefined); } }
  return <div className="page"><header className="page-heading"><div><p className="eyebrow">Managed tools</p><h1>Analysis tools</h1><p>Blunt Code keeps tool setup private and local. Install or repair only when needed.</p></div></header>{tools.loading ? <Loading /> : tools.error ? <ErrorPanel error={tools.error} retry={tools.reload} /> : <div className="tool-table table-wrap"><table><thead><tr><th scope="col">Tool</th><th scope="col">Version</th><th scope="col">Status</th><th scope="col">Details</th><th scope="col">Actions</th></tr></thead><tbody>{tools.data?.map((tool) => <tr key={tool.id}><td><strong>{tool.name ?? tool.id}</strong></td><td><code>{tool.version ?? 'Managed version'}</code></td><td><span className={`state ${tool.ready ? 'ready' : 'not-ready'}`}>{tool.ready ? 'Ready' : 'Not installed'}</span></td><td>{tool.detail ?? 'Ready for local analysis.'}</td><td className="table-actions"><button className="text-button" disabled={!tool.can_install || busy === `${tool.id}-install`} onClick={() => void action(tool, 'install')}>Install</button><button className="text-button" disabled={!tool.can_install || busy === `${tool.id}-repair`} onClick={() => void action(tool, 'repair')}>Repair</button><button className="text-button" disabled={!tool.can_install || busy === `${tool.id}-update`} onClick={() => void action(tool, 'update')}>Update</button></td></tr>)}</tbody></table></div>}</div>;
}
