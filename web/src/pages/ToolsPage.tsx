import { Fragment, useState } from 'react';
import '../css/tools.css';
import { api } from '../api';
import type { AnalyzerStatus } from '../types';
import type { Notice } from '../lib/notice';
import { message } from '../lib/notice';
import { useLoad } from '../hooks/useLoad';
import { Empty, ErrorPanel } from '../components/ui';
import { WrenchIcon } from '../components/icons';
import { SkeletonTable } from '../components/skeletons';
import { ANALYZER_CATALOG } from '../lib/analyzerCatalog';
import { LanguageCoverage } from '../components/LanguageCoverage';
import { ConfirmationDialog } from '../components/dialogs';
import { RowMenu } from '../components/RowMenu';
import type { Route } from '../lib/router';
import { PageHeader } from '../components/PageHeader';
import { formatBytes } from '../lib/format';
import { ChevronDown } from 'lucide-react';

type ToolOperation = 'install' | 'repair' | 'update' | 'uninstall';
type BusyAction = { tool: string; operation: ToolOperation };
type PendingAction = { analyzer: AnalyzerStatus; operation: ToolOperation };
type StatusFilter = 'all' | 'ready' | 'setup';

const operationLabels: Record<ToolOperation, string> = { install: 'Install', repair: 'Repair', update: 'Update', uninstall: 'Uninstall' };
const operationBusyLabels: Record<ToolOperation, string> = { install: 'Installing…', repair: 'Repairing…', update: 'Updating…', uninstall: 'Uninstalling…' };
const operationVerbs: Record<ToolOperation, string> = { install: 'installed', repair: 'repaired', update: 'updated', uninstall: 'uninstalled' };

/** Confirm-dialog copy naming the tool + operation, so a long re-download
 *  never starts from a single mis-click. */
const operationConfirmCopy: Record<ToolOperation, (name: string, diskBytes?: number) => string> = {
  install: (name) => `Install downloads and sets up ${name} on this machine.`,
  repair: (name) => `Repair re-runs setup for ${name}, re-downloading the managed tool if needed.`,
  update: (name) => `Update fetches and applies the latest managed ${name} release.`,
  uninstall: (name, bytes) => bytes
    ? `Uninstall removes ${name} and its downloaded files from this machine, freeing ${formatBytes(bytes)} of disk space. You can reinstall it anytime.`
    : `Uninstall removes ${name} and its downloaded files from this machine, freeing disk space. You can reinstall it anytime.`,
};

/** The backend capability inventory's category vocabulary (internal/analyzers)
 *  with human labels; unknown values fall back to the raw string. */
const API_CATEGORY_LABELS: Record<string, string> = {
  'code-quality': 'Code quality',
  compliance: 'Compliance',
  dependencies: 'Dependencies',
  infrastructure: 'Infrastructure',
  maintainability: 'Maintainability',
  secrets: 'Secrets',
  security: 'Security',
};

const NETWORK_LABELS: Record<AnalyzerStatus['network'], string> = {
  none: 'No network',
  outbound: 'Outbound',
  'loopback-only': 'Loopback only',
};

const PROFILE_LABELS: Record<string, string> = { quick: 'Quick', standard: 'Standard', deep: 'Deep', pentest: 'Pentest' };

const FILTER_LABELS: Record<StatusFilter, string> = { all: 'All', ready: 'Ready', setup: 'Needs setup' };

/** Status cell: external tools are Ready/Not installed, in-process analyzers are
 *  Ready — or Offline mode when offline mode withheld them at startup. The tone
 *  carries the chroma contract: "ready" is the norm and stays quiet; only
 *  not-ready states get color (danger = needs install, warning = withheld). */
function statusLabel(analyzer: AnalyzerStatus): { text: string; tone: 'ok' | 'danger' | 'warning' } {
  if (analyzer.execution === 'in-process') {
    return analyzer.registered ? { text: 'Ready', tone: 'ok' } : { text: 'Offline mode', tone: 'warning' };
  }
  return analyzer.ready ? { text: 'Ready', tone: 'ok' } : { text: 'Not installed', tone: 'danger' };
}

