import { serverApi } from '$lib/server/api';
import type { LayoutServerLoad } from './$types';

// The profile is read ONCE for the whole section, here, because the tab strip's existence
// depends on it — a seeker has no mentor tabs. Every pane reaches it through `parent()`
// rather than fetching it again.
//
// Authenticated, so the session cookie is forwarded explicitly: with an absolute API base
// URL `event.fetch` does not carry it (see $lib/server/api). The `/my` layout above this
// one already refuses an anonymous visitor, so there is no gate to repeat.
//
// A caller who is not a mentor gets `profile: null` — the ordinary answer, not a failure,
// which is why `myMentorProfile` turns the 404 into null rather than leaving every caller
// to catch it.
export const load: LayoutServerLoad = async ({ fetch, request }) => {
  const api = serverApi(fetch, request.headers.get('cookie'));
  return { profile: await api.myMentorProfile() };
};
