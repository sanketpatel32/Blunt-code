import type { Workspace } from '../types';

type RootFields = Pick<Workspace, 'root_missing' | 'root_problem' | 'root_path'>;

/** Short label for a workspace whose folder can no longer be used. */
export function rootProblemLabel(workspace: RootFields): string {
  if (workspace.root_problem === 'not_directory') return 'Not a folder';
  if (workspace.root_problem === 'unreadable') return 'Folder unreadable';
  if (workspace.root_problem === 'probe_timeout') return 'Folder unreachable';
  return 'Folder missing';
}

/** Full sentence explaining the root problem and the two ways out (restore or remove). */
export function rootProblemDescription(workspace: RootFields): string {
  const path = workspace.root_path || 'This workspace folder';
  if (workspace.root_problem === 'not_directory') return `${path} is now a file, not a folder. Restore the folder or remove this workspace.`;
  if (workspace.root_problem === 'unreadable') return `${path} cannot be read. Check its permissions or remove this workspace.`;
  if (workspace.root_problem === 'probe_timeout') return `${path} did not respond in time — the drive may be disconnected or the filesystem is hung. Check the folder is reachable, or remove this workspace.`;
  return `${path} no longer exists. Restore the folder or remove this workspace.`;
}

/** Warning pill shown beside a workspace name when its folder is gone; renders nothing for healthy workspaces. */
export function RootMissingBadge({ workspace, className = '' }: { workspace: RootFields; className?: string }) {
  if (!workspace.root_missing && workspace.root_problem !== 'probe_timeout') return null;
  return <span className={`root-missing-badge ${className}`.trim()} title={rootProblemDescription(workspace)}>{rootProblemLabel(workspace)}</span>;
}
