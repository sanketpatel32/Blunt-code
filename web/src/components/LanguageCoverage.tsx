import { useMemo, useState } from 'react';
import { ANALYZER_CATALOG, LANGUAGE_FAMILIES, ALL_LANGUAGES, type AnalyzerCategory } from '../lib/analyzerCatalog';

const FAMILY_ORDER = ['Systems', 'Web', 'Mobile', 'Script', 'Data', 'Functional'] as const;

const LANG_LABELS: Record<string, string> = {
  python: 'Python', javascript: 'JS', typescript: 'TS', go: 'Go', java: 'Java', kotlin: 'Kotlin', csharp: 'C#', c: 'C', cpp: 'C++', ruby: 'Ruby', php: 'PHP', rust: 'Rust', swift: 'Swift', scala: 'Scala', dart: 'Dart', elixir: 'Elixir', haskell: 'Haskell', clojure: 'Clojure', erlang: 'Erlang', fsharp: 'F#', lua: 'Lua', zig: 'Zig', ocaml: 'OCaml', perl: 'Perl', 'objective-c': 'ObjC', vue: 'Vue', svelte: 'Svelte', html: 'HTML', css: 'CSS', scss: 'SCSS', json: 'JSON', yaml: 'YAML', toml: 'TOML', xml: 'XML', sql: 'SQL', graphql: 'GQL', shell: 'Shell', powershell: 'PS', batch: 'Batch', markdown: 'MD', dockerfile: 'Docker', env: 'Env',
};

function analyzerCountFor(lang: string): number {
  return ANALYZER_CATALOG.filter((a) => a.languages.includes(lang)).length;
}

