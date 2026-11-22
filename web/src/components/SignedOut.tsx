/**
 * Shown when the API refuses this tab's session: the server restarted (every
 * launch mints a new token) or the page was opened without the launcher's
 * sign-in link. Nothing here may call the API.
 */
export function SignedOut() {
  return (
    <div className="farewell">
      <section className="about-card farewell-card" aria-labelledby="signed-out-title">
        <svg className="farewell-mark" viewBox="0 0 32 32" aria-hidden="true" focusable="false">
          <rect width="32" height="32" rx="8" fill="var(--color-brand-mark)" />
          <path d="M16 5.2 L23.6 9 L23.6 17.2 C23.6 21 20.2 24.5 16 26.8 C11.8 24.5 8.4 21 8.4 17.2 L8.4 9 Z" fill="none" stroke="var(--color-paper)" strokeOpacity="0.14" strokeWidth="1" strokeLinejoin="round" />
          <path d="M11.8 15.9 L15.2 19.1 L20.6 12.1" fill="none" stroke="var(--color-paper)" strokeWidth="2.7" strokeLinecap="round" strokeLinejoin="round" opacity="0.96" />
          <circle cx="23.4" cy="7.2" r="1.7" fill="var(--color-brand-accent)" />
        </svg>
        <h1 id="signed-out-title">This tab signed out</h1>
        <p>Every launch of Blunt Code uses a new private sign-in link, so tabs from an earlier launch stop working. Getting back in takes one step:</p>
        <ol className="farewell-steps">
          <li className="farewell-step"><span className="farewell-step-dot" aria-hidden="true" />Start Blunt Code from the Start menu, or run <code>bluntcode</code></li>
          <li className="farewell-step"><span className="farewell-step-dot" aria-hidden="true" />A signed-in tab opens automatically — close this one</li>
          <li className="farewell-step"><span className="farewell-step-dot" aria-hidden="true" />Running with <code>--no-browser</code>? Open the <strong>Sign in:</strong> link it prints</li>
        </ol>
      </section>
    </div>
  );
}
