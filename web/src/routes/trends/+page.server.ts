import { serverApi } from '$lib/server/api';
import type { PageServerLoad } from './$types';

// Server-render the daily activity series so the chart is in the initial HTML
// (usable without client JS on first paint, and indexable). The client re-fetches
// when the visitor switches the granularity toggle.
export const load: PageServerLoad = async ({ fetch }) => {
  const initial = await serverApi(fetch).jobsActivity('day');
  return { initial };
};
