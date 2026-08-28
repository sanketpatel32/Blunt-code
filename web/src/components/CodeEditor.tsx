import { useCallback, useEffect, useRef, useState } from 'react';
import { useReducedMotion } from '../hooks/useReducedMotion';

export type CodeEditorLanguage = 'yaml' | 'python' | 'javascript' | string;

export type CodeEditorProps = {
  value: string;
  onChange: (value: string) => void;
  language?: CodeEditorLanguage;
  ariaLabel?: string;
  ariaDescribedBy?: string;
  placeholder?: string;
  error?: string | null;
  onSave?: () => void;
  id?: string;
  minHeight?: string;
  readOnly?: boolean;
};

const YAML_KEYWORDS = new Set(['id', 'languages', 'pattern', 'severity', 'message']);
const PY_KEYWORDS = new Set(['def', 'class', 'import', 'from', 'return', 'if', 'else', 'elif', 'for', 'while', 'try', 'except', 'with', 'as', 'pass', 'raise', 'in', 'is', 'and', 'or', 'not', 'True', 'False', 'None', 'eval', 'exec']);
const JS_KEYWORDS = new Set(['function', 'const', 'let', 'var', 'return', 'if', 'else', 'for', 'while', 'import', 'from', 'export', 'class', 'new', 'try', 'catch', 'throw', 'async', 'await', 'eval', 'true', 'false', 'null', 'undefined']);

