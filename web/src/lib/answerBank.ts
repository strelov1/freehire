import type { AutoApplyPreviewPending } from '$lib/types';

/** The question stems that mean "may you work in this posting's country".
 *
 *  This list is the TypeScript half of one rule. The Go half is
 *  `internal/api/atsapply/sensitive.go`'s `workAuthorizationTerms`, which refuses to RECALL
 *  a banked answer to such a question: the answer depends on the job's own location, one
 *  topic covers every posting worded that way, and a "Yes" banked while looking at a US
 *  posting would otherwise fill a Brazilian one at submit time, unseen. Offering an input
 *  here is what would collect that answer in the first place, so the screen refuses too.
 *
 *  The two lists cannot share a definition across languages, so each names the other — the
 *  same arrangement `bankAnswerKeyPrefix` already documents. Keep them in step.
 *
 *  Deliberately NOT the whole sensitive-terms list the Go side keeps for drafting: salary is
 *  on that list and the salary case is what the bank exists for. */
const WORK_AUTHORIZATION_TERMS = ['authoriz', 'right to work', 'sponsor', 'visa'];

/** The pending questions worth offering the candidate an input for.
 *
 *  Kept out of JobDrawer.svelte so it unit-tests without mounting Svelte, the same
 *  convention autoApplyReview.ts and autoApplyButton.ts already follow.
 *
 *  Three are skipped. One that will be drafted at submission needs nothing from the
 *  candidate — asking anyway is work we already handle. One asking about work authorization
 *  is refused for the reason WORK_AUTHORIZATION_TERMS gives. And one with no readable text
 *  cannot be answered at all: there is nothing to show. */
export function answerableQuestions(
  pending: AutoApplyPreviewPending[] | undefined | null
): AutoApplyPreviewPending[] {
  if (!pending) return [];
  return pending.filter(isAnswerable);
}

/** The rule itself, so `answerableQuestions` and `pendingRows` share it rather than one
 *  reconstructing the other's verdict. See `pendingRows` for what the reconstruction cost. */
function isAnswerable(p: AutoApplyPreviewPending): boolean {
  return rowKind(p) === 'answerable';
}

/** How one pending question renders on the review screen.
 *
 *  - `draft`: the model fills this at submission; needs nothing from the candidate.
 *  - `answerable`: offer an input (and bank what they type).
 *  - `blocked`: has real text but cannot be answered — a work-authorization question, whose
 *    correct answer depends on the posting's own country. Still rendered, as plain text
 *    naming what is blocking the application: a screen that says nothing while auto-apply
 *    quietly cannot proceed is the exact failure this feature exists to end.
 *  - `empty`: no readable text at all. Genuinely nothing to show — unlike `blocked`, which
 *    has a label, this one draws no row.
 */
type PendingRowKind = 'draft' | 'answerable' | 'blocked' | 'empty';

function rowKind(p: AutoApplyPreviewPending): PendingRowKind {
  if (p.will_draft_at_submission) return 'draft';
  if (p.label.trim() === '') return 'empty';
  if (asksAboutWorkAuthorization(p.label)) return 'blocked';
  return 'answerable';
}

/** Whether a question asks whether the candidate may work in this posting's country. */
function asksAboutWorkAuthorization(label: string): boolean {
  const lower = label.toLowerCase();
  return WORK_AUTHORIZATION_TERMS.some((term) => lower.includes(term));
}

/** One pending question paired with a stable identity and whether the candidate can
 *  answer it — what JobDrawer.svelte iterates to render the pending list. */
export interface PendingAnswerRow {
  /** The entry's position in the raw list. Never the label: two distinct questions can
   *  share text (an "Additional information" field is common to more than one ATS
   *  question), and an entry can carry no text at all — either would collide if the
   *  identity came from the text itself, which breaks an `{#each}` key, a DOM `id`, and a
   *  per-question draft map alike. */
  key: number;
  pending: AutoApplyPreviewPending;
  /** How this row renders — see `PendingRowKind`. Computed once here so the template
   *  switches on it rather than re-deriving whether a question is answerable or blocked. */
  kind: PendingRowKind;
}

/** Every pending question, indexed and kinded, for the review screen to render.
 *
 *  A question the model will draft at submission stays in the list — it just renders as
 *  "filled automatically" rather than an input — because the candidate still needs to see
 *  it is accounted for. A question that is neither drafted nor answerable (a
 *  work-authorization one) stays in the list too, as `blocked`, for the same reason: the
 *  candidate needs to see what is stopping the application, not a screen that says nothing.
 *  Only `rowKind`'s own rule decides how a row renders; nothing here re-derives it.
 *
 *  It calls that rule directly rather than asking whether `answerableQuestions` kept the
 *  entry. Membership through a `Set` of the returned entries worked only for as long as
 *  `answerableQuestions` was a `filter` handing back the very same object references: the
 *  day it mapped, spread or copied one, every row would silently become unanswerable, no
 *  input would render anywhere, and every test here would still pass, because none of them
 *  can see an object's identity. A shared predicate has nothing to drift from. */
export function pendingRows(
  pending: AutoApplyPreviewPending[] | undefined | null
): PendingAnswerRow[] {
  return (pending ?? []).map((p, key) => ({ key, pending: p, kind: rowKind(p) }));
}

/** Whether any row in the list draws anything at all. An `empty` row (no readable label)
 *  draws nothing; every other kind — `draft`, `answerable`, `blocked` — draws a line. Lives
 *  here, not in JobDrawer.svelte, so the screen never re-derives which kinds render: that
 *  re-derivation is exactly what produced the bug this function closes, where a list of
 *  only work-authorization questions rendered an empty-looking block with no explanation. */
export function hasVisibleRows(rows: PendingAnswerRow[]): boolean {
  return rows.some((row) => row.kind !== 'empty');
}
