import { describe, expect, it } from 'vitest';
import { rootProblemDescription, rootProblemLabel } from './RootMissingBadge';

describe('root problem copy', () => {
  it('names each problem the server reports', () => {
    expect(rootProblemLabel({ root_path: '', root_missing: true, root_problem: 'missing' })).toBe('Folder missing');
    expect(rootProblemLabel({ root_path: '', root_missing: true, root_problem: 'not_directory' })).toBe('Not a folder');
    expect(rootProblemLabel({ root_path: '', root_missing: true, root_problem: 'unreadable' })).toBe('Folder unreadable');
    expect(rootProblemLabel({ root_path: '', root_missing: true })).toBe('Folder missing');
  });

  it('names the path and the way out', () => {
    expect(rootProblemDescription({ root_path: '/tmp/x', root_problem: 'unreadable' })).toBe('/tmp/x cannot be read. Check its permissions or remove this workspace.');
    expect(rootProblemDescription({ root_path: '/tmp/x', root_problem: 'not_directory' })).toContain('is now a file');
    expect(rootProblemDescription({ root_path: '/tmp/x' })).toContain('/tmp/x no longer exists');
  });
});
