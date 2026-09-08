// Server-rendered fragment for the Scalar API reference — plain JSON in, HTML
// string out, no fetch (the generated spec is imported directly). The client
// hydrates this same fragment with the same config in +page.svelte, so the two
// never disagree about what the spec says.
import { renderApiReferenceToString } from '@scalar/server-side-rendering';
import spec from '../../../../static/api-reference.openapi.json' with { type: 'json' };
import { scalarConfigFromContent } from '$lib/docs/scalarConfig';
import type { PageServerLoad } from './$types';

// The rendered fragment depends on nothing request-specific — the spec is a
// static import — so it is the same for every visitor until the next deploy
// regenerates it, which restarts this process anyway. Memoized as a Promise
// (not just its resolved value) so concurrent first requests share one
// in-flight render instead of each re-running Scalar's Vue SSR renderer over
// the same ~370KB/254-endpoint document.
let cachedScalarHtml: Promise<string> | undefined;

export const load: PageServerLoad = async () => {
  cachedScalarHtml ??= renderApiReferenceToString(scalarConfigFromContent(spec));
  return { scalarHtml: await cachedScalarHtml };
};
