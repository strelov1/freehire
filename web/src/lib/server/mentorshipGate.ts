import { error, redirect } from '@sveltejs/kit';
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
 *  abstraction with nobody on the other end.
 *
 *  ENDING THE BETA IS THREE EDITS, not one: this file, `inBeta` in `MentorBlock.svelte`,
 *  and `betaOnly` on `/my/mentorship` in `accountNav.ts`. They are deliberately separate —
 *  a door, an offer of the door, and a menu entry are three different things — but a
 *  comment that promised one edit would send somebody looking for a single switch that
 *  does not exist. Grep for `beta_tester` and for `betaOnly`; those three are all of it. */
export function requireMentorshipAccess(user: User | null | undefined): void {
  if (!user?.beta_tester) error(404, 'Not found');
}

/** Refuse a MENTOR-only pane to somebody who has no mentor profile.
 *
 *  A redirect rather than a 404: they are allowed in this section, just not on this pane,
 *  and the index is the part of it that is theirs. Without it the mentor-only reads throw
 *  for an account with no profile and the pane answers 500 — which is how two of them
 *  greeted anybody who simply typed the address, a tab hidden from the strip being no kind
 *  of closed door.
 *
 *  Beside `requireMentorshipAccess` and not inlined at each pane, because two callers with
 *  one rule is exactly the shape that drifts. */
export function requireMentorProfile(profile: unknown): void {
  if (!profile) redirect(303, '/my/mentorship');
}
