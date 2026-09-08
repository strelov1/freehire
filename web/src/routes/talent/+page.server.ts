import { serverApi } from '$lib/server/api';
import { readTalentQuery, writeTalentQuery } from '$lib/talentQuery';
import type { PageServerLoad } from './$types';

// The public Talent Network catalogue, server-rendered.
//
// The URL is the single source of truth for what is filtered: the loader reads it, the
// API is asked for exactly that, and every filter control navigates rather than mutating
// local state. That is what makes a narrowed catalogue shareable as a link and navigable
// with the back button — and, since this page is meant to be indexed, what lets a crawler
// see the same thing a visitor does.
export const load: PageServerLoad = async ({ fetch, url }) => {
  const query = readTalentQuery(url.searchParams);
  // Re-serialised from the PARSED query rather than forwarded raw, so a hand-edited URL
  // carrying a limit the API refuses to read is normalised here instead of quietly
  // widening the answer upstream.
  const search = writeTalentQuery(query);

  // Public read: no cookie forwarded.
  const page = await serverApi(fetch).listTalent(search);

  return { query, page };
};
