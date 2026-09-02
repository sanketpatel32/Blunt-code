import { useEffect, useRef, useState, type ReactNode, type SVGProps } from 'react';
import { api } from '../api';
import { copyToClipboard } from '../lib/clipboard';
import { useLoad } from '../hooks/useLoad';
import { LanguageCoverage } from '../components/LanguageCoverage';
import { PageHeader } from '../components/PageHeader';

/**
 * Privacy point icons drawn inline (AboutPage gets no stylesheet of its own) so
 * they match components/icons exactly: 24-grid, stroke 1.75, decorative.
 * Sizing rides the width/height attributes because no shared class targets them.
 */
function PointIcon({ children, ...props }: SVGProps<SVGSVGElement> & { children: ReactNode }) {
  return <svg width={16} height={16} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.75} strokeLinecap="round" strokeLinejoin="round" aria-hidden="true" focusable="false" {...props}>{children}</svg>;
}

/** Local-only — the analysis happens on this machine. */
function MonitorIcon(props: SVGProps<SVGSVGElement>) {
  return <PointIcon {...props}><rect x="4.5" y="4" width="15" height="11.5" rx="1.6" /><path d="M12 15.5V19" /><path d="M8.5 19h7" /></PointIcon>;
}

/** No account — nothing to register or sign in to. */
function PersonSlashIcon(props: SVGProps<SVGSVGElement>) {
  return <PointIcon {...props}><circle cx="10" cy="8" r="3.25" /><path d="M4.5 19c.8-2.9 3-4.5 5.5-4.5.95 0 1.87.22 2.68.65" /><path d="m15.5 15.5 4 4" /></PointIcon>;
}

/** No telemetry — usage never leaves the machine. */
function EyeOffIcon(props: SVGProps<SVGSVGElement>) {
  return <PointIcon {...props}><path d="M3 12s3.2-6.5 9-6.5S21 12 21 12s-3.2 6.5-9 6.5S3 12 3 12z" /><circle cx="12" cy="12" r="2.75" /><path d="m5 5 14 14" /></PointIcon>;
}

// Hallmark polished
 export function AboutPage({ onUpdateHandoff }: { onUpdateHandoff?: (version: string) => void }) {
  const meta = useLoad(api.meta, []); const health = useLoad(api.health, []);
  const [copied, setCopied] = useState(false);
  const revert = useRef<ReturnType<typeof setTimeout>>(undefined);
  // The confirmation reverts on a timer; drop it on unmount so the state
  // update never fires into a detached tree.
  useEffect(() => () => clearTimeout(revert.current), []);
  const platform = [meta.data?.os, meta.data?.architecture].filter(Boolean).join(' / ');
  const serverStatus = health.loading ? 'Checking…' : health.error ? 'Unavailable' : health.data?.status ?? 'Ready';
  async function copyVersionInfo() {
    const text = ['Blunt Code', `Version: ${meta.data?.version ?? 'Unknown'}`, `API version: ${meta.data?.api_version ?? 'Unknown'}`, `Platform: ${platform || 'Unknown'}`, `Server: ${serverStatus}`].join('\n');
    if (!(await copyToClipboard(text))) return;
    setCopied(true);
    clearTimeout(revert.current);
    revert.current = setTimeout(() => setCopied(false), 1500);
  }
  return (
    <div className="page narrow">
      <PageHeader
        eyebrow="About"
        title="Blunt Code"
        description="A local-first static analysis application for Windows."
      />
      <section className="about-card">
        <h2>Local by default <span className="badge">{meta.data?.version ? `v${meta.data.version}` : 'Version unknown'}</span></h2>
        <p>Analyzes selected files on this computer and combines results from local analysis tools.</p>
        <ul className="about-points" style={{ display: 'grid', gap: 'var(--space-xs)', margin: 'var(--space-lg) 0 0', padding: 0, listStyle: 'none' }}>
          <li className="local-signal"><MonitorIcon />Local-only analysis</li>
          <li className="local-signal"><PersonSlashIcon />No account required</li>
          <li className="local-signal"><EyeOffIcon />No telemetry</li>
        </ul>
        <dl>
          <dt>Server</dt><dd>{serverStatus}</dd>
          <dt>API version</dt><dd>{meta.data?.api_version ?? 'Unknown'}</dd>
          <dt>Platform</dt><dd>{platform || 'Unknown'}</dd>
        </dl>
        <p aria-live="polite" style={{ margin: 'var(--space-xl) 0 0' }}>
          <button type="button" className="button secondary" onClick={() => void copyVersionInfo()}>
            {copied ? 'Copied' : 'Copy version info'}
          </button>
        </p>
      </section>
      <UpdateCard version={meta.data?.version ?? ''} onUpdateHandoff={onUpdateHandoff} />
      <section className="about-card" style={{ marginTop: 'var(--space-lg)' }}>
        <LanguageCoverage />
      </section>
    </div>
  );
}

