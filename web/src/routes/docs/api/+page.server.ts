// Server-only load: Shiki-highlight the landing page's JSON code blocks. Keeping
// this in `+page.server.ts` (not a universal `+page.ts`) means the Shiki
// dependency never reaches the client bundle.
import { OVERVIEW } from '$lib/docs/api-spec';
import { highlight } from '$lib/docs/highlight';
import type { PageServerLoad } from './$types';

export const load: PageServerLoad = async () => {
  const overviewHtml = await Promise.all(
    OVERVIEW.map((o) => (o.code ? highlight(o.code, 'json') : Promise.resolve(null))),
  );
  return { overviewHtml };
};
