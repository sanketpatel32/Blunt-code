import { useEffect, useRef, useState } from 'react';

/**
 * Terminal screen shown once the app has stopped itself — via Close app or the
 * update handoff. The backend is gone by then, so nothing here may touch the
 * API: it is static reassurance, a best-effort tab close, and (while updating)
 * a stepper explaining that a new tab opens on its own.
 */
export function AppClosedScreen({ mode, version }: { mode: 'stopped' | 'updating'; version?: string }) {
  const [tabKeptOpen, setTabKeptOpen] = useState(false);
  const probe = useRef<ReturnType<typeof setTimeout>>(undefined);
  // If the tab survives, the pending fallback-hint timer must not fire into a
  // detached tree.
  useEffect(() => () => clearTimeout(probe.current), []);
  function closeTab() {
    window.close();
    // Browsers only let scripts close tabs they opened themselves. A closed
    // tab never runs timers, so this firing at all means window.close() was
    // blocked and the hint below should appear.
    probe.current = setTimeout(() => setTabKeptOpen(true), 400);
  }
  const updating = mode === 'updating';
  return (
    <div className="farewell">
      <section className="about-card farewell-card" aria-labelledby="farewell-title">
        <svg className="farewell-mark" viewBox="0 0 32 32" aria-hidden="true" focusable="false">
          <rect width="32" height="32" rx="8" fill="var(--color-brand-mark)" />
          <path d="M16 5.2 L23.6 9 L23.6 17.2 C23.6 21 20.2 24.5 16 26.8 C11.8 24.5 8.4 21 8.4 17.2 L8.4 9 Z" fill="none" stroke="var(--color-paper)" strokeOpacity="0.14" strokeWidth="1" strokeLinejoin="round" />
          <path d="M11.8 15.9 L15.2 19.1 L20.6 12.1" fill="none" stroke="var(--color-paper)" strokeWidth="2.7" strokeLinecap="round" strokeLinejoin="round" opacity="0.96" />
          <circle cx="23.4" cy="7.2" r="1.7" fill="var(--color-brand-accent)" />
        </svg>
        <h1 id="farewell-title">{updating ? 'Updating Blunt Code' : 'Blunt Code has stopped'}</h1>
        {updating ? (
          <>
            <p>{version ? `Version ${version} is installing. ` : 'The update is installing. '}Blunt Code closed itself and the new version opens in a new browser tab when it is ready — this tab can be closed.</p>
            <ol className="farewell-steps">
              <li className="farewell-step is-done"><span className="farewell-step-dot" aria-hidden="true" />Close the running app</li>
              <li className="farewell-step is-active"><span className="farewell-step-dot" aria-hidden="true" />Download and install the new version</li>
              <li className="farewell-step"><span className="farewell-step-dot" aria-hidden="true" />Reopen Blunt Code automatically</li>
            </ol>
          </>
        ) : (
          <p>Workspaces, scans, and settings stay saved on this computer. Start Blunt Code again from the Start menu whenever you need it.</p>
        )}
        <p className="farewell-actions">
          <button type="button" className="button" onClick={closeTab} autoFocus>Close this tab</button>
        </p>
        <p aria-live="polite" className="muted farewell-hint">{tabKeptOpen ? 'Your browser kept this tab open — it is safe to close it manually.' : ''}</p>
      </section>
    </div>
  );
}