type UpdateState =
  | { phase: 'idle' }
  | { phase: 'checking' }
  | { phase: 'current'; latest: string }
  | { phase: 'available'; latest: string; releaseUrl: string; notes: string }
  | { phase: 'applying'; latest: string; notes: string }
  | { phase: 'error'; message: string };

/**
 * In-app updater. The check respects offline mode server-side; applying stages
 * the official installer detached, then hands off to the app shell, which
 * stops this app (the installer waits for our exit via -WaitForCloseSeconds)
 * and shows the update screen. The staged launcher relaunches the new version
 * when the install finishes.
 */
const NEWLINE = '\n';

function UpdateCard({ version, onUpdateHandoff }: { version: string; onUpdateHandoff?: (version: string) => void }) {
  const [state, setState] = useState<UpdateState>({ phase: 'idle' });
  async function check() {
    setState({ phase: 'checking' });
    try {
      const result = await api.checkUpdate();
      setState(result.available
        ? { phase: 'available', latest: result.latest, releaseUrl: result.release_url, notes: result.release_notes }
        : { phase: 'current', latest: result.latest });
    } catch (error) {
      setState({ phase: 'error', message: error instanceof Error ? error.message : 'Could not check for updates.' });
    }
  }
  async function apply() {
    if (state.phase !== 'available') return;
    setState({ phase: 'applying', latest: state.latest, notes: state.notes });
    try {
      await api.applyUpdate();
      // The shell swaps the whole UI to the update screen and stops the
      // server; the installer waits up to 60s for that exit before swapping
      // the binary, then the staged launcher reopens the new version.
      if (onUpdateHandoff) onUpdateHandoff(state.latest);
      else await api.stopServer().catch(() => undefined);
    } catch (error) {
      setState({ phase: 'error', message: error instanceof Error ? error.message : 'Could not start the updater.' });
    }
  }
  return <section className="about-card" aria-label="Updates" style={{ marginTop: 'var(--space-lg)' }}>
    <h2>Updates {version ? <span className="badge">v{version} installed</span> : null}</h2>
    <p>Checks GitHub releases for a newer Blunt Code. Updating closes the app, installs the official package, and reopens the new version automatically; your data stays local.</p>
    <div aria-live="polite">
      {state.phase === 'idle' && <p style={{ margin: 'var(--space-md) 0 0' }}><button type="button" className="button" onClick={() => void check()} disabled={!version}>Check for updates</button></p>}
      {state.phase === 'checking' && <p className="muted" style={{ margin: 'var(--space-md) 0 0' }}>Checking GitHub releases...</p>}
      {state.phase === 'current' && <p style={{ margin: 'var(--space-md) 0 0' }}>You are on the latest release ({state.latest}). <button type="button" className="button secondary" onClick={() => void check()}>Check again</button></p>}
      {(state.phase === 'available' || state.phase === 'applying') && <div style={{ margin: 'var(--space-md) 0 0', display: 'grid', gap: 'var(--space-sm)' }}>
        <p><strong>v{state.latest} is available.</strong>{state.notes ? <span className="muted"> {state.notes.split(NEWLINE).find((line: string) => line.trim()) ?? ''}</span> : null}</p>
        {state.phase === 'available'
          ? <p style={{ display: 'flex', gap: 'var(--space-sm)', flexWrap: 'wrap' }}>
            <button type="button" className="button" onClick={() => void apply()}>Update now</button>
            <a className="button secondary" href={state.releaseUrl} target="_blank" rel="noreferrer">Release notes</a>
          </p>
          : <p className="muted">Installer launched. Blunt Code closes, installs the update, and reopens on its own.</p>}
      </div>}
      {state.phase === 'error' && <p style={{ margin: 'var(--space-md) 0 0' }} role="alert">{state.message} <button type="button" className="button secondary" onClick={() => void check()}>Try again</button></p>}
    </div>
  </section>;
}