function statusStateClass(tone: 'ok' | 'danger' | 'warning'): string {
  return tone === 'ok' ? 'tools-state' : tone === 'danger' ? 'tools-state blocked' : 'tools-state warn';
}

/** Only not-ready states earn a dot — a green dot on every Ready row is wallpaper. */
function statusDotClass(tone: 'danger' | 'warning'): string {
  return tone === 'danger' ? 'dot not-ready' : 'dot warn';
}

function matchesFilter(analyzer: AnalyzerStatus, filter: StatusFilter): boolean {
  if (filter === 'ready') return statusLabel(analyzer).tone === 'ok';
  if (filter === 'setup') return statusLabel(analyzer).tone !== 'ok';
  return true;
}

/** Languages are secondary detail: a long list collapses to a count so the
 *  details row never grows a second table. */
function languagesText(analyzer: AnalyzerStatus): string {
  return analyzer.languages.length > 8 ? `${analyzer.languages.length} languages` : analyzer.languages.join(', ');
}

export function ToolsPage({ notify, go }: { notify: (n: Notice) => void; go?: (r: Route) => void }) {
  const analyzers = useLoad(api.analyzers, []);
  const [busy, setBusy] = useState<BusyAction>();
  const [pending, setPending] = useState<PendingAction>();
  const [filter, setFilter] = useState<StatusFilter>('all');
  const [expanded, setExpanded] = useState<string>();

  async function action(analyzer: AnalyzerStatus, operation: ToolOperation) {
    if (!analyzer.managed_tool) return;
    setBusy({ tool: analyzer.managed_tool, operation });
    try {
      if (operation === 'uninstall') {
        await api.uninstallTool(analyzer.managed_tool);
      } else {
        await api.toolAction(analyzer.managed_tool, operation);
      }
      await analyzers.reload();
      notify({ kind: 'info', text: `${analyzer.display_name || analyzer.id}: ${operationVerbs[operation]}.` });
    } catch (e) {
      notify({ kind: 'error', text: message(e) });
    } finally {
      setBusy(undefined);
    }
  }

  const rows = analyzers.data ?? [];
  const managed = rows.filter((a) => a.managed_tool);
  const readyManaged = managed.filter((a) => a.ready).length;
  const managedAllReady = managed.length > 0 && readyManaged === managed.length;
  const filtered = rows.filter((a) => matchesFilter(a, filter));
  const pendingName = pending ? pending.analyzer.display_name || pending.analyzer.id : '';

  return (
    <div className="page">
      <PageHeader
        eyebrow="Analyzers"
        title="Analysis tools"
        description="Every analyzer registered on this machine — category, profiles, and local status. Pentest scans run per workspace: open one and choose Pentest suite."
      />
      {analyzers.error ? <ErrorPanel error={analyzers.error} retry={analyzers.reload} /> : <>
        <div className="toolbar-row tools-toolbar">
          <div className="toolbar-filters">
            <div className="segmented" role="group" aria-label="Filter analyzers by status">
              {(['all', 'ready', 'setup'] as const).map((f) => (
                <button key={f} type="button" aria-pressed={filter === f} disabled={analyzers.loading} onClick={() => setFilter(f)}>{FILTER_LABELS[f]}</button>
              ))}
            </div>
          </div>
          <span className="tools-readiness toolbar-meta" role="status">
            {analyzers.loading ? '… analyzers' : <>
              <span className="tools-count tabular-nums">{filter === 'all' ? `${rows.length} analyzers` : `${filtered.length} of ${rows.length} analyzers`}</span>
              {managed.length > 0 && <span className={managedAllReady ? 'tools-all-ready' : undefined}>{readyManaged} of {managed.length} optional tools ready</span>}
            </>}
          </span>
        </div>
        {analyzers.loading ? <SkeletonTable rows={ANALYZER_CATALOG.length} cols={5} className="tool-table" /> : !rows.length ? <Empty title="Nothing to set up" icon={<WrenchIcon />}>Analyzers appear automatically. Optional tools can be installed from here.</Empty> : <div className="tool-table table-wrap">
          <table>
            <thead>
              <tr>
                <th scope="col">Tool</th>
                <th scope="col">Version</th>
                <th scope="col">Status</th>
                <th scope="col">Source</th>
                <th scope="col">Actions</th>
              </tr>
            </thead>
            <tbody>
              {filtered.map((analyzer) => {
                const activeOperation = busy && busy.tool === analyzer.managed_tool ? busy.operation : undefined;
                const status = statusLabel(analyzer);
                // The inventory always sends display_name, but a hostile or
                // older payload must degrade to the id, never render "undefined".
                const name = analyzer.display_name || analyzer.id;
                const open = expanded === analyzer.id;
                const detailsId = `tools-details-${analyzer.id}`;
                return (
                  <Fragment key={analyzer.id}>
                    <tr>
                      <td>
                        <button type="button" className="tools-name" aria-expanded={open} {...(open ? { 'aria-controls': detailsId } : {})} onClick={() => setExpanded(open ? undefined : analyzer.id)}>
                          {name}
                          <ChevronDown className="tools-caret" aria-hidden="true" />
                        </button>
                      </td>
                      <td className="tools-version" title={analyzer.version ? `v${analyzer.version}` : undefined}>{analyzer.version ? `v${analyzer.version}` : '—'}</td>
                      <td><span className={statusStateClass(status.tone)}>{status.tone !== 'ok' && <i className={statusDotClass(status.tone)} aria-hidden="true" />}{status.text}</span></td>
                      <td className="tools-source">{analyzer.execution === 'in-process' ? 'Built-in' : 'Managed'}</td>
                      <td className="table-actions" aria-busy={activeOperation ? true : undefined}>
                        {analyzer.managed_tool ? (activeOperation
                          ? <span className="tools-busy"><span className="spinner" aria-hidden="true" />{operationBusyLabels[activeOperation]}</span>
                          : <>
                            {!analyzer.ready && <button type="button" className="button secondary tools-install" onClick={() => setPending({ analyzer, operation: 'install' })}>Install</button>}
                            <RowMenu
                              label={`Actions for ${name}`}
                              items={[
                                ...(analyzer.ready ? [
                                  { label: operationLabels.update, onSelect: () => setPending({ analyzer, operation: 'update' }) },
                                  { label: operationLabels.uninstall, onSelect: () => setPending({ analyzer, operation: 'uninstall' }) },
                                ] : []),
                                { label: operationLabels.repair, onSelect: () => setPending({ analyzer, operation: 'repair' }) },
                              ]}
                            />
                          </>) : <span className="tools-none" aria-hidden="true">—</span>}
                      </td>
                    </tr>
                    {open && (
                      <tr className="tools-details-row">
                        <td colSpan={5} id={detailsId}>
                          <div className="tools-details">
                            <p className="tools-detail-desc">{analyzer.detail || analyzer.description}</p>
                            <dl className="tools-facts">
                              <div><dt>Category</dt><dd>{API_CATEGORY_LABELS[analyzer.category] ?? analyzer.category}</dd></div>
                              {analyzer.disk_bytes ? <div><dt>Disk usage</dt><dd>{formatBytes(analyzer.disk_bytes)}</dd></div> : null}
                              <div><dt>Profiles</dt><dd>{(analyzer.profiles ?? []).map((p) => PROFILE_LABELS[p] ?? p).join(', ') || '—'}</dd></div>
                              <div><dt>Network</dt><dd>{NETWORK_LABELS[analyzer.network] ?? analyzer.network}{analyzer.network_note ? <small>{analyzer.network_note}</small> : null}</dd></div>
                              <div><dt>Languages</dt><dd>{languagesText(analyzer)}</dd></div>
                            </dl>
                          </div>
                        </td>
                      </tr>
                    )}
                  </Fragment>
                );
              })}
            </tbody>
          </table>
        </div>}
        <LanguageCoverage />
        {pending && <ConfirmationDialog
          tone={pending.operation === 'uninstall' ? 'destructive' : 'primary'}
          title={`${operationLabels[pending.operation]} ${pendingName}`}
          description={operationConfirmCopy[pending.operation](pendingName, pending.analyzer.disk_bytes)}
          confirmLabel={operationLabels[pending.operation]}
          busy={false}
          onCancel={() => setPending(undefined)}
          onConfirm={() => { const { analyzer, operation } = pending; setPending(undefined); void action(analyzer, operation); }}
        />}
      </>}
    </div>
  );
}
