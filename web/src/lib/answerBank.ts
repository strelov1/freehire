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
