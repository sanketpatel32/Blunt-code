import { useEffect, useMemo, useState } from 'react';
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '../components/ui/card';
import { Button } from '../components/ui/button';
import { Badge } from '../components/ui/badge';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '../components/ui/select';
import { CodeEditor, type CodeEditorLanguage } from '../components/CodeEditor';
import { useReducedMotion } from '../hooks/useReducedMotion';

type CustomRule = {
  id: string;
  languages: string[];
  pattern: string;
  severity: 'critical' | 'high' | 'medium' | 'low' | 'info';
  message: string;
  enabled: boolean;
};

const STORAGE_KEY = 'bluntcode.customRules';

function loadRules(): CustomRule[] {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) return [];
    const parsed = JSON.parse(raw);
    if (!Array.isArray(parsed)) return [];
    return parsed as CustomRule[];
  } catch { return []; }
}

function saveRules(rules: CustomRule[]) {
  try { localStorage.setItem(STORAGE_KEY, JSON.stringify(rules)); } catch { /* quota */ }
}

const DEFAULT_YAML = `id: my-custom-rule
languages: [python, javascript]
pattern: "eval($ARG)"
severity: high
message: Avoid eval — use safe parsing instead.
`;

const SNIPPETS: Record<CodeEditorLanguage, string> = {
  yaml: DEFAULT_YAML,
  python: `id: no-eval-python
languages: [python]
pattern: "eval($ARG)"
severity: high
message: Avoid eval in Python — use ast.literal_eval instead.
# test snippet — matches below
# eval(user_input)
`,
  javascript: `id: no-eval-js
languages: [javascript]
pattern: "eval($ARG)"
severity: high
message: Avoid eval in JS — use JSON.parse instead.
// test snippet — matches below
// eval(userInput)
`,
  js: `id: no-eval-js
languages: [javascript]
pattern: "eval($ARG)"
severity: high
message: Avoid eval in JS — use JSON.parse instead.
`,
};

