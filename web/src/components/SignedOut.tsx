/**
 * Shown when the API refuses this tab's session: the server restarted (every
 * launch mints a new token) or the page was opened without the launcher's
 * sign-in link. Nothing here may call the API.
 */
export function SignedOut() {
  return (
    <div className="farewell">
      <section className="about-card farewell-card" aria-labelledby="signed-out-title">
        <h1 id="signed-out-title">Open Blunt Code from its launcher</h1>
        <p>This tab is not signed in to the running Blunt Code. Each launch uses a new private sign-in link, so tabs from an earlier launch stop working.</p>
        <p>Start Blunt Code again from the Start menu or run <code>bluntcode</code>; it opens a signed-in tab. With <code>--no-browser</code>, open the <strong>Sign in:</strong> link it prints.</p>
      </section>
    </div>
  );
}
