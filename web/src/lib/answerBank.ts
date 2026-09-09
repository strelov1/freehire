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
  return pending.filter(
    (p) =>
      !p.will_draft_at_submission && p.label.trim() !== '' && !asksAboutWorkAuthorization(p.label)
  );
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
   *  question), and a live Greenhouse posting renders four pending entries with an EMPTY
   *  label — either would collide if the identity came from the text itself, which breaks
   *  an `{#each}` key, a DOM `id`, and a per-question draft map alike. */
  key: number;
  pending: AutoApplyPreviewPending;
  /** Whether `answerableQuestions` would have kept this entry — computed once here so the
   *  filtering rule lives in exactly one place, not copied into the template. */
  answerable: boolean;
}

/** Every pending question, indexed and flagged, for the review screen to render.
 *
 *  A question the model will draft at submission stays in the list — it just renders as
 *  "filled automatically" rather than an input — because the candidate still needs to see
 *  it is accounted for. Only `answerableQuestions`'s own rule decides whether the candidate
 *  gets an input; nothing here re-derives it. */
export function pendingRows(
  pending: AutoApplyPreviewPending[] | undefined | null
): PendingAnswerRow[] {
  const answerable = new Set(answerableQuestions(pending));
  return (pending ?? []).map((p, key) => ({ key, pending: p, answerable: answerable.has(p) }));
}
