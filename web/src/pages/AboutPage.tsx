import { api } from '../api';
import { useLoad } from '../hooks/useLoad';

export function AboutPage() {
  const meta = useLoad(api.meta, []); const health = useLoad(api.health, []);
  return <div className="page narrow"><header className="page-heading"><div><p className="eyebrow">About</p><h1>Blunt Code</h1><p>A local-first static analysis application for Windows.</p></div></header><section className="about-card"><h2>Local by default</h2><p>Blunt Code analyzes selected files on this computer and combines results from local analysis tools. It does not require an account, telemetry, or an AI provider.</p><dl><dt>Server</dt><dd>{health.loading ? 'Checking…' : health.error ? 'Unavailable' : health.data?.status ?? 'Ready'}</dd><dt>Version</dt><dd>{meta.data?.version ?? 'Unknown'}</dd><dt>API version</dt><dd>{meta.data?.api_version ?? 'Unknown'}</dd><dt>Platform</dt><dd>{[meta.data?.os, meta.data?.architecture].filter(Boolean).join(' / ') || 'Unknown'}</dd></dl></section></div>;
}
