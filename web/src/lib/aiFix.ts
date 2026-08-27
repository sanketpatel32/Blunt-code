import type { Finding } from '../types';

export type AiFixCategory = 'security' | 'style' | 'maintainability';
export type AiFixConfidence = 'high' | 'medium' | 'low';

export interface AiFixSuggestion {
  id: string;
  category: AiFixCategory;
  title: string;
  original: string;
  fixed: string;
  explanation: string;
  confidence: AiFixConfidence;
}

type Template = Omit<AiFixSuggestion, 'id'>;

const SECURITY_TEMPLATES: Template[] = [
  {
    category: 'security',
    title: 'Sanitize SQL query — use parameterized statement',
    original: `query = f"SELECT * FROM users WHERE id = '{user_input}'"\ncursor.execute(query)`,
    fixed: `query = "SELECT * FROM users WHERE id = %s"\ncursor.execute(query, (user_input,))`,
    explanation: 'Replaces string interpolation with a parameterized query to prevent SQL injection.',
    confidence: 'high',
  },
  {
    category: 'security',
    title: 'Escape HTML output to prevent XSS',
    original: `element.innerHTML = userContent;`,
    fixed: `element.textContent = userContent;\n// or: element.innerHTML = DOMPurify.sanitize(userContent);`,
    explanation: 'Avoids injecting raw user content as HTML. Use textContent or a sanitizer.',
    confidence: 'high',
  },
  {
    category: 'security',
    title: 'Validate and pin dependency version',
    original: `"dependencies": { "lodash": "*" }`,
    fixed: `"dependencies": { "lodash": "^4.17.21" }\n// run: npm audit fix`,
    explanation: 'Pins wildcard dependency to a patched version and suggests audit.',
    confidence: 'medium',
  },
];

const STYLE_TEMPLATES: Template[] = [
  {
    category: 'style',
    title: 'Consistent formatting — run formatter',
    original: `function foo(  a,b ){\nreturn a+b\n}`,
    fixed: `function foo(a, b) {\n  return a + b;\n}`,
    explanation: 'Applies canonical spacing and semicolons via the project formatter.',
    confidence: 'high',
  },
  {
    category: 'style',
    title: 'Prefer const and arrow function',
    original: `var x = function(y){ return y * 2 }`,
    fixed: `const x = (y) => y * 2;`,
    explanation: 'Modernizes declaration style for consistency.',
    confidence: 'medium',
  },
  {
    category: 'style',
    title: 'Alphabetize imports',
    original: `import { z } from 'zod';\nimport { a } from './a';`,
    fixed: `import { a } from './a';\nimport { z } from 'zod';`,
    explanation: 'Sorts imports alphabetically per style guide.',
    confidence: 'low',
  },
];

const MAINTAINABILITY_TEMPLATES: Template[] = [
  {
    category: 'maintainability',
    title: 'Extract duplicated logic into helper',
    original: `if (user.role === 'admin' && user.active && !user.banned) { /* ... */ }\n// repeated 3×`,
    fixed: `function isEligibleAdmin(user) {\n  return user.role === 'admin' && user.active && !user.banned;\n}\nif (isEligibleAdmin(user)) { /* ... */ }`,
    explanation: 'Reduces duplication by extracting a named predicate.',
    confidence: 'medium',
  },
  {
    category: 'maintainability',
    title: 'Replace magic number with named constant',
    original: `if (retries > 5) throw new Error('too many');`,
    fixed: `const MAX_RETRIES = 5;\nif (retries > MAX_RETRIES) throw new Error('too many');`,
    explanation: 'Names the threshold for readability and single-point change.',
    confidence: 'high',
  },
  {
    category: 'maintainability',
    title: 'Simplify nested conditionals',
    original: `if (a) { if (b) { doThing(); } }`,
    fixed: `if (a && b) {\n  doThing();\n}`,
    explanation: 'Flattens nesting for clarity.',
    confidence: 'medium',
  },
];

const BY_CATEGORY: Record<AiFixCategory, Template[]> = {
  security: SECURITY_TEMPLATES,
  style: STYLE_TEMPLATES,
  maintainability: MAINTAINABILITY_TEMPLATES,
};

function normalizeCategory(raw: string): AiFixCategory {
  const v = raw.toLowerCase();
  if (v.includes('secur') || v === 'secrets' || v === 'pentest' || v === 'dependencies' || v === 'container') return 'security';
  if (v.includes('style') || v === 'lint') return 'style';
  return 'maintainability';
}

function hashString(s: string): number {
  let h = 0;
  for (let i = 0; i < s.length; i++) h = (h * 31 + s.charCodeAt(i)) >>> 0;
  return h;
}

export function getAiFixSuggestion(finding: Finding, variant?: number): AiFixSuggestion {
  const cat = normalizeCategory(finding.category ?? '');
  const pool = BY_CATEGORY[cat];
  const key = `${finding.rule_id ?? ''}|${finding.fingerprint ?? ''}|${finding.id ?? ''}|${finding.category}`;
  const idx = variant != null ? variant % pool.length : hashString(key) % pool.length;
  const t = pool[idx];
  return { ...t, id: `${cat}-${idx}` };
}

export function regenerateAiFix(finding: Finding, currentId: string): AiFixSuggestion {
  const cat = normalizeCategory(finding.category ?? '');
  const pool = BY_CATEGORY[cat];
  const cur = pool.findIndex((t, i) => `${cat}-${i}` === currentId);
  const next = cur === -1 ? 0 : (cur + 1) % pool.length;
  const t = pool[next];
  return { ...t, id: `${cat}-${next}` };
}

export function confidenceVariant(c: AiFixConfidence): 'success' | 'warning' | 'default' {
  if (c === 'high') return 'success';
  if (c === 'medium') return 'warning';
  return 'default';
}
