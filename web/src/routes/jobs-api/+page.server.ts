import { serverApi } from '#lib/server/api.js';
import type { PageServerLoad } from './$types';

// The landing quotes live figures — catalogue scale and the plans' prices and limits — from
// the same endpoints /about and /pricing read, so a number here can never be one the API
// does not stand behind. Loaded on the server so the HTML a crawler sees carries them.
//
// Both reads degrade on their own: the page still renders without scale (static wording) or
// without plans (no price cards), because the pitch and the docs links are the point.
export const load: PageServerLoad = async ({ fetch }) => {
  const api = serverApi(fetch);
  const [scale, plans] = await Promise.all([
    api.catalogScale().catch(() => null),
    api.plans().catch(() => null),
  ]);
  return {
    stats: scale
      ? {
          // The de-duplicated count /jobs shows; zero means not measured.
          jobs: scale.unique_open_jobs || null,
          companies: scale.exact ? scale.companies : null,
          atsPlatforms: scale.ats_platforms || null,
        }
      : { jobs: null, companies: null, atsPlatforms: null },
    plans,
  };
};
