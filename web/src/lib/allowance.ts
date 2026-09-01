import type { Allowance } from './types';

/** Whether today's allowance for a feature is spent. An unlimited allowance is never
 *  spent — the fair-use guard behind it refuses at the point of use rather than being a
 *  ceiling anybody is shown approaching.
 *
 *  Deliberately not exported: on its own it is the wrong question for every surface that
 *  has one. Blocking is `refuses`, and reporting how much is left is `remaining` — a bare
 *  "is it spent" has no caller that would not be a bug. */
function isSpent(a: Allowance | null | undefined): boolean {
  return !!a && !a.unlimited && a.used >= (a.limit ?? 0);
}

/** Whether the server would actually turn this action away: the allowance is spent AND the
 *  feature's ceiling is switched on.
 *
 *  Every surface that hides or disables an action asks this one, and the distinction is the
 *  whole of shadow mode. A client that blocks on `isSpent` alone refuses what the server
 *  would have allowed — and worse, it suppresses exactly the requests the shadow run exists
 *  to count, so the numbers the enforcement decision rests on come back understated.
 *
 *  `enforced` is absent from an older server's response, which reads as false: not
 *  enforcing is the safe way to be wrong, since the server still refuses for real. */
export function refuses(a: Allowance | null | undefined): boolean {
  return isSpent(a) && a?.enforced === true;
}

/** How many of today's allowance are left, or null when it is unlimited or unknown. */
export function remaining(a: Allowance | null | undefined): number | null {
  if (!a || a.unlimited) return null;
  return Math.max(0, (a.limit ?? 0) - a.used);
}

/** When today's allowance starts over, in the reader's own clock: "3:00 AM". The day is
 *  keyed in UTC, but what the reader needs is the moment it happens where they are.
 *
 *  Falls back to "tomorrow" rather than showing an invalid date — the reset instant is
 *  only ever absent when the allowance itself could not be read, and a broken timestamp
 *  in a sentence about waiting reads as a bug in the waiting. */
export function resetsAtLabel(a: Allowance | null | undefined): string {
  if (!a?.resets_at) return 'tomorrow';
  const at = new Date(a.resets_at);
  if (Number.isNaN(at.getTime())) return 'tomorrow';
  return at.toLocaleTimeString(undefined, { hour: 'numeric', minute: '2-digit' });
}
