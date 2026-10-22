/**
 * Which of the candidate's own answers on a form are worth offering to the answer
 * bank, so the next application asking the same question is filled for them.
 *
 * Pure over the fields the page reported, like `applyPlan.ts`. It decides only what
 * to OFFER: the candidate ticks what to keep, because some answers belong to one
 * employer ("agree to arbitrate", "open to our office 25% of the time") and banking
 * them silently would have the agent answer them for everyone else.
 */

import { matchFieldKey, normalizeLabel } from './form';
import type { FramedField } from './protocol';

/** One answer the panel offers to save. */
export interface AnswerOffer {
  /** The question's identity on this page, as `applyPlan`'s PlanItem keys it. */
  key: string;
  question: string;
  answer: string;
}

/** The bank's own limit (internal/candidate/answerbank.MaxAnswerLen), in characters. */
const MAX_ANSWER_LEN = 2000;

/**
 * The answered questions on the form that neither the profile nor this panel's own
 * fill accounts for.
 *
 * `autofilled` holds the normalised labels freehire wrote on this page: those came from
 * the profile or the bank already. A label the profile vocabulary recognises
 * (`matchFieldKey`) is left out too — name, email, notice period live in typed records,
 * and a free-text copy in the bank would drift from them.
 */
export function answersToOffer(fields: FramedField[], autofilled: ReadonlySet<string>): AnswerOffer[] {
  return fields
    .filter((f) => {
      const answer = f.value.trim();
      if (!answer || [...answer].length > MAX_ANSWER_LEN) return false;
      if (!f.label.trim() || matchFieldKey(f.label)) return false;
      return !autofilled.has(normalizeLabel(f.label));
    })
    .map((f) => ({ key: `${f.frame}:${f.index}`, question: f.label.trim(), answer: f.value.trim() }));
}
