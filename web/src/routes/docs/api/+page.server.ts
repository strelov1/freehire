// Server-rendered fragment for the Scalar API reference — plain JSON in, HTML
// string out, no fetch (the generated spec is imported directly). The client
// hydrates this same fragment with the same config in +page.svelte, so the two
// never disagree about what the spec says.
import { renderApiReferenceToString } from '@scalar/server-side-rendering';
import spec from '../../../../static/api-reference.openapi.json' with { type: 'json' };
import { scalarConfigFromContent } from '$lib/docs/scalarConfig';
import type { PageServerLoad } from './$types';

export const load: PageServerLoad = async () => {
  const scalarHtml = await renderApiReferenceToString(scalarConfigFromContent(spec));
  return { scalarHtml };
};
