import { API_INTERNAL_URL } from '$app/env/private';
import { ssrTimeoutMs } from '#lib/server/api.js';

// The OJCP discovery documents live at well-known paths the specification fixes, but the Go
// service renders them: nginx routes /api/ to the backend and everything else to this Node
// process, so a Go route at /.well-known/ itself would never receive a request. That was
// found the only way it could be — deployed, with the API path answering 200 and the
// manifest answering 404. Each route here is therefore a PROXY, not a copy: writing the
// JSON out in the SPA would put a second copy of what the Go service derives where nothing
// can check it.

/** Forwards a Go-rendered discovery document, served with the content type the spec names. */
export async function proxyDiscoveryDocument(
  fetch: typeof globalThis.fetch,
  apiPath: string,
  contentType: string,
): Promise<Response> {
  const base = API_INTERNAL_URL ?? '';

  // Bounded, like every other server-side call here. An unbounded SSR fetch is what the
  // 2026-08-01 outage was made of: the backend hung, Node held its sockets open
  // indefinitely, CLOSE-WAIT filled the accept queue and nginx answered 504 for the whole
  // site. `createApi` sets this deadline on every typed call; this forwards a raw document
  // instead, so it sets it itself — from the same figure, not a second one.
  let upstream: Response;
  try {
    upstream = await fetch(`${base}${apiPath}`, { signal: AbortSignal.timeout(ssrTimeoutMs()) });
  } catch {
    return unavailable(503);
  }

  // A 404 is an answer, not a failure: a deployment with no signing key publishes no key
  // set, and an agent must be able to tell "there is none" from "the provider is down".
  if (upstream.status === 404) {
    return unavailable(404);
  }
  if (!upstream.ok) {
    return unavailable(503);
  }

  return new Response(await upstream.text(), {
    headers: {
      'content-type': contentType,
      // Readable from any origin. These are PUBLIC discovery documents — the whole point is
      // that a stranger's agent finds them — and the registry's own submission form fetches
      // the manifest from a browser on ojcp.dev, where a response with no CORS header is
      // discarded before the page can read it. curl saw 200 the entire time; the browser
      // saw nothing, which is exactly how this was missed.
      //
      // Nothing here is private or per-caller: the same bytes go to everyone, so `*` costs
      // nothing. The reference provider serves the same header.
      'access-control-allow-origin': '*',
      // An agent re-reads these to discover a change. An hour is long enough to matter and
      // short enough that a corrected figure reaches agents the same day — and it is well
      // inside the 24 hours a manifest signature stays valid, so a cached copy never
      // outlives its own signature.
      'cache-control': 'public, max-age=3600',
    },
  });
}

/** Refusing rather than serving a stale or invented document: an agent that cannot fetch
 *  one simply does not rely on it, which is the correct outcome when we cannot describe
 *  ourselves truthfully. */
function unavailable(status: 404 | 503): Response {
  const error = status === 404 ? 'not published' : 'unavailable';
  return new Response(JSON.stringify({ error }), {
    status,
    headers: {
      'content-type': 'application/json; charset=utf-8',
      // On the refusal too: a browser that cannot read the error learns only that something
      // went wrong somewhere, which is the less useful half of the two answers.
      'access-control-allow-origin': '*',
    },
  });
}
