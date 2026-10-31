import { describe, expect, it } from 'vitest';
import { editorUrl } from './SourcePane';
import type { Finding } from '../../types';

const finding = { id: 'f', analyzer_id: 'ruff', severity: 'high', category: 'security', title: 't', message: 'm', relative_path: 'src/app main.py', start_line: 12 } as Finding;

describe('editorUrl', () => {
  it('builds a vscode link from a Windows root with the line number', () => {
    expect(editorUrl('C:\\code\\my app\\', finding)).toBe('vscode://file/C:/code/my%20app/src/app%20main.py:12');
  });
  it('builds a vscode link from a POSIX root', () => {
    expect(editorUrl('/home/me/app', { ...finding, start_line: undefined })).toBe('vscode://file/home/me/app/src/app%20main.py');
  });
  it('returns nothing without a root or a file', () => {
    expect(editorUrl(undefined, finding)).toBeUndefined();
    expect(editorUrl('/x', { ...finding, relative_path: undefined })).toBeUndefined();
  });
});
