import { serverApi } from '$lib/server/api';
import type { PageServerLoad } from './$types';

// The plan matrix is loaded on the SERVER so the comparison is in the HTML a crawler sees.
// A pricing page whose numbers arrive by a later fetch is a pricing page search engines
// index as empty.
//
// The numbers themselves come from the backend rather than living in this file. Writing
// "3 analyses a day" into the markup would make a second source of truth about what a plan
// gives, and it would drift the first time the real limit moved — silently, and in the one
// place a customer can hold us to.
export const load: PageServerLoad = async ({ fetch }) => {
  try {
    return { plans: await serverApi(fetch).plans() };
  } catch {
    // The page still renders: the comparison is the point, and a missing price is better
    // than a stale one. The buy buttons simply do not appear.
    return { plans: null };
  }
};
