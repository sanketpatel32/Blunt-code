import type { MouseEvent } from 'react';

/** Keep modified clicks and explicit link targets under browser control. */
export function navigateFromLink(event: MouseEvent<HTMLAnchorElement>, navigate: () => void) {
  if (event.defaultPrevented || event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
  const link = event.currentTarget;
  if (link.hasAttribute('download') || (link.target && link.target !== '_self')) return;
  event.preventDefault();
  navigate();
}
