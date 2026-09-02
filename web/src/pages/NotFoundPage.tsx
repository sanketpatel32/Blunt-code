import type { Route } from '../lib/router';
import { Empty } from '../components/ui';
import { MagnifierIcon } from '../components/icons';
import { PageHeader } from '../components/PageHeader';

/** Workspace- or scan-shaped addresses get a removed-content hint instead of the generic mistype text; no fetching, just the path shape. */
const removedHintPattern = /^\/(workspaces?|scans?)\/[^/]+/i;

export function NotFoundPage({ go }: { go: (r: Route) => void }) {
  const pointsAtRemoved = removedHintPattern.test(window.location.pathname);
  return (
    <div className="page">
      <PageHeader
        eyebrow="404"
        title="Page not found"
        description="This address does not match any page in Blunt Code."
      />
      <Empty
        title="Nothing here"
        icon={<MagnifierIcon />}
        action={
          <div className="not-found-actions">
            <button type="button" className="button primary" onClick={() => go({ page: 'home' })}>
              Go to Home
            </button>
            <button type="button" className="button secondary" onClick={() => go({ page: 'workspaces' })}>
              Open Workspaces
            </button>
          </div>
        }
      >
        {pointsAtRemoved
          ? 'This workspace or scan may have been removed, or its id is mistyped. Your other workspaces and reports are still saved on this computer.'
          : 'The link may be mistyped or out of date. Your workspaces and reports are still saved on this computer.'}
      </Empty>
    </div>
  );
}
