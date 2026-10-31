import { useEffect, useState } from 'react';
import { api } from '../../api';
import type { Finding } from '../../types';
import { message } from '../../lib/notice';
import { SEVERITY_LABELS, analyzerName, findingLocation, friendlyFindingTitle, ruleDocsUrl } from '../../lib/format';
import { copyToClipboard } from '../../lib/clipboard';
import { useLoad } from '../../hooks/useLoad';
import { SkeletonLines } from '../../components/skeletons';
import { CommentsPanel } from '../../components/CommentsPanel';

/** Friendly copy for the preview endpoint's error codes; anything else keeps the
 *  server message. useLoad hands errors over as strings ("CODE: message"), so the
 *  codes are matched by prefix rather than a code field. */
function previewErrorText(error: unknown): string {
  const text = typeof error === 'string' ? error : message(error);
  if (text.startsWith('SOURCE_FILE_TOO_LARGE')) return 'This source file is larger than 1 MB, so Blunt Code will not load it into the preview. Open it in your editor instead.';
  if (text.startsWith('SOURCE_FILE_NOT_FOUND')) return 'The file moved or was deleted after this scan ran, so there is nothing to preview yet.';
  if (text.startsWith('SOURCE_PATH_UNAVAILABLE')) return 'This finding has no file location — it was reported at the project level.';
  if (text.startsWith('SOURCE_NOT_A_FILE')) return 'Only source files can be previewed.';
  if (text.startsWith('FINDING_NOT_FOUND')) return 'This finding is no longer in the report, so there is nothing to preview.';
  return text;
}

/** vscode://file link for a finding, or undefined when the workspace root or
 *  file path is unknown. Windows roots keep their drive letter (C:/…). */
