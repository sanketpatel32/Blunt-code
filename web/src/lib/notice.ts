import { ApiError } from '../api';

export type NoticeAction = { label: string; onClick: () => void };

export type Notice = { kind: 'error' | 'info' | 'success'; text: string; /** Optional inline follow-up, e.g. "View scan" after starting one. */ action?: NoticeAction } | null;

export function message(error: unknown) {
  if (error instanceof ApiError) return `${error.code}: ${error.message}`;
  if (error instanceof Error) {
    // Chrome/Edge say "Failed to fetch", Safari "Load failed", Firefox "NetworkError..."
    // whenever a request never reaches the local server - almost always because the
    // app window was closed while a tab was still open.
    if (error.message === 'Failed to fetch' || error.message === 'Load failed' || error.message.startsWith('NetworkError')) {
      return "Can't reach the Blunt Code server. If the app was closed, start bluntcode.exe again - your workspaces and reports are safe.";
    }
    return error.message;
  }
  return 'An unexpected error occurred.';
}
