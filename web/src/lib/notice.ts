import { ApiError } from '../api';

export type Notice = { kind: 'error' | 'info'; text: string } | null;

export function message(error: unknown) {
  return error instanceof ApiError ? `${error.code}: ${error.message}` : error instanceof Error ? error.message : 'An unexpected error occurred.';
}
