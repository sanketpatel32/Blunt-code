import { useState } from 'react';
import { Sparkles, Copy, Check, RefreshCw } from 'lucide-react';
import type { Finding } from '../types';
import { copyToClipboard } from '../lib/clipboard';
import { getAiFixSuggestion, regenerateAiFix, confidenceVariant } from '../lib/aiFix';
import { Card, CardHeader, CardTitle, CardDescription, CardContent, CardFooter } from './ui/card';
import { Button } from './ui/button';
import { Badge } from './ui/badge';

export function AutoFixPanel({ finding, onClose }: { finding: Finding; onClose?: () => void }) {
  const [suggestion, setSuggestion] = useState(() => getAiFixSuggestion(finding));
  const [copied, setCopied] = useState(false);

  const handleApply = async () => {
    if (await copyToClipboard(suggestion.fixed)) {
      setCopied(true);
      setTimeout(() => setCopied(false), 1800);
    }
  };

  const handleRegenerate = () => {
    setSuggestion(regenerateAiFix(finding, suggestion.id));
    setCopied(false);
  };

  const previewText =
    finding.message && finding.relative_path
      ? `${finding.relative_path}: ${finding.message}`
      : finding.message;

  return (
    <Card
      className="flex flex-col gap-0 rounded-[var(--radius-card)] border border-[var(--color-rule)] bg-[var(--color-surface)] shadow-[var(--shadow-card)] overflow-hidden"
      aria-label="AI auto-fix suggestion"
    >
      <CardHeader className="pb-3">
        <div className="flex items-start justify-between gap-2">
          <div className="flex items-center gap-2">
            <span
              className="flex h-7 w-7 items-center justify-center rounded-[var(--radius-sm)] bg-[var(--color-accent-soft)] text-[var(--color-accent-strong)] motion-safe:animate-in motion-reduce:animate-none"
              aria-hidden="true"
            >
              <Sparkles className="h-4 w-4" />
            </span>
            <CardTitle className="text-[15px]">AI Fix Suggestion</CardTitle>
          </div>
          <Badge variant={confidenceVariant(suggestion.confidence)} className="shrink-0 capitalize">
            {suggestion.confidence} confidence
          </Badge>
        </div>
        <CardDescription className="text-xs leading-relaxed">
          {suggestion.title} — <span className="font-mono text-[var(--color-ink-soft)]">{finding.rule_id ?? finding.category}</span>
        </CardDescription>
      </CardHeader>

      <CardContent className="flex flex-col gap-4 pt-0">
        {/* Original snippet from finding preview */}
        <div>
          <p className="mb-1.5 text-xs font-semibold tracking-wide text-[var(--color-ink-faint)] uppercase">Original</p>
          <pre className="max-h-28 overflow-auto rounded-[var(--radius-sm)] border border-[var(--color-rule-faint)] bg-[var(--color-surface-muted)] p-3 font-mono text-xs leading-relaxed text-[var(--color-ink)] whitespace-pre-wrap break-words">
            {previewText}
          </pre>
        </div>

        {/* Side-by-side diff: monaco-like pre blocks */}
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
          <div>
            <p className="mb-1.5 text-xs font-semibold tracking-wide text-[var(--color-ink-faint)] uppercase">Before</p>
            <pre className="max-h-48 overflow-auto rounded-[var(--radius-sm)] border border-[var(--color-danger)]/20 bg-[var(--color-danger-soft)] p-3 font-mono text-xs leading-relaxed text-[var(--color-ink)] whitespace-pre-wrap break-words">
              {suggestion.original}
            </pre>
          </div>
          <div>
            <p className="mb-1.5 text-xs font-semibold tracking-wide text-[var(--color-ink-faint)] uppercase">Suggested fix</p>
            <pre className="max-h-48 overflow-auto rounded-[var(--radius-sm)] border border-[var(--color-success)]/20 bg-[var(--color-success-soft)] p-3 font-mono text-xs leading-relaxed text-[var(--color-ink)] whitespace-pre-wrap break-words">
              {suggestion.fixed}
            </pre>
          </div>
        </div>

        <p className="rounded-[var(--radius-sm)] bg-[var(--color-surface-muted)] px-3 py-2 text-xs leading-relaxed text-[var(--color-ink-soft)]">
          {suggestion.explanation}
        </p>

        <p className="text-xs italic text-[var(--color-ink-faint)]" role="note">
          AI suggestion — review before applying
        </p>
      </CardContent>

      <CardFooter className="flex flex-wrap gap-2 border-t border-[var(--color-rule-faint)] bg-[var(--color-surface-muted)]/40 pt-4">
        <Button
          onClick={() => void handleApply()}
          className="gap-1.5 rounded-[var(--radius-button)]"
          aria-label={copied ? 'Copied to clipboard' : 'Copy fix to clipboard'}
        >
          {copied ? <Check className="h-4 w-4" aria-hidden="true" /> : <Copy className="h-4 w-4" aria-hidden="true" />}
          {copied ? 'Copied!' : 'Apply — copy to clipboard'}
        </Button>
        <Button variant="outline" onClick={handleRegenerate} className="gap-1.5 rounded-[var(--radius-button)]">
          <RefreshCw className="h-4 w-4" aria-hidden="true" />
          Regenerate
        </Button>
        {onClose && (
          <Button variant="ghost" onClick={onClose} className="rounded-[var(--radius-button)]">
            Close
          </Button>
        )}
      </CardFooter>
    </Card>
  );
}
