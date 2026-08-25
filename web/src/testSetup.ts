import { afterEach } from 'vitest';

// Views like ReportView hydrate filter/sort/page state from window.location.
// Tests that drive those controls leave query params behind, which would leak
// into every later mount in the same jsdom environment and make suites
// order-dependent. Resetting the document URL after each test keeps every
// test starting from a clean "/" address.
afterEach(() => {
  if (typeof window === 'undefined') return;
  window.history.replaceState(null, '', window.location.pathname);
});
