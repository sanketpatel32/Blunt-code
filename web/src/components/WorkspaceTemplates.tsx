import { Card, CardContent, CardHeader, CardTitle, CardDescription } from './ui/card';
import { Button } from './ui/button';
import { Badge } from './ui/badge';
import { ANALYZER_CATALOG, analyzerMeta } from '../lib/analyzerCatalog';
import { useReducedMotion } from '../hooks/useReducedMotion';

export type WorkspaceTemplate = {
  id: string;
  title: string;
  description: string;
  languages: string[];
  analyzers: string[];
  suggestedName: string;
};

export const WORKSPACE_TEMPLATES: WorkspaceTemplate[] = [
  { id: 'python-fastapi', title: 'Python FastAPI', description: 'API backend with async endpoints, Pydantic models, and pytest.', languages: ['python'], analyzers: ['ruff', 'semgrep', 'secrets', 'sonarqube'], suggestedName: 'FastAPI Service' },
  { id: 'ts-react', title: 'TypeScript React', description: 'Modern SPA with Vite, React, and strict TypeScript.', languages: ['typescript', 'tsx', 'javascript'], analyzers: ['biome', 'semgrep', 'secrets', 'license-scan'], suggestedName: 'React App' },
  { id: 'go-cli', title: 'Go CLI', description: 'Command-line tool with Cobra, structured logging, and tests.', languages: ['go'], analyzers: ['semgrep', 'secrets', 'osv-dependencies', 'sonarqube'], suggestedName: 'Go CLI' },
  { id: 'java-spring', title: 'Java Spring', description: 'Spring Boot service with JPA, security, and Gradle/Maven.', languages: ['java'], analyzers: ['semgrep', 'sonarqube', 'snyk-oss', 'secrets'], suggestedName: 'Spring Service' },
  { id: 'pentest-lab', title: 'Pentest Lab', description: 'Intentionally vulnerable targets for DAST and manual testing.', languages: ['javascript', 'python', 'php'], analyzers: ['zap-pentest', 'nuclei-pentest', 'burp-pentest', 'semgrep'], suggestedName: 'Pentest Lab' },
  { id: 'full-hack-suite', title: 'Full Hack Suite', description: 'Polyglot monorepo — run every analyzer across the full catalog.', languages: ['python', 'typescript', 'go', 'java', 'yaml', 'dockerfile'], analyzers: ANALYZER_CATALOG.slice(0, 8).map((a) => a.id), suggestedName: 'Hack Suite' },
];

export const TEMPLATE_EVENT = 'bluntcode:use-template';
export const TEMPLATE_STORAGE_KEY = 'bluntcode.templatePrefill';

export function useTemplatePrefill(template: WorkspaceTemplate) {
  const payload = JSON.stringify({ name: template.suggestedName, languages: template.languages, analyzers: template.analyzers, templateId: template.id });
  try { localStorage.setItem(TEMPLATE_STORAGE_KEY, payload); } catch { /* ignore */ }
  window.dispatchEvent(new CustomEvent(TEMPLATE_EVENT, { detail: { name: template.suggestedName, languages: template.languages, analyzers: template.analyzers, templateId: template.id } }));
}

export function WorkspaceTemplates({ onUseTemplate }: { onUseTemplate?: () => void }) {
  const reduced = useReducedMotion();
  return (
    <section aria-labelledby="workspace-templates-heading" className="mt-8">
      <header className="mb-4">
        <h2 id="workspace-templates-heading" className="font-display text-lg font-bold">Start from a template</h2>
        <p className="text-sm text-[var(--color-ink-soft)]">Pick a stack — we pre-fill the workspace dialog with the right languages and analyzers.</p>
      </header>
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
        {WORKSPACE_TEMPLATES.map((t, idx) => (
          <Card key={t.id} className={`flex flex-col ${reduced ? '' : 'anim-fadeInUp'}`} style={reduced ? undefined : { animationDelay: `${idx * 40}ms` } as never}>
            <CardHeader className="pb-3">
              <CardTitle className="text-base">{t.title}</CardTitle>
              <CardDescription className="text-sm leading-relaxed">{t.description}</CardDescription>
            </CardHeader>
            <CardContent className="flex flex-1 flex-col gap-4">
              <div>
                <p className="mb-1.5 text-[0.68rem] font-mono font-bold uppercase tracking-widest text-[var(--color-ink-faint)]">Languages</p>
                <div className="flex flex-wrap gap-1.5">
                  {t.languages.map((l) => <Badge key={l} variant="outline" className="text-xs">{l}</Badge>)}
                </div>
              </div>
              <div>
                <p className="mb-1.5 text-[0.68rem] font-mono font-bold uppercase tracking-widest text-[var(--color-ink-faint)]">Analyzers</p>
                <div className="flex flex-wrap gap-1.5">
                  {t.analyzers.map((id) => {
                    const meta = analyzerMeta(id);
                    return <Badge key={id} variant="secondary" className="text-xs" title={meta?.description ?? id}>{meta?.displayName ?? id}</Badge>;
                  })}
                </div>
              </div>
              <Button
                className="mt-auto w-full"
                variant="outline"
                size="sm"
                onClick={() => { useTemplatePrefill(t); onUseTemplate?.(); }}
                aria-label={`Use ${t.title} template`}
              >
                Use template
              </Button>
            </CardContent>
          </Card>
        ))}
      </div>
    </section>
  );
}
