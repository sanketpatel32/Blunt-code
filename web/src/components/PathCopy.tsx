import { useEffect, useRef, useState } from 'react';
import { Check, Copy } from 'lucide-react';
import { copyToClipboard } from '../lib/clipboard';
import { shortPath } from '../lib/format';

/**
 * A workspace path for tight surfaces (dashboard rows, workspace cards): the
 * short tail is shown, and a small button copies the FULL path — whole Windows
 * paths don't fit those rows, and hover-only truncation hides the meaningful
 * end instead of the noise. Hovering the short form still reveals the whole
 * path via the title attribute.
 */
export function PathCopy({ path }: { path: string }) {
  const [copied, setCopied] = useState(false);
  const timer = useRef<number | undefined>(undefined);
  useEffect(() => () => { window.clearTimeout(timer.current); }, []);
  const copy = async () => {
    if (!(await copyToClipboard(path))) return;
    setCopied(true);
    window.clearTimeout(timer.current);
    timer.current = window.setTimeout(() => setCopied(false), 2000);
  };
  return <span className="path-copy">
    <code title={path}>{shortPath(path)}</code>
    <button type="button" className={`path-copy-button${copied ? ' copied' : ''}`} onClick={() => void copy()} aria-label={copied ? 'Copied to clipboard' : 'Copy full path'} title={copied ? 'Copied' : 'Copy full path'}>
      {copied ? <Check size={12} aria-hidden /> : <Copy size={12} aria-hidden />}
    </button>
  </span>;
}
