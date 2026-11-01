// Display labels for two facts the job wire shape carries only when they are known:
// what the posting says about equity, and how many questions its application form
// requires. Both are absent far more often than present, and absence means "we do not
// know", so each helper answers null for it and the caller renders nothing.

import { EQUITY_LABELS } from './labels';

/** The `apply_questions_max` bound the filter modal's "Short application" toggle writes.
 *  Measured 2026-10-31: about half of the captured forms ask three or fewer, and the
 *  share demanding a written answer steps up past three (search.ApplyQuestionsMaxParam). */
export const SHORT_APPLICATION_MAX = 3;

/** "Salary + equity" / "Equity only", or null when the posting states neither — and for
 *  a value outside the vocabulary, since a facet label is never guessed. */
export function equityLabel(equity: string | null | undefined): string | null {
  return (equity && EQUITY_LABELS[equity]) || null;
}

/** "No extra questions" / "1 question" / "N questions", or null when no form was read.
 *  0 is a real answer and must never collapse into the absent case. */
export function applyQuestionsLabel(n: number | null | undefined): string | null {
  if (n == null || !Number.isInteger(n) || n < 0) return null;
  if (n === 0) return 'No extra questions';
  return n === 1 ? '1 question' : `${n} questions`;
}

/** The filter summary's chip for an `apply_questions_max` bound: "Short" for the toggle's
 *  own bound, the bound itself for one that arrived from a link or the API. */
export function applyQuestionsBoundLabel(max: number): string {
  if (max === SHORT_APPLICATION_MAX) return 'Short';
  if (max === 0) return 'No extra questions';
  return `Up to ${applyQuestionsLabel(max) ?? max}`;
}
