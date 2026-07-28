// Fetch helpers for the assistant. The agent now runs inside the freehire
// backend, so everything is same-origin under `/api/v1/assistant` and the session
// cookie authenticates it — there is no separate agent service, no cross-origin
// WebSocket, and no credential to hand anywhere.

import type { SessionSummary, StoredMessage } from './wire';

const BASE = '/api/v1/assistant';

/** One conversation plus its stored transcript. */
export interface SessionTranscript {
  session: SessionSummary;
  messages: StoredMessage[];
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${BASE}${path}`, {
    credentials: 'include',
    headers: { 'content-type': 'application/json' },
    ...init,
  });
  if (!res.ok) {
    throw new Error(`assistant request failed (${res.status})`);
  }
  if (res.status === 204) return undefined as T;
  const body = (await res.json()) as { data: T };
  return body.data;
}

/** Start a new chat conversation. Tailoring conversations are created by the
 *  tailoring bootstrap, which knows the CV and vacancy to bind them to. */
export function createSession(): Promise<SessionSummary> {
  return request<SessionSummary>('/sessions', { method: 'POST', body: '{}' });
}

/** The caller's conversations, most recently active first. */
export function listSessions(): Promise<SessionSummary[]> {
  return request<SessionSummary[]>('/sessions');
}

/** One conversation with its full transcript, for replay. */
export function getSession(id: string): Promise<SessionTranscript> {
  return request<SessionTranscript>(`/sessions/${encodeURIComponent(id)}`);
}

/** Delete one of the caller's conversations. */
export function deleteSession(id: string): Promise<void> {
  return request<void>(`/sessions/${encodeURIComponent(id)}`, { method: 'DELETE' });
}
