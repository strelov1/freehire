import { env } from '$env/dynamic/public';
import type { SessionSummary } from './sessions';

// Fetch helpers for the agent backend (`freehire-agent`). Auth is UNIFIED with
// freehire: the agent verifies the same httpOnly `hire_token` cookie (shared JWT
// secret + `.freehire.dev` cookie domain), so `credentials: 'include'` carries
// it and there is no separate agent login.
//
// Base origin:
//  - prod: `PUBLIC_ASSISTANT_ORIGIN=https://agent.freehire.dev` — a cross-origin
//    but SAME-SITE subdomain (shared eTLD+1), so the Lax cookie is still sent;
//    the agent's nginx adds the CORS headers for the freehire.dev origin.
//  - dev: unset → the same-origin `/assistant-api` path (the Vite proxy).
const BASE = env.PUBLIC_ASSISTANT_ORIGIN || '/assistant-api';

/** The agent's WebSocket URL, derived from the same base as the fetch calls. */
export function assistantWsUrl(): string {
  if (env.PUBLIC_ASSISTANT_ORIGIN) {
    return env.PUBLIC_ASSISTANT_ORIGIN.replace(/^http/, 'ws') + '/ws';
  }
  const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
  return `${proto}//${location.host}/assistant-api/ws`;
}

/** Optional context that turns the new session into a CV-tailoring session: the agent is
 *  seeded to reframe the given CV toward a vacancy, acting on the freehire API with the
 *  short-lived key the tailoring bootstrap minted. */
export interface TailoringSession {
  cli_token: string;
  cv_id: number;
  base_cv_id: number;
}

/** Create the assistant session. For a normal chat the client sends an empty body and the
 *  backend decides everything (harness, persona, sandbox, scope). Passing `tailoring` seeds
 *  a CV-tailoring session instead (persona + FREEHIRE_TOKEN); the backend still owns the rest. */
export async function createSession(tailoring?: TailoringSession): Promise<string> {
  const res = await fetch(`${BASE}/sessions`, {
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    credentials: 'include',
    body: JSON.stringify(tailoring ? { tailoring } : {}),
  });
  if (!res.ok) throw new Error(`could not create session (${res.status})`);
  const body = (await res.json()) as { session_id?: string };
  if (!body?.session_id) throw new Error('session response missing session_id');
  return body.session_id;
}

/** List the caller's held sessions from the agent backend. The list is
 *  owner-scoped and newest-first server-side (only the caller's own sessions;
 *  orphans excluded). */
export async function listSessions(): Promise<SessionSummary[]> {
  const res = await fetch(`${BASE}/sessions`, { credentials: 'include' });
  if (!res.ok) throw new Error(`could not list sessions (${res.status})`);
  return (await res.json()) as SessionSummary[];
}

/** Delete one of the caller's sessions by id (204 on success; the backend 404s
 *  for a session the caller does not own). */
export async function deleteSession(id: string): Promise<void> {
  const res = await fetch(`${BASE}/sessions/${encodeURIComponent(id)}`, {
    method: 'DELETE',
    credentials: 'include',
  });
  if (!res.ok) throw new Error(`could not delete session (${res.status})`);
}
