import { useEffect, useRef, useState } from 'react';
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
  if (text.startsWith('SOURCE_PATH_UNAVAILABLE')) return 'This finding has no file location \u2014 it was reported at the project level.';
  if (text.startsWith('SOURCE_NOT_A_FILE')) return 'Only source files can be previewed.';
  if (text.startsWith('FINDING_NOT_FOUND')) return 'This finding is no longer in the report, so there is nothing to preview.';
  return text;
}

/**
 * Docked source viewer for the analysis split layout: the finding's code with the
 * highlighted range, its context (rule, tool, status, remediation), and the
 * triage actions \u2014 all beside the still-scrollable findings list. Below 72rem it
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
  currentIndex,
  totalCount,
  expanded,
  onToggleExpand,
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
  currentIndex?: number;
  totalCount?: number;
  expanded?: boolean;
  onToggleExpand?: () => void;
}) {
  const preview = useLoad(() => api.findingPreview(scanId, finding.id), [scanId, finding.id]);
  const [copiedLocation, setCopiedLocation] = useState(false);
  const [copiedSnippet, setCopiedSnippet] = useState(false);
  const [copiedRule, setCopiedRule] = useState(false);
  const [commentsOpen, setCommentsOpen] = useState(false);
  const bodyRef = useRef<HTMLDivElement>(null);

  useEffect(() => { setCommentsOpen(false); }, [finding.id]);

  useEffect(() => {
    if (!copiedLocation) return;
    const timer = window.setTimeout(() => setCopiedLocation(false), 2000);
    return () => window.clearTimeout(timer);
  }, [copiedLocation]);

  useEffect(() => {
    if (!copiedSnippet) return;
    const timer = window.setTimeout(() => setCopiedSnippet(false), 2000);
    return () => window.clearTimeout(timer);
  }, [copiedSnippet]);

  useEffect(() => {
    if (!copiedRule) return;
    const timer = window.setTimeout(() => setCopiedRule(false), 2000);
    return () => window.clearTimeout(timer);
  }, [copiedRule]);

  /** Auto-scroll to the highlighted lines when preview data loads */
  useEffect(() => {
    if (!preview.data || !bodyRef.current) return;
    const highlight = bodyRef.current.querySelector('code.highlight');
    if (highlight && typeof highlight.scrollIntoView === 'function') {
      highlight.scrollIntoView({ block: 'center', behavior: 'smooth' });
    }
  }, [preview.data, finding.id]);

  /** Esc closes the pane; j/k and arrows walk findings while focus is inside the pane.
   *  An open dialog (suppress, notes\u2026) owns Escape though. Text inputs/textareas
   *  are left alone so typing notes never walks findings. */
  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      const target = event.target instanceof Element ? event.target : null;
      const active = document.activeElement instanceof Element ? document.activeElement : null;
      if (target?.closest('dialog') || active?.closest('dialog')) return;

      if (event.key === 'Escape') {
        event.preventDefault();
        onClose();
        return;
      }

      if (target?.tagName === 'INPUT' || target?.tagName === 'TEXTAREA' || (target instanceof HTMLElement && target.isContentEditable)) {
        return;
      }

      if (event.key === 'ArrowDown' || event.key === 'j') {
        if (hasNext) {
          event.preventDefault();
          onNext();
        }
      } else if (event.key === 'ArrowUp' || event.key === 'k') {
        if (hasPrev) {
          event.preventDefault();
          onPrev();
        }
      }
    };
    window.addEventListener('keydown', onKeyDown);
    return () => window.removeEventListener('keydown', onKeyDown);
  }, [onClose, onNext, onPrev, hasNext, hasPrev]);

  const copyLocation = async () => {
    if (await copyToClipboard(findingLocation(finding))) setCopiedLocation(true);
  };

  const copySnippet = async () => {
    if (!data?.lines?.length) return;
    const start = data.highlight_start_line ?? 0;
    const end = data.highlight_end_line ?? 0;
    const targetLines = (start > 0 && end >= start)
      ? data.lines.filter((l) => l.number >= start && l.number <= end)
      : data.lines;
    const text = targetLines.map((l) => l.text).join('\n');
    if (await copyToClipboard(text)) setCopiedSnippet(true);
  };

  const copyRuleId = async () => {
    if (finding.rule_id && (await copyToClipboard(finding.rule_id))) {
      setCopiedRule(true);
    }
  };

  const data = preview.data;
  const suppressed = finding.status === 'suppressed';
  const canSuppress = Boolean(workspaceId && finding.fingerprint);
  // When the backend could only echo the rule id back as the title, the heading
  // is synthesized from the message so it reads like a sentence, not an id.
  const title = friendlyFindingTitle(finding);
  const docsUrl = ruleDocsUrl(finding);

  return <aside className={`source-pane pane-fade${expanded ? ' expanded' : ''}`} aria-label={`Source for ${title}`}>
    <div className="source-pane-head">
      <button type="button" className="icon-button" onClick={onPrev} disabled={!hasPrev} aria-label="Previous finding" title="Previous finding (↑ or k in list)">↑</button>
      <button type="button" className="icon-button" onClick={onNext} disabled={!hasNext} aria-label="Next finding" title="Next finding (↓ or j in list)">↓</button>
      {currentIndex !== undefined && totalCount !== undefined ? (
        <span className="source-pane-counter" aria-label={`Finding ${currentIndex + 1} of ${totalCount}`} title={`Finding ${currentIndex + 1} of ${totalCount}`}>
          {currentIndex + 1}/{totalCount}
        </span>
      ) : null}
      <code title={findingLocation(finding)}>{findingLocation(finding)}</code>
      <span className={`severity ${finding.severity}`}>{SEVERITY_LABELS[finding.severity] ?? finding.severity}</span>
      {onToggleExpand && (
        <button
          type="button"
          className="icon-button pane-expand-toggle"
          onClick={onToggleExpand}
          aria-label={expanded ? 'Restore pane width' : 'Expand pane width'}
          title={expanded ? 'Restore pane width' : 'Expand pane width (wider view)'}
        >
          {expanded ? '⤡' : '⤢'}
        </button>
      )}
      <button type="button" className="icon-button" onClick={onClose} aria-label="Close source pane" title="Close (Esc)">×</button>
    </div>
    <div className="source-pane-body" ref={bodyRef}>
      {preview.loading ? <SkeletonLines lines={8} /> : preview.error ? <div className="source-pane-error" role="note"><strong>Preview unavailable</strong>{previewErrorText(preview.error)}</div> : data ? <>
        <p className="sr-only">{data.note ?? 'Current source near this finding.'}</p>
        <pre className="code-preview">{data.lines.map((line) => <code key={line.number} className={line.number >= (data.highlight_start_line ?? 0) && line.number <= (data.highlight_end_line ?? 0) ? 'highlight' : ''}><span aria-hidden="true">{line.number}</span>{line.text || ' '}</code>)}</pre>
      </> : null}
    </div>
    <div className="source-pane-context">
      <div className="context-row">
        <strong>{title}</strong>
        {finding.rule_id && finding.rule_id !== title && (
          <button
            type="button"
            className={`rule-chip-button${copiedRule ? ' copied' : ''}`}
            onClick={() => void copyRuleId()}
            title="Click to copy rule ID"
            aria-label={`Rule ${finding.rule_id} — click to copy`}
          >
            <code>{finding.rule_id}</code>
            {copiedRule ? <span className="copied-hint">Copied</span> : null}
          </button>
        )}
        <span className="badge">{analyzerName(finding.analyzer_id)}</span>
        {finding.status && <span className={`status-text${suppressed ? ' suppressed' : ''}`}>{finding.status}</span>}
      </div>
      <p className="pane-message">{finding.message}</p>
      {finding.remediation ? (
        <div className="remediation-box">
          <strong className="remediation-heading">Suggested remediation</strong>
          <p className="remediation">{finding.remediation}</p>
        </div>
      ) : (
        <p className="remediation">No remediation provided for this rule.</p>
      )}
      {docsUrl && <a href={docsUrl} target="_blank" rel="noreferrer" title="Opens the rule's documentation">Rule docs</a>}
    </div>
    <div className="source-pane-foot">
      <button type="button" className={`button secondary copy-location${copiedLocation ? ' copied' : ''}`} onClick={() => void copyLocation()}>{copiedLocation ? 'Copied' : 'Copy location'}</button>
      {data?.lines?.length ? (
        <button type="button" className={`button secondary copy-snippet${copiedSnippet ? ' copied' : ''}`} onClick={() => void copySnippet()}>
          {copiedSnippet ? 'Copied snippet' : 'Copy snippet'}
        </button>
      ) : null}
      {canSuppress && (suppressed
        ? <button type="button" className="text-button restore-finding" onClick={() => onRestore(finding)}>Restore</button>
        : <button type="button" className="text-button suppress-finding" onClick={() => onSuppress(finding)}>Suppress…</button>)}
      <button type="button" className="text-button" aria-expanded={commentsOpen} onClick={() => setCommentsOpen((open) => !open)}>{commentsOpen ? 'Hide notes' : 'Notes'}</button>
    </div>
    {commentsOpen && <div className="source-pane-comments"><CommentsPanel fingerprint={finding.fingerprint ?? finding.id ?? 'unknown'} title={findingLocation(finding)} /></div>}
  </aside>;
}
