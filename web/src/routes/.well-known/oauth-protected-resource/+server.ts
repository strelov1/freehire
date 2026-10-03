import { env } from '$env/dynamic/private';
import { ssrTimeoutMs } from '$lib/server/api';
import type { RequestHandler } from './$types';

// RFC 9728's protected-resource metadata, at the well-known path the spec fixes:
// which authorization server protects the signed-in MCP server at
// /api/v1/mcp/account. Same proxy shape as oauth-authorization-server/+server.ts
// beside it — see that file for why this lives here rather than in the Go service.
export const GET: RequestHandler = async ({ fetch }) => {
  const base = env.API_INTERNAL_URL ?? '';

  let upstream: Response;
  try {
    upstream = await fetch(`${base}/api/v1/oauth/metadata/protected-resource`, {
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
      'access-control-allow-origin': '*',
      'cache-control': 'public, max-age=3600',
    },
  });
};

function metadataUnavailable(): Response {
  return new Response('{"error":"metadata unavailable"}', {
    status: 503,
    headers: {
      'content-type': 'application/json; charset=utf-8',
      'access-control-allow-origin': '*',
    },
  });
}
