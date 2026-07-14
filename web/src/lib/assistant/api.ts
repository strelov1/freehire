import { env } from '$env/dynamic/public';

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

/** Create the assistant session. The client sends an empty body and knows
 *  nothing about the agent's configuration — the backend decides everything
 *  (harness, persona/system prompt, the `freehire`-only sandbox, scope). */
export async function createSession(): Promise<string> {
  const res = await fetch(`${BASE}/sessions`, {
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    credentials: 'include',
    body: '{}',
  });
  if (!res.ok) throw new Error(`could not create session (${res.status})`);
  const body = (await res.json()) as { session_id?: string };
  if (!body?.session_id) throw new Error('session response missing session_id');
  return body.session_id;
}
