import spec from '#lib/docs/generated/api-reference.openapi.json' with { type: 'json' };
import type { RequestHandler } from './$types';

// The client half of the Scalar hydration in ../docs/api/+page.svelte fetches this
// same document by URL (scalarConfig.ts's SCALAR_SPEC_URL) after the server already
// sent it once, inline, via +page.server.ts's SSR render. Serving it as a route
// rather than a plain static/ file is what lets it carry its own Cache-Control: a
// static/ file under adapter-node is served by sirv ahead of SvelteKit's router with
// no long-lived cache header (unlike hashed /_app/immutable/ assets), so every visit
// re-fetched the ~370KB spec regardless of whether anything in it had changed.
//
// The content is generated at build time (scripts/gen-openapi.mjs) and the same for
// every visitor until the next deploy restarts this process — same reasoning as
// +page.server.ts's own memoized SSR render — so a moderate max-age costs nothing.
// Not `immutable`: unlike a hashed asset, this URL's content DOES change across a
// deploy, and a visitor who loaded the page just before a release should not hold a
// stale spec for a year.
//
// Serialized once, at module scope: `spec` is as invariant as the render above, and
// JSON.stringify-ing the ~370KB object fresh on every cache-miss (every visitor's
// first request per hour, and every crawler that ignores Cache-Control) would redo
// work the process never needs to repeat.
const body = JSON.stringify(spec);

export const GET: RequestHandler = () =>
  new Response(body, {
    headers: {
      'content-type': 'application/json',
      'cache-control': 'public, max-age=3600',
    },
  });
