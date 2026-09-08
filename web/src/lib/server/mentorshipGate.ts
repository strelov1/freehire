import { error } from '@sveltejs/kit';
import type { User } from '$lib/types';

/** Refuse a mentorship route to somebody outside the beta.
 *
 *  The marketplace ships behind the flag WHOLE: the public directory and a mentor's page as
 *  much as the cabinet. Gating only the cabinet was considered and rejected — it would leave
 *  a visitor able to BOOK from a public profile and then unable to find the session again or
 *  cancel it, and the hour a mentor is holding is real.
 *
 *  404, not 403: while the feature is unreleased the honest answer is that the page is not
 *  there. A 403 advertises a door, which invites the question of when it opens and puts a
 *  crawler on a URL that is meant to be invisible.
 *
 *  One function rather than a predicate plus a guard. The predicate had no second caller —
 *  `MentorBlock` spells its own copy on purpose, because the routes are what close the door
 *  and the block only stops offering a link that would 404 — so exporting it was an
 *  abstraction with nobody on the other end. Ending the beta is still one edit: this file. */
export function requireMentorshipAccess(user: User | null | undefined): void {
  if (!user?.beta_tester) error(404, 'Not found');
}
