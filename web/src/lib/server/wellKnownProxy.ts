import { env } from '$env/dynamic/private';
import { ssrTimeoutMs } from '$lib/server/api';

/** Proxy a public, static JSON document from the Go backend to a `/.well-known/*`
 *  path — the shape both OAuth metadata routes need and `/.well-known/ojcp.json`
 *  already established: nginx routes `/.well-known/*` to this Node process, not
 *  the Go backend, so a Go route at the well-known path itself would never
 *  receive a request.
 *
 *  Bounded like every other server-side call here — an unbounded SSR fetch is
 *  what the 2026-08-01 CLOSE-WAIT outage was made of. Refuses rather than
 *  serving a stale or invented document: a client that cannot fetch this
 *  metadata simply cannot discover the OAuth endpoints, which is the honest
 *  outcome when the backend cannot be reached. */
export async function proxyWellKnownJSON(fetch: typeof globalThis.fetch, backendPath: string): Promise<Response> {
  const base = env.API_INTERNAL_URL ?? '';
  let upstream: Response;
  try {
    upstream = await fetch(`${base}${backendPath}`, { signal: AbortSignal.timeout(ssrTimeoutMs()) });
  } catch {
    return wellKnownUnavailable();
  }
  if (!upstream.ok) return wellKnownUnavailable();

  return new Response(await upstream.text(), {
    headers: {
      'content-type': 'application/json; charset=utf-8',
      // Public discovery document — any MCP client's browser or process may fetch it.
      'access-control-allow-origin': '*',
      'cache-control': 'public, max-age=3600',
    },
  });
}

function wellKnownUnavailable(): Response {
  return new Response('{"error":"metadata unavailable"}', {
    status: 503,
    headers: {
      'content-type': 'application/json; charset=utf-8',
      'access-control-allow-origin': '*',
    },
  });
}
