// How long the onboarding wizard keeps asking whether the background CV parse has landed.
//
// A CV upload answers immediately with the dictionary-derived facets — skills, categories,
// seniority, no model involved. Everything the MODEL reads out of the same file, the profile
// links and the total years among it, is written afterwards by a background job that takes
// seconds. The wizard used to read the résumé back exactly once, the instant the CV step was
// left, which is reliably BEFORE that job has finished: the links step therefore opened empty
// for everybody, and the "review on the next step" the CV step promises was never kept.
//
// Waiting is a policy, not a mechanism, so it lives here on its own — a table that can be
// read, tested and argued about without opening the wizard.
//
// The shape is a short head and a flat tail. The head is tight because a parse that is going
// to be quick is quick, and the candidate is looking at the very next step within a second or
// two. The tail is flat and long because they have six more steps to walk through, and a link
// that arrives while they are on the salary step still reaches the box it belongs in — the
// fill only ever touches a box they have not typed in themselves.

/** What the wizard currently knows about the background CV parse.
 *
 *  'idle' covers three situations the candidate cannot tell apart and does not need to: no
 *  CV, a parse that landed, and a parse still running after the wizard stopped waiting. Only
 *  'failed' is a thing we owe them an explanation for. */
export type CvParseState = 'idle' | 'waiting' | 'failed';

/** The waits, in order, between one read of `GET /me/resume` and the next. */
const DELAYS_MS = [800, 1200, 2000, 3000, 5000, 8000, 8000, 8000, 8000, 8000, 8000, 8000];

/** The wait before poll number `attempt` (0-based), or null when the wizard should stop
 *  asking.
 *
 *  Giving up is not the same as failing. The résumé goes on being parsed server-side and its
 *  links are there on the candidate's next visit; the wizard has simply stopped holding the
 *  door open for a run that is taking longer than the wizard itself lasts. */
export function nextResumePollDelayMs(attempt: number): number | null {
  return DELAYS_MS[attempt] ?? null;
}

/** The whole wait, so the comment above, the test, and anyone tuning the table read one
 *  number rather than adding the array up by hand. */
export const RESUME_POLL_BUDGET_MS = DELAYS_MS.reduce((total, ms) => total + ms, 0);
