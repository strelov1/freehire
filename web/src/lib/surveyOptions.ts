// The labels for the onboarding survey's two closed vocabularies.
//
// The VALUES here must match internal/dict/vocab exactly — the server validates against
// that list and rejects anything else, so a typo in a value is a step that can never be
// answered. `surveyOptions.test.ts` locks each list against the strings the API documents.
//
// The labels are the wizard's alone: the server never sends prose, and it should not — a
// vocabulary member is an identifier, and how it is worded to a candidate is a product
// decision that changes far more often than the value does.

export interface SurveyOption {
  value: string;
  label: string;
}

/** How far along the candidate says their search is. Ordered by commitment: the two who are
 *  actively looking come first, so the most common answer is not at the bottom. */
export const JOB_SEARCH_STAGE_OPTIONS: SurveyOption[] = [
  { value: 'searching', label: "I'm looking for a job now and could use help" },
  { value: 'not_started', label: "I haven't started yet, but I want to begin" },
  { value: 'employed_looking', label: "I'm already working and looking for something better" },
  { value: 'exploring', label: "I'm just looking around — it's not a priority right now" },
];

/** The single biggest thing in the candidate's way. */
export const JOB_CHALLENGE_OPTIONS: SurveyOption[] = [
  { value: 'english', label: 'English' },
  { value: 'recruiter_contact', label: 'Getting recruiters to reply' },
  { value: 'working_abroad', label: 'Finding work abroad' },
  { value: 'technical_interviews', label: 'Passing technical interviews' },
  { value: 'other', label: 'Something else' },
];

/** The vocabulary member that admits a written note. Mirrors vocab.JobChallengeOther; the
 *  note field is revealed only for this value, and the server rejects a note sent with any
 *  other. */
export const JOB_CHALLENGE_OTHER = 'other';

/** The currencies a candidate may state a figure in — the same four the job filter's
 *  currency facet offers, so a stated expectation and a posting's salary are comparable
 *  without a conversion nobody has the rates for. */
export const INCOME_CURRENCIES: SurveyOption[] = [
  { value: 'USD', label: 'USD' },
  { value: 'EUR', label: 'EUR' },
  { value: 'GBP', label: 'GBP' },
  { value: 'RUB', label: 'RUB' },
];

/** Mirrors vocab.SalaryPeriodValues, minus the two nobody states a salary in when asked
 *  about their own pay. `day` and `hour` exist because a POSTING can quote them; a
 *  candidate answering "what do you earn" does not. */
export const INCOME_PERIODS: SurveyOption[] = [
  { value: 'month', label: 'per month' },
  { value: 'year', label: 'per year' },
];

/** The slider's step. Coarse on purpose: the question is what bracket someone is in, and a
 *  control that invites a to-the-dollar answer implies a precision the answer does not
 *  have. */
export const INCOME_STEP = 500;

/** The slider's ceiling, in the stated currency and period. The last stop is the maximum
 *  itself, which the UI labels as an open-ended "or more" rather than as a figure — a
 *  candidate at the top of the range has not told us their salary is exactly this. */
export const INCOME_MAX_MONTHLY = 30_000;
export const INCOME_MAX_YEARLY = 360_000;

/** The ceiling for the currently-selected period. */
export function incomeMax(period: string): number {
  return period === 'year' ? INCOME_MAX_YEARLY : INCOME_MAX_MONTHLY;
}
