// What the jobs feed shows of the page it fetched. Pure, so JobsView only wires it.
import { computeClientMatch, type ClientMatch } from '#lib/jobMatch.js';
import type { Job } from '#lib/types.js';

/** Each job's match against the viewer's profile skills, by slug. */
export function clientMatchesOf(jobs: Job[], profileSkills: string[]): Map<string, ClientMatch> {
  return new Map(jobs.map((job) => [job.public_slug, computeClientMatch(job.skills ?? [], profileSkills)]));
}

/**
 * The feed minus the signed-in user's hidden jobs, and (when the match slider is active and
 * the browser applies it, `minMatch`) minus jobs below the chosen match threshold.
 * Dismissal is cross-referenced client-side against the shared dismissed set (loaded on
 * mount), mirroring the viewed/saved sets — the server search is untouched. The caller
 * re-derives when the page changes, when a hide/undo mutates the dismissed set, or when the
 * slider moves, so a hidden or filtered-out card drops (and an undone one returns)
 * instantly. A job with no skills has no percent to test (see computeClientMatch) and
 * stays, matching the card's own `no-skills` state, which shows no match at all rather
 * than a false 0%.
 *
 * Under a client threshold the list's total is this list's length, not the server's raw
 * count: that count is accurate for every facet, which all narrow the actual search, but
 * not for the slider, which only trims the already-fetched page. Showing the raw total
 * next to a shrunk list would read as a lie ("500 open jobs" over three visible cards).
 */
export function visibleFeed(
  jobs: Job[],
  matches: Map<string, ClientMatch>,
  minMatch: number | null | undefined,
  isDismissed: (slug: string) => boolean,
): Job[] {
  return jobs.filter((j) => {
    if (isDismissed(j.public_slug)) return false;
    if (minMatch != null && (j.skills ?? []).length > 0) {
      return (matches.get(j.public_slug)?.percent ?? 0) >= minMatch;
    }
    return true;
  });
}
