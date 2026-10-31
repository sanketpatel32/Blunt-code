import { afterEach } from 'vitest';

// Recharts' ResponsiveContainer measures with ResizeObserver, which jsdom
// lacks. A no-op observer keeps chart routes renderable in tests.
if (typeof globalThis.ResizeObserver === 'undefined') {
  globalThis.ResizeObserver = class { observe() {} unobserve() {} disconnect() {} } as unknown as typeof ResizeObserver;
}

// Views like ReportView hydrate filter/sort/page state from window.location.
// Tests that drive those controls leave query params behind, which would leak
// into every later mount in the same jsdom environment and make suites
// order-dependent. Resetting the document URL after each test keeps every
// test starting from a clean "/" address.
afterEach(() => {
  if (typeof window === 'undefined') return;
  window.history.replaceState(null, '', window.location.pathname);
});
