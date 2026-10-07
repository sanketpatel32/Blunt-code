import { api } from '../api';
import type { Route } from '../lib/router';
import { useLoad } from './useLoad';

/** Resolve scan IDs to their workspace before constructing workspace links. */
export function useWorkspaceContext(route: Route) {
  const scoped = ['workspace', 'files', 'history', 'pentest', 'scan'].includes(route.page) && !!route.id;
  const key = `${route.page}:${route.id ?? ''}`;
  const state = useLoad(async () => {
    if (!scoped) return { key, workspace: undefined };
    const id = route.page === 'scan' ? (await api.scan(route.id!))?.workspace_id : route.id!;
    return { key, workspace: id ? await api.workspace(id) : undefined };
  }, [scoped, route.page, route.id]);
  return { ...state, data: state.data?.key === key ? state.data.workspace : undefined };
}
