import type { AutoApplyPreviewPending } from '$lib/types';

/** The pending questions worth offering the candidate an input for.
 *
 *  Kept out of JobDrawer.svelte so it unit-tests without mounting Svelte, the same
 *  convention autoApplyReview.ts and autoApplyButton.ts already follow.
 *
 *  Two are skipped. One that will be drafted at submission needs nothing from the
 *  candidate — asking anyway is work we already handle. One with no readable label cannot
 *  be answered at all: there is nothing to show, and the server refuses to key a question
 *  that folds to nothing, so an input there would collect an answer that could never be
 *  saved. */
export function answerableQuestions(
  pending: AutoApplyPreviewPending[] | undefined | null
): AutoApplyPreviewPending[] {
  if (!pending) return [];
  return pending.filter((p) => !p.will_draft_at_submission && p.label.trim() !== '');
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
