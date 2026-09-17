import { env } from '$env/dynamic/private';
import { ssrTimeoutMs } from '$lib/server/api';
import type { RequestHandler } from './$types';

// The OJCP manifest, at the well-known path the specification fixes. This is the one
// document an OJCP provider MUST serve, and how an agent discovers everything else.
//
// It is a PROXY, not a copy. The Go service renders the manifest from its own
// configuration — `tools` from what the MCP server registered, `rate_limits` from the
// constant the limiter enforces — because the spec makes both claims binding. Writing the
// JSON out here instead would put a second copy of those figures where nothing can check
// them against the router that has to honour them.
//
// It lives in the SPA because /.well-known/ is served by this Node process, not by the Go
// service: nginx routes /api/ to the backend and everything else here, so a Go route at
// this path would never receive a request. That was found the only way it could be —
// deployed, with the API path answering 200 and the manifest answering 404.
export const GET: RequestHandler = async ({ fetch }) => {
  const base = env.API_INTERNAL_URL ?? '';

  // Bounded, like every other server-side call here. An unbounded SSR fetch is what the
  // 2026-08-01 outage was made of: the backend hung, Node held its sockets open
  // indefinitely, CLOSE-WAIT filled the accept queue and nginx answered 504 for the whole
  // site. `createApi` sets this deadline on every typed call; this route forwards a raw
  // document instead, so it sets it itself — from the same figure, not a second one.
  let upstream: Response;
  try {
    upstream = await fetch(`${base}/api/v1/ojcp/manifest`, {
      signal: AbortSignal.timeout(ssrTimeoutMs()),
    });
  } catch {
    return manifestUnavailable();
  }

  if (!upstream.ok) {
    return manifestUnavailable();
  }

  return new Response(await upstream.text(), {
    headers: {
      // The spec requires this content type by name.
      'content-type': 'application/json; charset=utf-8',
      // Readable from any origin. A manifest is a PUBLIC discovery document — the whole
      // point is that a stranger's agent finds it — and the registry's own submission form
      // fetches it from a browser on ojcp.dev, where a response with no CORS header is
      // discarded before the page can read it. curl saw 200 the entire time; the browser
      // saw nothing, which is exactly how this was missed.
      //
      // Nothing here is private or per-caller: the same bytes go to everyone, so `*` costs
      // nothing. The reference provider serves the same header.
      'access-control-allow-origin': '*',
      // An agent re-reads the manifest to discover a change, and the figures in it move
      // only with a deploy. An hour is long enough to matter and short enough that a
      // corrected rate limit reaches agents the same day.
      'cache-control': 'public, max-age=3600',
    },
  });
};

/** Refusing rather than serving a stale or invented manifest: an agent that cannot fetch
 *  this document simply does not treat us as a provider, which is the correct outcome when
 *  we cannot describe ourselves truthfully. */
function manifestUnavailable(): Response {
  return new Response('{"error":"manifest unavailable"}', {
    status: 503,
    headers: {
      'content-type': 'application/json; charset=utf-8',
      // On the refusal too: a browser that cannot read the error learns only that something
      // went wrong somewhere, which is the less useful half of the two answers.
      'access-control-allow-origin': '*',
    },
  });
}
