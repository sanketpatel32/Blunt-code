import { describe, expect, it } from 'vitest';
import type { Finding } from '../types';
import { isSecurityRelevant, mapFindingToOwasp } from './ComplianceMatrix';

const f = (over: Partial<Finding>): Finding => ({ id: 'x', analyzer_id: 'semgrep', rule_id: 'r', severity: 'medium', category: 'security', message: '', path: 'a.go', ...over } as Finding);

describe('OWASP classification', () => {
  it('never files TODO markers or code-quality findings under an OWASP category', () => {
    expect(mapFindingToOwasp(f({ analyzer_id: 'todo', category: 'maintainability', rule_id: 'todo.todo', title: 'TODO comment marker', message: 'TODO: add logging' }))).toBeNull();
    expect(mapFindingToOwasp(f({ category: 'style', message: 'config key not sorted' }))).toBeNull();
    expect(isSecurityRelevant(f({ analyzer_id: 'todo', category: 'other' }))).toBe(false);
  });
  it('still classifies real security findings, matching hints at word starts', () => {
    expect(mapFindingToOwasp(f({ title: 'SQL injection via string concat' }))).toBe('A03');
    expect(mapFindingToOwasp(f({ title: 'Unsafe Object Deserialization' }))).toBe('A08');
    expect(mapFindingToOwasp(f({ message: 'written by the author of this file' }))).toBeNull();
  });
});
