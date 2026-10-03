import { env } from '$env/dynamic/private';
import { ssrTimeoutMs } from '$lib/server/api';
import type { RequestHandler } from './$types';

// RFC 8414's authorization-server metadata, at the well-known path the spec fixes.
// An MCP client (Claude, Cursor, ChatGPT) fetches this before ever calling /authorize,
// to find the registration/authorize/token endpoints without them being hardcoded.
//
// It is a PROXY, not a copy — same reasoning as /.well-known/ojcp.json/+server.ts
// beside it. The Go service renders the document from its own route table
// (oauth_metadata.go), and it lives here because /.well-known/ is served by this
// Node process, not by the Go service: nginx routes /api/ to the backend and
// everything else here, so a Go route at this path would never receive a request.
export const GET: RequestHandler = async ({ fetch }) => {
  const base = env.API_INTERNAL_URL ?? '';

  // Bounded, like every other server-side call here — see ojcp.json/+server.ts for
  // why an unbounded SSR fetch is the kind of thing that takes the whole site down.
  let upstream: Response;
  try {
    upstream = await fetch(`${base}/api/v1/oauth/metadata/authorization-server`, {
      signal: AbortSignal.timeout(ssrTimeoutMs()),
    });
  } catch {
    return metadataUnavailable();
  }

  if (!upstream.ok) {
    return metadataUnavailable();
  }

  return new Response(await upstream.text(), {
    headers: {
      'content-type': 'application/json; charset=utf-8',
      // Public discovery document — any MCP client's browser or process may fetch it.
      'access-control-allow-origin': '*',
      'cache-control': 'public, max-age=3600',
    },
  });
};

/** Refusing rather than serving a stale or invented document: a client that cannot
 *  fetch this simply cannot discover the OAuth endpoints, which is the correct
 *  outcome when we cannot describe them truthfully. */
function metadataUnavailable(): Response {
  return new Response('{"error":"metadata unavailable"}', {
    status: 503,
    headers: {
      'content-type': 'application/json; charset=utf-8',
      'access-control-allow-origin': '*',
    },
  });
}
