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
import { api } from '../api';
import type { Route } from '../lib/router';
import type { Notice } from '../lib/notice';
import { message } from '../lib/notice';

export interface ScanActionDropdownProps {
  workspaceId: string;
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
  defaultProfile = 'standard',
  size = 'sm',
  variant = 'default',
  go,
  notify,
  onScanStarted,
  className = '',
}: ScanActionDropdownProps) {
  const [running, setRunning] = useState(false);
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
      notify?.({ kind: 'error', text: message(e) });
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
        onClick={() => void triggerScan(defaultProfile)}
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
          <DropdownMenuLabel>Scan Profiles</DropdownMenuLabel>
          <DropdownMenuItem onClick={() => void triggerScan('standard')} className="gap-2 cursor-pointer">
            <Play className="h-4 w-4 text-[var(--color-accent-strong)]" />
            <div className="flex flex-col">
              <span className="font-medium">Standard Scan</span>
              <span className="text-[11px] text-[var(--color-ink-faint)]">Fast SAST &amp; code quality</span>
            </div>
          </DropdownMenuItem>
          <DropdownMenuItem onClick={() => void triggerScan('quick')} className="gap-2 cursor-pointer">
            <Zap className="h-4 w-4 text-[var(--color-warning)]" />
            <div className="flex flex-col">
              <span className="font-medium">Quick Scan</span>
              <span className="text-[11px] text-[var(--color-ink-faint)]">Rapid linter &amp; secret check</span>
            </div>
          </DropdownMenuItem>
          <DropdownMenuItem onClick={() => void triggerScan('deep')} className="gap-2 cursor-pointer">
            <Layers className="h-4 w-4 text-[var(--color-accent)]" />
            <div className="flex flex-col">
              <span className="font-medium">Deep Scan</span>
              <span className="text-[11px] text-[var(--color-ink-faint)]">Comprehensive multi-engine</span>
            </div>
          </DropdownMenuItem>
          <DropdownMenuSeparator />
          <DropdownMenuLabel>Pentest &amp; Security</DropdownMenuLabel>
          <DropdownMenuItem onClick={() => void triggerScan('pentest')} className="gap-2 cursor-pointer text-[var(--color-danger)] focus:text-[var(--color-danger)]">
            <ShieldAlert className="h-4 w-4" />
            <div className="flex flex-col">
              <span className="font-medium font-semibold">Run Pentest Scan</span>
              <span className="text-[11px] text-[var(--color-ink-faint)]">OWASP Top 10 &amp; vulnerability audit</span>
            </div>
          </DropdownMenuItem>
          <DropdownMenuItem onClick={() => go({ page: 'pentest', id: workspaceId })} className="gap-2 cursor-pointer">
            <ShieldCheck className="h-4 w-4 text-[var(--color-accent)]" />
            <div className="flex flex-col">
              <span className="font-medium">Open Pentest Suite</span>
              <span className="text-[11px] text-[var(--color-ink-faint)]">Interactive tests &amp; DAST probe</span>
            </div>
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  );
}