export function editorUrl(root: string | undefined, finding: Finding): string | undefined {
  if (!root || !finding.relative_path) return undefined;
  const base = root.replace(/\\/g, '/').replace(/\/+$/, '');
  const rel = finding.relative_path.replace(/\\/g, '/').replace(/^\/+/, '');
  const path = `${base}/${rel}`.replace(/^\/*/, '/');
  const line = finding.start_line ? `:${finding.start_line}` : '';
  return `vscode://file${encodeURI(path)}${line}`;
}

/**
 * Finding detail for the analysis layout: what the problem is (title, rule,
 * message, remediation) first, then the code with the highlighted range, then
 * the triage actions. On very wide screens it docks beside the list; below
 * 100rem it is a drawer over the right edge so every table column stays
 * visible; on phones it is a full-height sheet (see styles.css "Source pane").
 */
export function SourcePane({
  scanId,
  finding,
  workspaceId,
  onClose,
  onPrev,
  onNext,
  hasPrev,
  hasNext,
  onSuppress,
  onRestore,
  workspaceRoot,
}: {
  scanId: string;
  finding: Finding;
  workspaceId: string;
  onClose: () => void;
  onPrev: () => void;
  onNext: () => void;
  hasPrev: boolean;
  hasNext: boolean;
  onSuppress: (finding: Finding) => void;
  onRestore: (finding: Finding) => void;
  /** Absolute workspace folder, when known, for the "Open in editor" link. */
  workspaceRoot?: string;
}) {
  const preview = useLoad(() => api.findingPreview(scanId, finding.id), [scanId, finding.id]);
  const [copied, setCopied] = useState(false);
  const [commentsOpen, setCommentsOpen] = useState(false);
  useEffect(() => { setCommentsOpen(false); }, [finding.id]);
  useEffect(() => {
    if (!copied) return;
    const timer = window.setTimeout(() => setCopied(false), 2000);
    return () => window.clearTimeout(timer);
  }, [copied]);
  /** Esc closes the pane wherever focus sits; arrows walk findings while focus is inside the pane.
   *  An open dialog (suppress, notes…) owns Escape though — closing the pane underneath it would
   *  dismiss the wrong layer, so skip and let the dialog's handler (which ignores events a pane
   *  has already handled) win instead. */
  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key !== 'Escape') return;
      const target = event.target instanceof Element ? event.target : null;
      const active = document.activeElement instanceof Element ? document.activeElement : null;
      if (target?.closest('dialog') || active?.closest('dialog')) return;
      event.preventDefault();
      onClose();
    };
    window.addEventListener('keydown', onKeyDown);
    return () => window.removeEventListener('keydown', onKeyDown);
  }, [onClose]);
  const copyLocation = async () => {
    if (await copyToClipboard(findingLocation(finding))) setCopied(true);
  };
  const data = preview.data;
  const suppressed = finding.status === 'suppressed';
  const canSuppress = Boolean(workspaceId && finding.fingerprint);
  // When the backend could only echo the rule id back as the title, the heading
  // is synthesized from the message so it reads like a sentence, not an id.
  const title = friendlyFindingTitle(finding);
  const docsUrl = ruleDocsUrl(finding);
  const openUrl = editorUrl(workspaceRoot, finding);
  return <>
  {/* biome-ignore lint/a11y/noStaticElementInteractions lint/a11y/useKeyWithClickEvents: phone-only (CSS) dimmer behind the full-height sheet; tapping it closes the sheet, keyboard users close with Esc or the × button. */}
  <div className="source-pane-backdrop" aria-hidden="true" onClick={onClose} />
  <aside className="source-pane pane-fade" aria-label={`Source for ${title}`}>
    <div className="source-pane-head">
      <button type="button" className="icon-button" onClick={onPrev} disabled={!hasPrev} aria-label="Previous finding" title="Previous finding (↑ in the list)">↑</button>
      <button type="button" className="icon-button" onClick={onNext} disabled={!hasNext} aria-label="Next finding" title="Next finding (↓ in the list)">↓</button>
      <code title={findingLocation(finding)}>{findingLocation(finding)}</code>
      <span className={`severity ${finding.severity}`}>{SEVERITY_LABELS[finding.severity] ?? finding.severity}</span>
      <button type="button" className="icon-button" onClick={onClose} aria-label="Close source pane" title="Close (Esc)">×</button>
    </div>
    <div className="source-pane-scroll">
      <div className="source-pane-context">
        <div className="context-row"><strong>{title}</strong>{finding.rule_id && finding.rule_id !== title && <code>{finding.rule_id}</code>}<span className="badge">{analyzerName(finding.analyzer_id)}</span>{finding.status && <span className={`status-text${suppressed ? ' suppressed' : ''}`}>{finding.status}</span>}</div>
        <p className="pane-message">{finding.message}</p>
        <p className="remediation">{finding.remediation || 'No remediation provided for this rule.'}</p>
        {docsUrl && <a href={docsUrl} target="_blank" rel="noreferrer" title="Opens the rule's documentation">Rule docs</a>}
      </div>
      <div className="source-pane-body">
        {preview.loading ? <SkeletonLines lines={8} /> : preview.error ? <div className="source-pane-error" role="note"><strong>Preview unavailable</strong>{previewErrorText(preview.error)}</div> : data ? <>
          <p className="source-provenance" role="note">{data.note ?? 'Current source near this finding.'}</p>
          <pre className="code-preview">{data.lines.map((line) => <code key={line.number} className={line.number >= (data.highlight_start_line ?? 0) && line.number <= (data.highlight_end_line ?? 0) ? 'highlight' : ''}><span aria-hidden="true">{line.number}</span>{line.text || ' '}</code>)}</pre>
        </> : null}
      </div>
    </div>
    <div className="source-pane-foot">
      {openUrl && <a className="button secondary open-in-editor" href={openUrl} title="Opens this line in VS Code">Open in editor</a>}
      <button type="button" className={`button secondary copy-location${copied ? ' copied' : ''}`} onClick={() => void copyLocation()}>{copied ? 'Copied' : 'Copy location'}</button>
      {canSuppress && (suppressed
        ? <button type="button" className="text-button restore-finding" onClick={() => onRestore(finding)}>Restore</button>
        : <button type="button" className="text-button suppress-finding" onClick={() => onSuppress(finding)}>Suppress…</button>)}
      <button type="button" className="text-button" aria-expanded={commentsOpen} onClick={() => setCommentsOpen((open) => !open)}>{commentsOpen ? 'Hide notes' : 'Notes'}</button>
    </div>
    {commentsOpen && <div className="source-pane-comments"><CommentsPanel key={`${workspaceId}:${finding.fingerprint ?? finding.id}`} workspaceId={workspaceId} fingerprint={finding.fingerprint ?? finding.id ?? 'unknown'} title={findingLocation(finding)} /></div>}
  </aside>
  </>;
}
