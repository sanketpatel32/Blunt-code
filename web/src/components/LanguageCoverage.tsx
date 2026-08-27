import { ANALYZER_CATALOG, type AnalyzerCategory } from '../lib/analyzerCatalog';

const LANGUAGES = ['python', 'javascript', 'typescript', 'go', 'java', 'kotlin', 'csharp', 'php', 'rust', 'swift', 'ruby', 'shell', 'dockerfile', 'yaml'] as const;

const LANG_LABELS: Record<string, string> = {
  python: 'Python', javascript: 'JS', typescript: 'TS', go: 'Go', java: 'Java', kotlin: 'Kotlin', csharp: 'C#', php: 'PHP', rust: 'Rust', swift: 'Swift', ruby: 'Ruby', shell: 'Shell', dockerfile: 'Dockerfile', yaml: 'YAML',
};

export function LanguageCoverage({ compact }: { compact?: boolean }) {
  const analyzers = compact ? ANALYZER_CATALOG.slice(0, 8) : ANALYZER_CATALOG;
  return (
    <section className="language-coverage" aria-label="Language coverage matrix">
      <div className="section-head">
        <div>
          <h3>Language coverage</h3>
          <p className="muted text-sm">Which analyzers run for each detected language.</p>
        </div>
      </div>
      <div className="table-wrap overflow-auto">
        <table className="coverage-table text-xs">
          <thead>
            <tr>
              <th scope="col">Analyzer</th>
              {LANGUAGES.map((lang) => <th key={lang} scope="col" title={lang}>{LANG_LABELS[lang]}</th>)}
            </tr>
          </thead>
          <tbody>
            {analyzers.map((a) => (
              <tr key={a.id}>
                <td><span className="badge text-xs">{a.displayName}</span> <small className="text-[var(--color-ink-faint)] hidden sm:inline">{a.category}</small></td>
                {LANGUAGES.map((lang) => {
                  const covered = a.languages.includes(lang) || a.languages.includes(lang.toLowerCase());
                  return <td key={lang} className="text-center">{covered ? <span aria-label={`${a.displayName} covers ${lang}`} title="covered" className="inline-block w-2 h-2 rounded-full bg-[var(--color-success)]" /> : <span className="text-[var(--color-ink-faint)]">—</span>}</td>;
                })}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </section>
  );
}

export function LanguageChips({ languages }: { languages?: string[] }) {
  if (!languages?.length) return null;
  return <span className="flex flex-wrap gap-1">{languages.map((l) => <span key={l} className="badge text-[10px]">{l}</span>)}</span>;
}
