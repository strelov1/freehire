import { error } from '@sveltejs/kit';
import type { User } from '$lib/types';

/** Whether the mentorship marketplace exists for this visitor.
 *
 *  It ships behind the beta flag, whole: the public directory and a mentor's page as much
 *  as the cabinet. Gating only the cabinet would leave a visitor able to BOOK from a public
 *  profile and then unable to find the session again or cancel it — worse than not offering
 *  it, because the hour a mentor is holding is real.
 *
 *  Stated once and imported, rather than repeated in each `load`: this is a predicate that
 *  will be deleted in one edit when the beta ends, and four copies of it would not be. */
export function seesMentorship(user: User | null | undefined): boolean {
  return Boolean(user?.beta_tester);
}

/** Refuse a mentorship route to somebody outside the beta.
 *
 *  404, not 403: while the feature is unreleased the honest answer is that the page is not
 *  there. A 403 advertises a door, which invites the question of when it opens and puts a
 *  crawler on a URL that is meant to be invisible. */
export function requireMentorshipAccess(user: User | null | undefined): void {
  if (!seesMentorship(user)) error(404, 'Not found');
}
