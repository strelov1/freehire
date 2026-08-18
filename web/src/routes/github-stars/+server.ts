import { json } from '@sveltejs/kit';
import { githubStats } from '$lib/server/github';
import type { RequestHandler } from './$types';

// The star count for the header badge, served from our own origin so the browser
// never talks to api.github.com (see $lib/server/github for why that mattered).
//
// `stars: null` is a real answer, not an error: GitHub being unreachable or rate-
// limited must leave the badge without a number, never break the header. A 5xx
// here would put a failed request in the console — exactly what this replaced.
export const GET: RequestHandler = async ({ fetch, setHeaders }) => {
  const stats = await githubStats(fetch);
  // An hour matches the server-side memo: past it the next caller repopulates the
  // process cache anyway, so a longer browser TTL would only serve a staler number
  // than we already hold. Shared caches may hold it too — it is identical for
  // everyone and carries nothing about the visitor.
  setHeaders({ 'cache-control': 'public, max-age=3600' });
  return json({ stars: stats?.stars ?? null });
};
