import spec from '#lib/docs/generated/api-reference.data.openapi.json' with { type: 'json' };
import type { RequestHandler } from './$types';

// The Data plan counterpart of ../api-reference.openapi.json/+server.ts, consumed by
// /docs/api/data's client-side Scalar hydration.
const body = JSON.stringify(spec);

export const GET: RequestHandler = () =>
  new Response(body, {
    headers: {
      'content-type': 'application/json',
      'cache-control': 'public, max-age=3600',
    },
  });
