import { useState } from 'react';
import { Button } from './ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from './ui/dropdown-menu';
import { ChevronDown, Play, Sparkles, Zap, ShieldAlert, ShieldCheck, Layers } from 'lucide-react';
import { ApiError, api } from '../api';
import type { Route } from '../lib/router';
import type { Notice } from '../lib/notice';
import { message } from '../lib/notice';
import { ConfirmationDialog } from './dialogs';

export interface ScanActionDropdownProps {
  workspaceId: string;
  /** Shown in the confirmation dialog so the user knows exactly what is about to run; optional because not every caller has the display name. */
  workspaceName?: string;
  defaultProfile?: string;
  size?: 'default' | 'sm' | 'lg' | 'icon';
  variant?: 'default' | 'primary' | 'outline' | 'secondary' | 'ghost';
  go: (r: Route) => void;
  notify?: (n: Notice) => void;
  onScanStarted?: (scanId: string) => void;
  className?: string;
}

export function ScanActionDropdown({
  workspaceId,
  workspaceName,
  defaultProfile = 'standard',
  size = 'sm',
  variant = 'default',
  go,
  notify,
  onScanStarted,
  className = '',
}: ScanActionDropdownProps) {
  const [running, setRunning] = useState(false);
  // Profile picked but not yet confirmed: a scan takes minutes, so one misclick
  // must open a confirmation instead of starting the job outright.
  const [pendingProfile, setPendingProfile] = useState<string | null>(null);
  const isFullWidth = className.includes('w-full');

  async function triggerScan(profile: string) {
    if (running) return;
    setRunning(true);
    try {
      const active = await api.startScan(workspaceId, profile);
      notify?.({ kind: 'info', text: `${profile.charAt(0).toUpperCase() + profile.slice(1)} scan initiated.` });
      onScanStarted?.(active.id);
      go({ page: 'scan', id: active.id });
    } catch (e) {
      // A scan already running for this workspace is not an error to fight —
      // the 409 carries the active scan id, so offer to open it instead of
      // leaving the user with a bare toast.
      if (e instanceof ApiError && e.code === 'SCAN_ALREADY_ACTIVE' && typeof e.details?.scan_id === 'string') {
        const activeId = e.details.scan_id as string;
        notify?.({
          kind: 'info',
          text: 'A scan is already running for this workspace.',
          action: { label: 'View scan', onClick: () => go({ page: 'scan', id: activeId }) },
        });
      } else {
        notify?.({ kind: 'error', text: message(e) });
      }
    } finally {
      setRunning(false);
    }
  }

  return (
    <div
      className={`inline-flex items-stretch rounded-[var(--radius-button)] ${className}`}
      role="group"
      aria-label="Scan actions"
    >
      <Button
        size={size}
        variant={variant as never}
        disabled={running}
        onClick={() => setPendingProfile(defaultProfile)}
        className={`rounded-r-none border-r border-black/15 dark:border-white/20 gap-1.5 focus-visible:z-10 focus-visible:ring-1 focus-visible:ring-offset-0 active:scale-100 ${
          isFullWidth ? 'flex-1 justify-center' : ''
        }`}
        title={`Run ${defaultProfile} scan`}
      >
        {running ? (
          <span className="h-3.5 w-3.5 animate-spin rounded-full border-2 border-current border-t-transparent motion-reduce:animate-none" aria-hidden="true" />
        ) : (
          <Sparkles className="h-3.5 w-3.5" />
        )}
        <span>{running ? 'Scanning…' : 'Run scan'}</span>
      </Button>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            size={size}
            variant={variant as never}
            disabled={running}
            className="rounded-l-none px-2 focus-visible:z-10 focus-visible:ring-1 focus-visible:ring-offset-0 active:scale-100 shrink-0"
            aria-label="Scan options"
            title="Choose scan profile or open pentest"
          >
            <ChevronDown className="h-3.5 w-3.5" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="w-56">
          <DropdownMenuLabel>Scan profiles</DropdownMenuLabel>
          <DropdownMenuItem onClick={() => setPendingProfile('standard')} className="gap-2 cursor-pointer">
            <Play className="h-4 w-4 text-[var(--color-accent-strong)]" />
            <div className="flex flex-col">
              <span className="font-medium">Standard scan</span>
              <span className="text-[11px] text-[var(--color-ink-faint)]">Recommended — full analyzers · a few minutes</span>
            </div>
          </DropdownMenuItem>
          <DropdownMenuItem onClick={() => setPendingProfile('quick')} className="gap-2 cursor-pointer">
            <Zap className="h-4 w-4 text-[var(--color-warning)]" />
            <div className="flex flex-col">
              <span className="font-medium">Quick scan</span>
              <span className="text-[11px] text-[var(--color-ink-faint)]">Lint and secret check · usually under a minute</span>
            </div>
          </DropdownMenuItem>
          <DropdownMenuItem onClick={() => setPendingProfile('deep')} className="gap-2 cursor-pointer">
            <Layers className="h-4 w-4 text-[var(--color-accent)]" />
            <div className="flex flex-col">
              <span className="font-medium">Deep scan</span>
              <span className="text-[11px] text-[var(--color-ink-faint)]">Every analyzer incl. dependencies and containers · can take 10+ minutes</span>
            </div>
          </DropdownMenuItem>
          <DropdownMenuSeparator />
          <DropdownMenuLabel>Pentest &amp; security</DropdownMenuLabel>
          <DropdownMenuItem onClick={() => setPendingProfile('pentest')} className="gap-2 cursor-pointer text-[var(--color-danger)] focus:text-[var(--color-danger)]">
            <ShieldAlert className="h-4 w-4" />
            <div className="flex flex-col">
              <span className="font-medium font-semibold">Run pentest scan</span>
              <span className="text-[11px] text-[var(--color-ink-faint)]">OWASP Top 10 security checks</span>
            </div>
          </DropdownMenuItem>
          <DropdownMenuItem onClick={() => go({ page: 'pentest', id: workspaceId })} className="gap-2 cursor-pointer">
            <ShieldCheck className="h-4 w-4 text-[var(--color-accent)]" />
            <div className="flex flex-col">
              <span className="font-medium">Open pentest suite</span>
              <span className="text-[11px] text-[var(--color-ink-faint)]">Interactive security tests &amp; live HTTP probing (DAST)</span>
            </div>
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
      {pendingProfile !== null && (
        // tone="primary": starting a scan is a safe, cancellable action — the
        // destructive red confirm would read as "you are about to break things".
        <ConfirmationDialog
          tone="primary"
          title={`Run ${pendingProfile} scan${workspaceName ? ` on ${workspaceName}` : ''}?`}
          description={`A ${pendingProfile} scan runs the enabled analyzers over this workspace and can take several minutes. You can cancel it from the scan page while it runs.`}
          confirmLabel={`Run ${pendingProfile} scan`}
          busy={running}
          onCancel={() => setPendingProfile(null)}
          onConfirm={() => {
            const selected = pendingProfile;
            setPendingProfile(null);
            if (selected) void triggerScan(selected);
          }}
        />
      )}
    </div>
  );
}
