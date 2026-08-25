import { ApiError } from '../api';

export type NoticeAction = { label: string; onClick: () => void };

export type Notice = { kind: 'error' | 'info' | 'success'; text: string; /** Optional inline follow-up, e.g. "View scan" after starting one. */ action?: NoticeAction } | null;

export function message(error: unknown) {
  return error instanceof ApiError ? `${error.code}: ${error.message}` : error instanceof Error ? error.message : 'An unexpected error occurred.';
}
