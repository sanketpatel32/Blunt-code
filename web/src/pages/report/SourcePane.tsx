import { useEffect, useState } from 'react';
import { api } from '../../api';
import type { Finding } from '../../types';
import { message } from '../../lib/notice';
import { analyzerName, findingLocation } from '../../lib/format';
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
  return text;
}

/**
 * Docked source viewer for the analysis split layout: the finding's code with the
 * highlighted range, its context (rule, tool, status, remediation), and the
 * triage actions — all beside the still-scrollable findings list. Below 72rem it
 * renders as a bottom sheet over the list (see analysis.css).
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
  /** Esc closes the pane wherever focus sits; arrows walk findings while focus is inside the pane. */
  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') { event.preventDefault(); onClose(); }
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
  const title = finding.title ?? finding.rule_id ?? 'Finding';
  return <aside className="source-pane pane-fade" aria-label={`Source for ${title}`}>
    <div className="source-pane-head">
      <button type="button" className="icon-button" onClick={onPrev} disabled={!hasPrev} aria-label="Previous finding" title="Previous finding (↑ in the list)">↑</button>
      <button type="button" className="icon-button" onClick={onNext} disabled={!hasNext} aria-label="Next finding" title="Next finding (↓ in the list)">↓</button>
      <code title={findingLocation(finding)}>{findingLocation(finding)}</code>
      <span className={`severity ${finding.severity}`}>{finding.severity}</span>
      <button type="button" className="icon-button" onClick={onClose} aria-label="Close source pane" title="Close (Esc)">×</button>
    </div>
    <div className="source-pane-body">
      {preview.loading ? <SkeletonLines lines={8} /> : preview.error ? <div className="source-pane-error" role="note"><strong>Preview unavailable</strong>{previewErrorText(preview.error)}</div> : data ? <>
        <p className="sr-only">{data.note ?? 'Current source near this finding.'}</p>
        <pre className="code-preview">{data.lines.map((line) => <code key={line.number} className={line.number >= (data.highlight_start_line ?? 0) && line.number <= (data.highlight_end_line ?? 0) ? 'highlight' : ''}><span aria-hidden="true">{line.number}</span>{line.text || ' '}</code>)}</pre>
      </> : null}
    </div>
    <div className="source-pane-context">
      <div className="context-row"><strong>{title}</strong>{finding.rule_id && <code>{finding.rule_id}</code>}<span className="badge">{analyzerName(finding.analyzer_id)}</span>{finding.status && <span className={`status-text${suppressed ? ' suppressed' : ''}`}>{finding.status}</span>}</div>
      <p className="pane-message">{finding.message}</p>
      <p className="remediation">{finding.remediation || 'No remediation provided for this rule.'}</p>
      {finding.documentation_url && <a href={finding.documentation_url} target="_blank" rel="noreferrer">Rule docs</a>}
    </div>
    <div className="source-pane-foot">
      <button type="button" className={`button secondary copy-location${copied ? ' copied' : ''}`} onClick={() => void copyLocation()}>{copied ? 'Copied' : 'Copy location'}</button>
      {canSuppress && (suppressed
        ? <button type="button" className="text-button restore-finding" onClick={() => onRestore(finding)}>Restore</button>
        : <button type="button" className="text-button suppress-finding" onClick={() => onSuppress(finding)}>Suppress…</button>)}
      <button type="button" className="text-button" aria-expanded={commentsOpen} onClick={() => setCommentsOpen((open) => !open)}>{commentsOpen ? 'Hide notes' : 'Notes'}</button>
    </div>
    {commentsOpen && <div className="source-pane-comments"><CommentsPanel fingerprint={finding.fingerprint ?? finding.id ?? 'unknown'} title={findingLocation(finding)} /></div>}
  </aside>;
}
