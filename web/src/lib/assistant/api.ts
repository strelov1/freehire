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

/** Start a conversation held against one application: a rehearsal before the interview,
 *  a debrief after it. The vacancy is named by the client because the binding is one the
 *  caller already holds — the backend resolves it through their own application and
 *  answers a vacancy they never applied to as not found. */
function createApplicationSession(preset: string, slug: string): Promise<SessionSummary> {
  return request<SessionSummary>(
    `/sessions?preset=${preset}&job=${encodeURIComponent(slug)}`,
    { method: 'POST', body: '{}' },
  );
}

/** Rehearse the interview that is coming. */
export function createRehearsal(slug: string): Promise<SessionSummary> {
  return createApplicationSession('interview', slug);
}

/** Review the interview that already happened. */
export function createDebrief(slug: string): Promise<SessionSummary> {
  return createApplicationSession('debrief', slug);
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

/** Thrown when the deployment has no Realtime gateway configured (501). Its own type
 *  for the same reason TranscriptionUnavailable exists: the answer is to remove the
 *  voice-mode control, not to report a failure — the feature is absent here. */
export class VoiceModeUnavailable extends Error {
  constructor() {
    super('voice mode is not configured');
    this.name = 'VoiceModeUnavailable';
  }
}

/** A one-call Realtime credential: the ephemeral secret, and which model it was
 *  minted for. The model rides along because the WebRTC SDP exchange names it on its
 *  own URL — the browser has no other way to learn which one this deployment runs,
 *  and hardcoding it here would drift silently from the backend's REALTIME_MODEL. */
export interface VoiceToken {
  value: string;
  model: string;
}

/** Mint a short-lived Realtime API client secret scoped to one interview session. The
 *  browser takes this straight to OpenAI over WebRTC — it never transits our backend
 *  again after this call. */
export async function mintVoiceToken(sessionId: string): Promise<VoiceToken> {
  const res = await fetch(`${BASE}/sessions/${encodeURIComponent(sessionId)}/voice-token`, {
    method: 'POST',
    credentials: 'include',
  });
  if (res.status === 501) throw new VoiceModeUnavailable();
  if (res.status === 404) throw new SessionNotFound();
  if (!res.ok) throw new Error(`Could not start voice mode (${res.status}).`);
  const body = (await res.json()) as { data: VoiceToken };
  return body.data;
}

/** Append one completed voice-mode turn to a session's transcript, so it reads the
 *  same afterward whether the exchange was typed or spoken. */
export function appendVoiceTurn(
  sessionId: string,
  role: 'user' | 'assistant',
  content: string,
): Promise<void> {
  return request<void>(`/sessions/${encodeURIComponent(sessionId)}/voice-turns`, {
    method: 'POST',
    body: JSON.stringify({ role, content }),
  });
}