function parseYamlLike(text: string): Partial<CustomRule> & { rawLanguages?: string } {
  const out: Record<string, string> = {};
  for (const line of text.split('\n')) {
    const trimmed = line.trim();
    if (!trimmed || trimmed.startsWith('#') || trimmed.startsWith('//')) continue;
    const idx = trimmed.indexOf(':');
    if (idx === -1) continue;
    const k = trimmed.slice(0, idx).trim();
    const v = trimmed.slice(idx + 1).trim();
    out[k] = v;
  }
  let languages: string[] = [];
  if (out.languages) {
    const inner = out.languages.replace(/^\[/, '').replace(/\]$/, '');
    languages = inner.split(',').map((s) => s.trim().replace(/^["']|["']$/g, '')).filter(Boolean);
  }
  const severity = (out.severity?.toLowerCase().replace(/^["']|["']$/g, '') as CustomRule['severity']) ?? 'medium';
  return {
    id: out.id?.replace(/^["']|["']$/g, ''),
    languages,
    pattern: out.pattern?.replace(/^["']|["']$/g, ''),
    severity: ['critical', 'high', 'medium', 'low', 'info'].includes(severity) ? severity : 'medium',
    message: out.message?.replace(/^["']|["']$/g, ''),
  };
}

function validateYaml(text: string): string | null {
  const lines = text.split('\n');
  for (let i = 0; i < lines.length; i++) {
    const line = lines[i];
    const trimmed = line.trim();
    if (!trimmed || trimmed.startsWith('#') || trimmed.startsWith('//')) continue;
    // allow bracket-wrapped lines like languages: [python] — handled, but bare lines without colon are invalid YAML
    if (!trimmed.includes(':')) {
      // heuristic: if line looks like a YAML key without colon -> invalid
      // also catch stray tokens
      return `Invalid YAML on line ${i + 1}: missing ':' — each rule field needs "key: value"`;
    }
    // check for unclosed quotes/brackets in that line value part
    const colonIdx = trimmed.indexOf(':');
    const valuePart = trimmed.slice(colonIdx + 1).trim();
    if (valuePart) {
      const single = (valuePart.match(/'/g) ?? []).length;
      const dbl = (valuePart.match(/"/g) ?? []).length;
      if (single % 2 !== 0 || dbl % 2 !== 0) {
        return `Invalid YAML on line ${i + 1}: unclosed quote`;
      }
      const openB = (valuePart.match(/\[/g) ?? []).length;
      const closeB = (valuePart.match(/\]/g) ?? []).length;
      if (openB !== closeB) {
        return `Invalid YAML on line ${i + 1}: unclosed bracket`;
      }
    }
  }
  return null;
}

type MockFinding = { file: string; line: number; message: string; severity: CustomRule['severity'] };

function mockFindings(rule: Partial<CustomRule>): MockFinding[] {
  if (!rule.pattern || !rule.id) return [];
  const sev = (rule.severity ?? 'medium') as CustomRule['severity'];
  const msg = rule.message ?? `Matched ${rule.id}`;
  return [
    { file: `src/example.${(rule.languages?.[0] ?? 'py')}`, line: 12, message: msg, severity: sev },
    { file: `src/utils.${(rule.languages?.[0] ?? 'py')}`, line: 34, message: msg, severity: sev },
  ];
}

export function RuleStudioPage() {
  const [yaml, setYaml] = useState(DEFAULT_YAML);
  const [language, setLanguage] = useState<CodeEditorLanguage>('yaml');
  const [rules, setRules] = useState<CustomRule[]>(() => loadRules());
  const [error, setError] = useState<string | null>(null);
  const reduced = useReducedMotion();

  const parsed = useMemo(() => parseYamlLike(yaml), [yaml]);
  const yamlValidationError = useMemo(() => validateYaml(yaml), [yaml]);
  const findings = useMemo(() => mockFindings(parsed), [parsed]);
  const canSave = Boolean(parsed.id && parsed.pattern && parsed.message) && !yamlValidationError;

  useEffect(() => { saveRules(rules); }, [rules]);

  function handleSave() {
    if (yamlValidationError) { setError(yamlValidationError); return; }
    if (!canSave || !parsed.id) { setError('Fill id, pattern, and message.'); return; }
    setError(null);
    const rule: CustomRule = {
      id: parsed.id!,
      languages: parsed.languages ?? [],
      pattern: parsed.pattern ?? '',
      severity: (parsed.severity ?? 'medium') as CustomRule['severity'],
      message: parsed.message ?? '',
      enabled: true,
    };
    setRules((prev) => {
      const idx = prev.findIndex((r) => r.id === rule.id);
      if (idx >= 0) { const copy = [...prev]; copy[idx] = { ...copy[idx], ...rule }; return copy; }
      return [...prev, rule];
    });
  }

  function toggle(id: string) {
    setRules((prev) => prev.map((r) => r.id === id ? { ...r, enabled: !r.enabled } : r));
  }
  function remove(id: string) {
    setRules((prev) => prev.filter((r) => r.id !== id));
  }

  function handleInsertSnippet() {
    const snippet = SNIPPETS[language] ?? SNIPPETS.yaml;
    setYaml(snippet);
    setError(null);
  }

  const editorError = yamlValidationError ?? error;

  return (
    <div className="page">
      <header className="page-heading">
        <div>
          <p className="eyebrow">Rule Studio</p>
          <h1>Custom rules</h1>
          <p>Create YAML rules, preview matched findings, and save locally. Tab indents 2 spaces · Ctrl+S saves.</p>
        </div>
      </header>

      <div className="grid gap-6 lg:grid-cols-2">
        <Card className={reduced ? '' : 'anim-fadeInUp'}>
          <CardHeader>
            <div className="flex flex-wrap items-start justify-between gap-3">
              <div>
                <CardTitle>Rule editor</CardTitle>
                <CardDescription>Fields: id, languages, pattern, severity, message. Saved to localStorage <code className="font-mono text-xs">bluntcode.customRules</code>.</CardDescription>
              </div>
              <div className="flex items-center gap-2">
                <label htmlFor="rule-language" className="text-xs font-medium text-[var(--color-ink-soft)]">Language</label>
                <Select value={language} onValueChange={(v) => setLanguage(v as CodeEditorLanguage)}>
                  <SelectTrigger id="rule-language" aria-label="Select editor language" className="h-8 w-[9rem] text-xs">
                    <SelectValue placeholder="yaml" />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="yaml">yaml</SelectItem>
                    <SelectItem value="python">python</SelectItem>
                    <SelectItem value="javascript">javascript</SelectItem>
                  </SelectContent>
                </Select>
              </div>
            </div>
          </CardHeader>
          <CardContent className="space-y-4">
            <CodeEditor
              id="rule-yaml-editor"
              value={yaml}
              onChange={(v) => { setYaml(v); if (error) setError(null); }}
              language={language}
              ariaLabel="Rule YAML editor"
              placeholder={DEFAULT_YAML}
              error={editorError}
              onSave={handleSave}
              minHeight="14rem"
            />
            {editorError && <p role="alert" className="sr-only">{editorError}</p>}
            {!canSave && !yamlValidationError && <p className="text-xs text-[var(--color-ink-faint)]">Add at least <code>id</code>, <code>pattern</code>, and <code>message</code> to enable save.</p>}
            <div className="flex flex-wrap gap-2">
              <Button onClick={handleSave} disabled={!canSave} aria-label="Save rule">Save rule</Button>
              <Button variant="outline" onClick={() => setYaml(DEFAULT_YAML)} aria-label="Reset editor">Reset</Button>
              <Button variant="secondary" onClick={handleInsertSnippet} aria-label="Insert test snippet">Test snippet</Button>
            </div>
            <p className="text-xs text-[var(--color-ink-faint)]">Tip: Tab inserts 2 spaces · <kbd className="rounded border border-[var(--color-rule)] bg-[var(--color-surface-muted)] px-1 py-0.5 font-mono text-xs">Ctrl</kbd> + <kbd className="rounded border border-[var(--color-rule)] bg-[var(--color-surface-muted)] px-1 py-0.5 font-mono text-xs">S</kbd> saves. Monaco loads automatically if installed.</p>
          </CardContent>
        </Card>

        <Card className={reduced ? '' : 'anim-fadeInUp'} style={reduced ? undefined : { animationDelay: '60ms' } as never}>
          <CardHeader>
            <CardTitle>Live preview</CardTitle>
            <CardDescription>Mock findings generated from the current pattern.</CardDescription>
          </CardHeader>
          <CardContent>
            {!parsed.pattern ? <p className="text-sm text-[var(--color-ink-faint)]">Enter a pattern to see preview findings.</p> : findings.length ? (
              <ul className="space-y-2" aria-label="Preview findings">
                {findings.map((f, i) => (
                  <li key={i} className="flex items-start justify-between gap-3 rounded-[var(--radius-md)] border border-[var(--color-rule-faint)] bg-[var(--color-surface)] p-3">
                    <div>
                      <code className="text-xs font-mono text-[var(--color-ink-soft)]">{f.file}:{f.line}</code>
                      <p className="mt-1 text-sm text-[var(--color-ink)]">{f.message}</p>
                      {parsed.pattern && <code className="mt-1 block text-xs text-[var(--color-ink-faint)]">pattern: {parsed.pattern}</code>}
                    </div>
                    <Badge variant={f.severity === 'critical' || f.severity === 'high' ? 'danger' : f.severity === 'medium' ? 'warning' : 'secondary'} className="shrink-0">{f.severity}</Badge>
                  </li>
                ))}
              </ul>
            ) : <p className="text-sm text-[var(--color-ink-faint)]">No matches for the current rule.</p>}
            {parsed.id && <div className="mt-4 flex flex-wrap gap-1.5"><Badge variant="outline">{parsed.id}</Badge>{(parsed.languages ?? []).map((l) => <Badge key={l} variant="secondary">{l}</Badge>)}{parsed.severity && <Badge variant="outline">{parsed.severity}</Badge>}</div>}
          </CardContent>
        </Card>
      </div>

      <section className="mt-8" aria-labelledby="saved-rules-heading">
        <h2 id="saved-rules-heading" className="font-display text-lg font-bold">Saved rules</h2>
        <p className="text-sm text-[var(--color-ink-soft)]">{rules.length} rule{rules.length === 1 ? '' : 's'} stored locally.</p>
        {!rules.length ? <p className="mt-4 rounded-[var(--radius-md)] border border-dashed border-[var(--color-rule)] p-6 text-sm text-[var(--color-ink-faint)]">No custom rules yet. Save one from the editor above.</p> : (
          <div className="mt-4 grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
            {rules.map((r) => (
              <Card key={r.id} className={reduced ? '' : 'anim-fadeInUp'}>
                <CardHeader className="pb-2">
                  <CardTitle className="text-base flex items-center justify-between gap-2">
                    <span className="truncate">{r.id}</span>
                    <Badge variant={r.severity === 'critical' || r.severity === 'high' ? 'danger' : r.severity === 'medium' ? 'warning' : 'secondary'}>{r.severity}</Badge>
                  </CardTitle>
                  <CardDescription className="line-clamp-2">{r.message}</CardDescription>
                </CardHeader>
                <CardContent className="space-y-3">
                  <code className="block truncate rounded bg-[var(--color-surface-muted)] px-2 py-1 font-mono text-xs text-[var(--color-ink-soft)]" title={r.pattern}>{r.pattern}</code>
                  <div className="flex flex-wrap gap-1.5">{r.languages.map((l) => <Badge key={l} variant="outline" className="text-xs">{l}</Badge>)}</div>
                  <div className="flex items-center justify-between gap-2 pt-2">
                    <label className="flex items-center gap-2 text-sm">
                      <span className="relative inline-flex">
                        <input type="checkbox" role="switch" aria-checked={r.enabled} aria-label={`Enable ${r.id}`} checked={r.enabled} onChange={() => toggle(r.id)} className="peer sr-only" />
                        <span className="inline-flex h-5 w-9 items-center rounded-full border border-[var(--color-rule)] bg-[var(--color-surface-muted)] p-0.5 transition-colors peer-checked:bg-[var(--color-accent)] peer-checked:border-[var(--color-accent)] peer-focus-visible:ring-2 peer-focus-visible:ring-[var(--color-focus)] peer-focus-visible:ring-offset-2" aria-hidden="true">
                          <span className={`block h-4 w-4 rounded-full bg-white shadow transition-transform ${r.enabled ? 'translate-x-4' : 'translate-x-0'}`} style={reduced ? { transition: 'none' } : undefined} />
                        </span>
                      </span>
                      <span className="text-xs font-medium text-[var(--color-ink-soft)]">{r.enabled ? 'Enabled' : 'Disabled'}</span>
                    </label>
                    <Button variant="ghost" size="sm" onClick={() => remove(r.id)} aria-label={`Delete ${r.id}`} className="text-[var(--color-danger)] hover:text-[var(--color-danger)] hover:bg-[var(--color-danger-soft)]">Delete</Button>
                  </div>
                </CardContent>
              </Card>
            ))}
          </div>
        )}
      </section>
    </div>
  );
}
