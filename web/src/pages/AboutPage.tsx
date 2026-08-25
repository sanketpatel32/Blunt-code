import { useEffect, useRef, useState, type ReactNode, type SVGProps } from 'react';
import { api } from '../api';
import { copyToClipboard } from '../lib/clipboard';
import { useLoad } from '../hooks/useLoad';

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

export function AboutPage() {
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
  return <div className="page narrow"><header className="page-heading"><div><p className="eyebrow">About</p><h1>Blunt Code</h1><p>A local-first static analysis application for Windows.</p></div></header><section className="about-card"><h2>Local by default <span className="badge">{meta.data?.version ? `v${meta.data.version}` : 'Version unknown'}</span></h2><p>Analyzes selected files on this computer and combines results from local analysis tools.</p><ul className="about-points" style={{ display: 'grid', gap: 'var(--space-xs)', margin: 'var(--space-lg) 0 0', padding: 0, listStyle: 'none' }}><li className="local-signal"><MonitorIcon />Local-only analysis</li><li className="local-signal"><PersonSlashIcon />No account required</li><li className="local-signal"><EyeOffIcon />No telemetry</li></ul><dl><dt>Server</dt><dd>{serverStatus}</dd><dt>API version</dt><dd>{meta.data?.api_version ?? 'Unknown'}</dd><dt>Platform</dt><dd>{platform || 'Unknown'}</dd></dl><p aria-live="polite" style={{ margin: 'var(--space-xl) 0 0' }}><button type="button" className="button secondary" onClick={() => void copyVersionInfo()}>{copied ? 'Copied' : 'Copy version info'}</button></p></section></div>;
}
