import spec from '#lib/docs/generated/api-reference.internal.openapi.json' with { type: 'json' };
import type { RequestHandler } from './$types';

// The internal counterpart of ../api-reference.openapi.json/+server.ts — same
// reasoning (own Cache-Control via a route rather than a raw static/ file, body
// serialized once at module scope), serving the session-cookie-only surface that
// #lib/docs/generated/api-reference.internal.openapi.json holds. Consumed by
// /docs/api/internal's client-side Scalar hydration.
const body = JSON.stringify(spec);

export const GET: RequestHandler = () =>
  new Response(body, {
    headers: {
      'content-type': 'application/json',
      'cache-control': 'public, max-age=3600',
    },
  });
