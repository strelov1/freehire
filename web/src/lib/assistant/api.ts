// Fetch helpers for the assistant. The agent now runs inside the freehire
// backend, so everything is same-origin under `/api/v1/assistant` and the session
// cookie authenticates it — there is no separate agent service, no cross-origin
// WebSocket, and no credential to hand anywhere.

import type { SessionSummary, StoredMessage } from './wire';
import type { ChatPreset } from './presets';

const BASE = '/api/v1/assistant';

/** One conversation plus its stored transcript. */
export interface SessionTranscript {
  session: SessionSummary;
  messages: StoredMessage[];
}

/** Thrown when a conversation is not the caller's to open: deleted, or someone
 *  else's (the API reports both as 404 so ids stay unprobeable). Carried as its
 *  own type so the UI can tell a dead link apart from a broken assistant without
 *  matching on an error message. */
export class SessionNotFound extends Error {
  constructor() {
    super('session not found');
    this.name = 'SessionNotFound';
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${BASE}${path}`, {
    credentials: 'include',
    headers: { 'content-type': 'application/json' },
    ...init,
  });
  if (res.status === 404) throw new SessionNotFound();
  if (!res.ok) {
    throw new Error(`assistant request failed (${res.status})`);
  }
  if (res.status === 204) return undefined as T;
  const body = (await res.json()) as { data: T };
  return body.data;
}

/** Start a new unbound conversation: a general chat, or the experience interviewer.
 *  Tailoring conversations are created by the tailoring bootstrap instead, which knows
 *  the CV and vacancy to bind them to — a preset that binds cannot be minted here. */
export function createSession(preset: ChatPreset = 'chat'): Promise<SessionSummary> {
  const query = preset === 'chat' ? '' : `?preset=${preset}`;
  return request<SessionSummary>(`/sessions${query}`, { method: 'POST', body: '{}' });
}

/** Start a rehearsal for one application. The vacancy is named by the client because the
 *  binding is one the caller already holds — the backend resolves it through their own
 *  application and answers a vacancy they never applied to as not found. */
export function createRehearsal(slug: string): Promise<SessionSummary> {
  return request<SessionSummary>(`/sessions?preset=interview&job=${encodeURIComponent(slug)}`, {
    method: 'POST',
    body: '{}',
  });
}

/** The caller's conversations, most recently active first. */
export function listSessions(): Promise<SessionSummary[]> {
  return request<SessionSummary[]>('/sessions');
}

/** One conversation with its full transcript, for replay. */
export function getSession(id: string): Promise<SessionTranscript> {
  return request<SessionTranscript>(`/sessions/${encodeURIComponent(id)}`);
}

/** Up to three questions the caller might ask next, drawn from the conversation's most
 *  recent exchange.
 *
 *  A POST despite reading nothing: it spends a model call, and a GET is the one method
 *  every prefetcher and browser feels free to issue twice. The server answers an empty
 *  list rather than an error for everything that can go wrong on its side, so a
 *  rejection here means the request itself failed — and the caller's answer to that is
 *  silence, not a message. */
export function suggestFollowUps(id: string): Promise<string[]> {
  return request<{ followups: string[] }>(`/sessions/${encodeURIComponent(id)}/followups`, {
    method: 'POST',
    body: '{}',
  }).then((d) => d.followups ?? []);
}

/** Delete one of the caller's conversations. */
export function deleteSession(id: string): Promise<void> {
  return request<void>(`/sessions/${encodeURIComponent(id)}`, { method: 'DELETE' });
}