export function LanguageCoverage({ compact }: { compact?: boolean }) {
  const [filter, setFilter] = useState('');
  const [familyFilter, setFamilyFilter] = useState<string>('all');
  const q = filter.trim().toLowerCase();

  const allLangs = useMemo(() => {
    let langs = [...ALL_LANGUAGES] as string[];
    if (familyFilter !== 'all') {
      const fam = LANGUAGE_FAMILIES[familyFilter] ?? [];
      langs = langs.filter((l) => (fam as readonly string[]).includes(l));
    }
    if (q) langs = langs.filter((l) => l.toLowerCase().includes(q) || (LANG_LABELS[l] ?? '').toLowerCase().includes(q));
    return langs;
  }, [q, familyFilter]);

  // For compact mode show first 12 languages only
  const visibleLangs = compact ? allLangs.slice(0, 12) : allLangs;
  const analyzers = compact ? ANALYZER_CATALOG.slice(0, 8) : ANALYZER_CATALOG;

  function exportCsv() {
    const header = ['Analyzer', 'Category', ...visibleLangs.map((l) => LANG_LABELS[l] ?? l), 'Total'];
    const rows = analyzers.map((a) => {
      const cells = visibleLangs.map((lang) => (a.languages.includes(lang) ? '1' : '0'));
      const total = visibleLangs.filter((lang) => a.languages.includes(lang)).length;
      return [a.displayName, a.category, ...cells, String(total)];
    });
    // Add analyzer count row per language
    const countRow = ['Analyzers covering', '', ...visibleLangs.map((l) => String(analyzerCountFor(l))), ''];
    const all = [header, ...rows, countRow];
    const csv = all.map((r) => r.map((c) => `"${String(c).replaceAll('"', '""')}"`).join(',')).join('\n');
    const blob = new Blob([csv], { type: 'text/csv;charset=utf-8;' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = 'language-coverage.csv';
    a.click();
    URL.revokeObjectURL(url);
  }

  return (
    <section className="language-coverage" aria-label="Language coverage matrix">
      <div className="section-head flex flex-wrap items-start justify-between gap-3">
        <div>
          <h3>Language coverage</h3>
          <p className="muted text-sm">Which analyzers run for each detected language. Grouped by family. {visibleLangs.length} languages shown.</p>
        </div>
        <button type="button" onClick={exportCsv} className="button secondary text-xs" aria-label="Export coverage as CSV">Export CSV</button>
      </div>

      <div className="flex flex-wrap items-center gap-2 py-2">
        <label className="flex items-center gap-2 text-xs">
          <span className="sr-only">Search languages</span>
          <input
            type="search"
            placeholder="Filter languages"
            value={filter}
            onChange={(e) => setFilter(e.target.value)}
            className="rounded-md border border-[var(--color-rule)] bg-[var(--color-surface)] px-2 py-1 text-xs w-36"
            aria-label="Search languages"
          />
        </label>
        <div className="flex items-center gap-1" role="group" aria-label="Filter by family">
          <button type="button" onClick={() => setFamilyFilter('all')} aria-pressed={familyFilter === 'all'} className={`rounded-full px-2 py-0.5 text-xs border ${familyFilter === 'all' ? 'bg-[var(--color-accent)] text-[var(--color-accent-ink)] border-[var(--color-accent)]' : 'bg-[var(--color-surface)] border-[var(--color-rule)]'}`}>All</button>
          {FAMILY_ORDER.map((fam) => (
            <button key={fam} type="button" onClick={() => setFamilyFilter(fam)} aria-pressed={familyFilter === fam} className={`rounded-full px-2 py-0.5 text-xs border ${familyFilter === fam ? 'bg-[var(--color-accent)] text-[var(--color-accent-ink)] border-[var(--color-accent)]' : 'bg-[var(--color-surface)] border-[var(--color-rule)]'}`}>{fam}</button>
          ))}
        </div>
        {q && <span className="text-xs text-[var(--color-ink-soft)]" aria-live="polite">{visibleLangs.length} match{q ? ` for "${filter}"` : ''}</span>}
      </div>

      <div className="table-wrap overflow-auto">
        <table className="coverage-table text-xs">
          <thead>
            <tr>
              <th scope="col">Analyzer</th>
              {visibleLangs.map((lang) => (
                <th key={lang} scope="col" title={lang} className="text-center">
                  <span className="inline-flex flex-col items-center">
                    <span>{LANG_LABELS[lang] ?? lang}</span>
                    <span className="text-[10px] font-normal text-[var(--color-ink-faint)]">{analyzerCountFor(lang)}x</span>
                  </span>
                </th>
              ))}
              <th scope="col" className="text-center">Total</th>
            </tr>
          </thead>
          <tbody>
            {analyzers.map((a) => {
              const total = visibleLangs.filter((lang) => a.languages.includes(lang)).length;
              return (
                <tr key={a.id}>
                  <td><span className="badge text-xs">{a.displayName}</span> <small className="text-[var(--color-ink-faint)] hidden sm:inline">{a.category}</small></td>
                  {visibleLangs.map((lang) => {
                    const covered = a.languages.includes(lang);
                    return <td key={lang} className="text-center">{covered ? <span aria-label={`${a.displayName} covers ${lang}`} title="covered" className="inline-block w-2 h-2 rounded-full bg-[var(--color-success)]" /> : <span className="text-[var(--color-ink-faint)]">—</span>}</td>;
                  })}
                  <td className="text-center font-mono text-[var(--color-ink-soft)]">{total}</td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
      <div className="mt-2 flex flex-wrap gap-1" aria-label="Language families">
        {FAMILY_ORDER.map((fam) => (
          <span key={fam} className="inline-flex items-center gap-1 rounded-full border border-[var(--color-rule)] bg-[var(--color-surface)] px-2 py-0.5 text-[10px] text-[var(--color-ink-soft)]">
            <strong>{fam}:</strong> {(LANGUAGE_FAMILIES[fam] as readonly string[]).map((l) => LANG_LABELS[l] ?? l).join(', ')}
          </span>
        ))}
      </div>
    </section>
  );
}

export function LanguageChips({ languages }: { languages?: string[] }) {
  if (!languages?.length) return null;
  return <span className="flex flex-wrap gap-1">{languages.map((l) => <span key={l} className="badge text-[10px]">{l}</span>)}</span>;
}
