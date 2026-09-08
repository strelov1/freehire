// Server-rendered fragment for the Scalar API reference — plain JSON in, HTML
// string out, no fetch (the generated spec is imported directly). The client
// hydrates this same fragment with the same config in +page.svelte, so the two
// never disagree about what the spec says.
import spec from '../../../../static/api-reference.openapi.json' with { type: 'json' };
import { scalarConfigFromContent } from '$lib/docs/scalarConfig';
import type { PageServerLoad } from './$types';

// @scalar/server-side-rendering is imported dynamically, inside `load`, not at
// module top level: one of its transitive dependencies (@scalar/snippetz's
// stringify-object -> is-identifier -> super-regex -> make-asynchronous)
// spins up a worker_threads.Worker as an import-time side effect. SvelteKit's
// build-time route analysis (`analyse.js`) imports every route module inside
// its OWN worker thread to inspect exports like `prerender` — a nested Worker
// created during that pass crashes with "Cannot destructure property 'mod' of
// 'threads.workerData'" because the outer worker's workerData isn't there for
// it to read. A dynamic import here means analyse.js's static import of this
// file never touches that dependency chain at all; `load()` itself still only
// runs at actual request time (or once, at prerender, on real routes), where
// there is no such nested-worker context. Confirmed by reproducing the crash
// with `pnpm run build` and clearing it with this change.

// The rendered fragment depends on nothing request-specific — the spec is a
// static import — so it is the same for every visitor until the next deploy
// regenerates it, which restarts this process anyway. Memoized as a Promise
// (not just its resolved value) so concurrent first requests share one
// in-flight render instead of each re-running Scalar's Vue SSR renderer over
// the same ~370KB/254-endpoint document.
let cachedScalarHtml: Promise<string> | undefined;

export const load: PageServerLoad = async () => {
  cachedScalarHtml ??= (async () => {
    const { renderApiReferenceToString } = await import('@scalar/server-side-rendering');
    return renderApiReferenceToString(scalarConfigFromContent(spec));
  })();
  return { scalarHtml: await cachedScalarHtml };
};
