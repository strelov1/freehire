import { serverApi } from '#lib/server/api.js';
import type { PageServerLoad } from './$types';

// The one number this page cannot get away with inventing: how many sources actually
// feed the pipeline. Reads the same published snapshot /open and /about read (never a
// live count on the request path — see the "Catalogue scale" convention in
// AGENTS.md), so this page's figure and /open's figure cannot disagree.
export const load: PageServerLoad = async ({ fetch, setHeaders }) => {
  setHeaders({ 'cache-control': 'public, max-age=300' });

  const catalog = await serverApi(fetch).catalogScale().catch(() => null);

  return {
    scale: {
      sources: catalog?.sources ?? null,
      // A degraded snapshot (`exact: false`) has no real company count — it reads
      // zero, and zero is not "we measured zero companies", so drop it like /open does.
      companies: catalog?.exact ? (catalog?.companies ?? null) : null,
    },
  };
};