function tokenize(text: string, language: string) {
  const lines = text.split('\n');
  const tokens: Array<{ text: string; color: string }> = [];
  const lang = language.toLowerCase();

  for (const lineRaw of lines) {
    const line = lineRaw;
    // keep exact line + newline for overlay sync
    const withNl = line + '\n';

    if (line.trim() === '') {
      tokens.push({ text: withNl, color: 'var(--color-ink)' });
      continue;
    }

    if (lang === 'yaml') {
      const trimmed = line.trim();
      if (trimmed.startsWith('#')) {
        tokens.push({ text: withNl, color: 'var(--color-ink-faint)' });
        continue;
      }
      const idx = line.indexOf(':');
      if (idx === -1) {
        // invalid yaml line — still render but color danger faint? keep ink for fallback; error underline handled via border
        tokens.push({ text: withNl, color: 'var(--color-ink)' });
        continue;
      }
      const key = line.slice(0, idx).trim();
      const isKeyword = YAML_KEYWORDS.has(key);
      const keyPart = line.slice(0, idx);
      const rest = line.slice(idx);
      tokens.push({ text: keyPart, color: isKeyword ? 'var(--color-accent)' : 'var(--color-accent)' });
      // rest contains colon + value; strings get ink-soft
      // detect quoted string in rest
      const hasQuote = rest.includes('"') || rest.includes("'") || rest.includes('[');
      tokens.push({ text: rest + '\n', color: hasQuote ? 'var(--color-ink-soft)' : 'var(--color-ink-soft)' });
      continue;
    }

    // python / javascript generic highlighting
    // comment detection
    const trimmedStart = line.trimStart();
    const isComment = lang === 'python' ? trimmedStart.startsWith('#') : trimmedStart.startsWith('//');
    if (isComment) {
      tokens.push({ text: withNl, color: 'var(--color-ink-faint)' });
      continue;
    }

    // string detection — naive but token-colored
    // split by quoted segments
    // We render whole line as ink-soft if it contains a quote, otherwise check for keywords
    const keywords = lang === 'python' ? PY_KEYWORDS : lang === 'javascript' ? JS_KEYWORDS : new Set<string>();
    if (line.includes('"') || line.includes("'") || line.includes('`')) {
      // highlight strings: keep strings as ink-soft, keywords accent — simple whole-line soft fallback for readability
      // do per-word split to keep keywords accent where possible
      const parts = line.split(/("[^"]*"|'[^']*'|`[^`]*`)/g);
      for (const part of parts) {
        if (!part) continue;
        const isString = (part.startsWith('"') && part.endsWith('"')) || (part.startsWith("'") && part.endsWith("'")) || (part.startsWith('`') && part.endsWith('`'));
        if (isString) {
          tokens.push({ text: part, color: 'var(--color-ink-soft)' });
        } else {
          // keyword scan inside non-string fragment
          const words = part.split(/(\b\w+\b)/g);
          for (const w of words) {
            if (!w) continue;
            if (keywords.has(w)) tokens.push({ text: w, color: 'var(--color-accent)' });
            else tokens.push({ text: w, color: 'var(--color-ink)' });
          }
        }
      }
      tokens.push({ text: '\n', color: 'var(--color-ink)' });
      continue;
    }

    // no strings: keyword scan
    if (keywords.size) {
      const words = line.split(/(\b\w+\b)/g);
      for (const w of words) {
        if (!w) continue;
        if (keywords.has(w)) tokens.push({ text: w, color: 'var(--color-accent)' });
        else tokens.push({ text: w, color: 'var(--color-ink)' });
      }
      tokens.push({ text: '\n', color: 'var(--color-ink)' });
    } else {
      tokens.push({ text: withNl, color: 'var(--color-ink)' });
    }
  }
  return tokens;
}

export function CodeEditor({
  value,
  onChange,
  language = 'yaml',
  ariaLabel = 'Code editor',
  ariaDescribedBy,
  placeholder,
  error,
  onSave,
  id,
  minHeight = '14rem',
  readOnly = false,
}: CodeEditorProps) {
  const reduced = useReducedMotion();
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const highlightRef = useRef<HTMLPreElement>(null);
  const gutterRef = useRef<HTMLDivElement>(null);
  const [monacoReady, setMonacoReady] = useState(false);
  const monacoContainerRef = useRef<HTMLDivElement>(null);
  const monacoEditorRef = useRef<unknown>(null);

  // line numbers
  const lineCount = value === '' ? 1 : value.split('\n').length;
  const lineNumbers = Array.from({ length: lineCount }, (_, i) => String(i + 1));

  const highlighted = tokenize(value, language);

  const syncScroll = useCallback(() => {
    const ta = textareaRef.current;
    const hl = highlightRef.current;
    const gutter = gutterRef.current;
    if (!ta) return;
    if (hl) {
      hl.scrollTop = ta.scrollTop;
      hl.scrollLeft = ta.scrollLeft;
    }
    if (gutter) gutter.scrollTop = ta.scrollTop;
  }, []);

  // Dynamic monaco import — optional. Uses variable indirection + @vite-ignore so build does not hard-require the package.
  useEffect(() => {
    let cancelled = false;
    async function tryLoad() {
      // variable indirection prevents Vite static analysis
      const m1 = 'monaco-editor';
      try {
        // @ts-ignore — optional peer dep, may not be installed
        const mod: unknown = await import(/* @vite-ignore */ /* webpackIgnore: true */ m1);
        if (cancelled) return;
        if (mod && monacoContainerRef.current) setMonacoReady(true);
        return;
      } catch {
        // not available — stay on textarea fallback
      }
      const m2 = '@monaco-editor/react';
      if (!cancelled && !monacoReady) {
        try {
          // @ts-ignore
          const alt: unknown = await import(/* @vite-ignore */ /* webpackIgnore: true */ m2);
          if (cancelled) return;
          if (alt && monacoContainerRef.current) setMonacoReady(true);
        } catch {
          /* ignore */
        }
      }
    }
    void tryLoad();
    return () => {
      cancelled = true;
    };
  }, [monacoReady]);

  // If monaco becomes ready, mount a basic instance when available
  useEffect(() => {
    if (!monacoReady || !monacoContainerRef.current) return;
    let disposed = false;
    (async () => {
      try {
        const mp = 'monaco-editor';
        // @ts-ignore
        const monaco: { editor?: { create?: (el: HTMLElement, opts: unknown) => { dispose: () => void; onDidChangeModelContent?: (cb: () => void) => { dispose: () => void }; getValue?: () => string; setValue?: (v: string) => void } } } = await import(/* @vite-ignore */ /* webpackIgnore: true */ mp);
        if (disposed || !monacoContainerRef.current || !monaco.editor?.create) return;
        const container = monacoContainerRef.current;
        container.innerHTML = '';
        const instance = monaco.editor.create(container, {
          value,
          language: language === 'javascript' ? 'javascript' : language === 'python' ? 'python' : 'yaml',
          theme: 'vs',
          minimap: { enabled: false },
          fontSize: 12,
          fontFamily: 'var(--font-mono)',
          lineNumbers: 'on',
          tabSize: 2,
          insertSpaces: true,
          automaticLayout: true,
          scrollBeyondLastLine: false,
          wordWrap: 'on',
          readOnly,
        } as unknown);
        monacoEditorRef.current = instance;
        const maybe = instance as { onDidChangeModelContent?: (cb: () => void) => { dispose: () => void }; getValue?: () => string };
        if (maybe.onDidChangeModelContent && maybe.getValue) {
          maybe.onDidChangeModelContent(() => {
            const next = maybe.getValue?.() ?? '';
            if (next !== value) onChange(next);
          });
        }
      } catch {
        setMonacoReady(false);
      }
    })();
    return () => {
      disposed = true;
      const inst = monacoEditorRef.current as { dispose?: () => void } | null;
      if (inst?.dispose) inst.dispose();
      monacoEditorRef.current = null;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [monacoReady, language, readOnly]);

  function handleKeyDown(e: React.KeyboardEvent<HTMLTextAreaElement>) {
    // Tab -> 2 spaces
    if (e.key === 'Tab') {
      e.preventDefault();
      const ta = textareaRef.current;
      if (!ta) return;
      const start = ta.selectionStart;
      const end = ta.selectionEnd;
      const before = value.slice(0, start);
      const after = value.slice(end);
      const next = before + '  ' + after;
      onChange(next);
      // restore caret after state flush
      requestAnimationFrame(() => {
        if (!textareaRef.current) return;
        textareaRef.current.selectionStart = textareaRef.current.selectionEnd = start + 2;
      });
      return;
    }
    // Ctrl/Cmd+S -> save
    if ((e.ctrlKey || e.metaKey) && (e.key === 's' || e.key === 'S')) {
      e.preventDefault();
      onSave?.();
    }
  }

  const hasError = Boolean(error);
  const describedBy = [ariaDescribedBy, hasError ? `${id ?? 'code-editor'}-error` : null].filter(Boolean).join(' ') || undefined;

  if (monacoReady) {
    return (
      <div className="space-y-2">
        <div
          ref={monacoContainerRef}
          role="region"
          aria-label={ariaLabel}
          className="overflow-hidden rounded-[var(--radius-md)] border bg-[var(--color-surface)]"
          style={{
            minHeight,
            borderColor: hasError ? 'var(--color-danger)' : 'var(--color-rule)',
            boxShadow: hasError ? '0 0 0 2px var(--color-danger-soft)' : undefined,
          }}
        />
        {hasError && (
          <p id={`${id ?? 'code-editor'}-error`} role="alert" className="text-xs leading-5 text-[var(--color-danger)] underline decoration-wavy underline-offset-2" style={{ textDecorationColor: 'var(--color-danger)' }}>
            {error}
          </p>
        )}
      </div>
    );
  }

  return (
    <div className="space-y-2">
      <div
        className="flex overflow-hidden rounded-[var(--radius-md)] border bg-[var(--color-surface-muted)] focus-within:ring-2 focus-within:ring-[var(--color-focus)] focus-within:ring-offset-2"
        style={{
          borderColor: hasError ? 'var(--color-danger)' : 'var(--color-rule)',
          transition: reduced ? 'none' : 'border-color var(--dur-fast) var(--ease-out), box-shadow var(--dur-fast) var(--ease-out)',
          ...(hasError ? { boxShadow: '0 0 0 2px var(--color-danger-soft)' } : {}),
        }}
      >
        {/* line numbers gutter */}
        <div
          ref={gutterRef}
          aria-hidden="true"
          className="hidden sm:flex shrink-0 select-none flex-col items-end overflow-hidden border-r bg-[var(--color-surface-muted)] px-2 py-3 font-mono text-xs leading-5"
          style={{
            minWidth: '2.75rem',
            color: 'var(--color-ink-faint)',
            borderColor: 'var(--color-rule-faint)',
          }}
        >
          {lineNumbers.map((n) => (
            <span key={n} className="block text-right tabular-nums leading-5">
              {n}
            </span>
          ))}
        </div>

        {/* editor area */}
        <div className="relative flex-1 min-w-0">
          {/* syntax highlight layer */}
          <pre
            ref={highlightRef}
            aria-hidden="true"
            className="pointer-events-none absolute inset-0 overflow-hidden whitespace-pre-wrap break-words px-3 py-3 font-mono text-xs leading-5"
            style={{
              color: 'var(--color-ink)',
              background: 'transparent',
              margin: 0,
            }}
          >
            {value === '' && placeholder ? (
              <span style={{ color: 'var(--color-ink-faint)' }}>{placeholder}</span>
            ) : (
              highlighted.map((tok, i) => (
                <span key={i} style={{ color: tok.color }}>
                  {tok.text}
                </span>
              ))
            )}
            {hasError && (
              <span
                className="pointer-events-none absolute inset-x-3 bottom-1 block h-1"
                style={{
                  background: `repeating-linear-gradient(90deg, var(--color-danger) 0 4px, transparent 4px 8px)`,
                  opacity: 0.9,
                  borderRadius: 'var(--radius-xs)',
                }}
                aria-hidden="true"
              />
            )}
          </pre>

          <textarea
            ref={textareaRef}
            id={id}
            value={value}
            onChange={(e) => onChange(e.target.value)}
            onScroll={syncScroll}
            onKeyDown={handleKeyDown}
            spellCheck={false}
            autoComplete="off"
            autoCorrect="off"
            autoCapitalize="off"
            aria-label={ariaLabel}
            aria-invalid={hasError ? true : undefined}
            aria-describedby={describedBy}
            placeholder={placeholder}
            readOnly={readOnly}
            rows={10}
            className="relative min-h-0 w-full resize-y bg-transparent px-3 py-3 font-mono text-xs leading-5 text-transparent caret-[var(--color-ink)] placeholder:text-transparent focus:outline-none"
            style={{
              minHeight,
              caretColor: 'var(--color-ink)',
              // wavy underline for invalid YAML fallback — via text decoration when needed; keep transparent text so highlight shows
              textDecoration: hasError ? 'wavy underline' : undefined,
              textDecorationColor: hasError ? 'var(--color-danger)' : undefined,
              textUnderlineOffset: hasError ? '4px' : undefined,
            }}
          />
        </div>
      </div>

      {hasError ? (
        <p id={`${id ?? 'code-editor'}-error`} role="alert" className="text-xs leading-5 text-[var(--color-danger)]" style={{ textDecoration: 'wavy underline', textDecorationColor: 'var(--color-danger)', textUnderlineOffset: '3px' }}>
          {error}
        </p>
      ) : null}

      <p className="sr-only" aria-live="polite">
        {language} editor, {lineCount} {lineCount === 1 ? 'line' : 'lines'}
      </p>
    </div>
  );
}
