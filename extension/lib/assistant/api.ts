// Fetch helpers for freehire's assistant, ported from the web app's own client
// (`web/src/lib/assistant/api.ts` in the hire repo).
//
// One divergence, and it is the whole reason this file is not a copy: the web is
// same-origin and lets the httpOnly session cookie authenticate it. Extension code
// is not — it reaches hire by absolute origin, where that cookie is invisible to
// it — so it presents the session JWT the connect flow minted as a Bearer
// credential, exactly as it already does for `/me/autofill/run`.

import { HIRE_ORIGIN } from '../auth';
import type { SessionSummary, StoredMessage } from './wire';

const BASE = `${HIRE_ORIGIN}/api/v1/assistant`;

/** One conversation plus its stored transcript. */
export interface SessionTranscript {
  session: SessionSummary;
  messages: StoredMessage[];
}

/** Thrown when a conversation is not the caller's to open: deleted, or someone
 *  else's (the API reports both as 404 so ids stay unprobeable). Carried as its
 *  own type because the panel's response to it differs from its response to a
 *  broken assistant — a dead id starts a fresh conversation silently. */
export class SessionNotFound extends Error {
  constructor() {
    super('session not found');
    this.name = 'SessionNotFound';
  }
}

async function request<T = void>(path: string, token: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${BASE}${path}`, {
    ...init,
    headers: {
      'content-type': 'application/json',
      Authorization: `Bearer ${token}`,
      ...init?.headers,
    },
  });
  if (res.status === 404) throw new SessionNotFound();
  if (!res.ok) {
    throw new Error(`assistant request failed (${res.status})`);
  }
  if (res.status === 204) return undefined as T;
  const body = (await res.json()) as { data: T };
  return body.data;
}

/** Start a conversation for the panel. It is created under the `browse` preset —
 *  the one whose agent is given the page tool. A chat session would work here and
 *  answer perfectly well, but it would be blind. */
export function createSession(token: string): Promise<SessionSummary> {
  return request<SessionSummary>('/sessions?preset=browse', token, { method: 'POST', body: '{}' });
}

/** One conversation with its full transcript, for replay. */
export function getSession(id: string, token: string): Promise<SessionTranscript> {
  return request<SessionTranscript>(`/sessions/${encodeURIComponent(id)}`, token);
}

/** Delete one of the caller's conversations. */
export function deleteSession(id: string, token: string): Promise<void> {
  return request(`/sessions/${encodeURIComponent(id)}`, token, { method: 'DELETE' });
}
