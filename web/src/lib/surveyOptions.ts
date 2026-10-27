// The labels for the onboarding survey's challenge vocabulary.
//
// The VALUES here must match internal/dict/vocab exactly — the server validates against
// that list and rejects anything else, so a typo in a value is a step that can never be
// answered. `surveyOptions.test.ts` locks the list against the one generated from the server's.
//
// The labels are the wizard's alone: the server never sends prose, and it should not — a
// vocabulary member is an identifier, and how it is worded to a candidate is a product
// decision that changes far more often than the value does.

export interface SurveyOption {
  value: string;
  label: string;
}

/** The things in the candidate's way; several may be picked. The most common answers on
 *  prod (2026-10) lead, and the ones the old free-text "other" answers kept asking for sit
 *  beside their nearest neighbour. `english` is worded "Language requirements" since
 *  the tour: answers from before it mean English only. */
export const JOB_CHALLENGE_OPTIONS: SurveyOption[] = [
  { value: 'recruiter_contact', label: 'Getting replies and interviews' },
  { value: 'job_fit', label: 'Finding jobs that fit me' },
  { value: 'remote_flexible', label: 'Finding remote or flexible work' },
  { value: 'working_abroad', label: 'Moving to another country' },
  { value: 'visa_sponsorship', label: 'Getting visa sponsorship' },
  { value: 'technical_interviews', label: 'Passing interviews' },
  { value: 'english', label: 'Language requirements' },
  { value: 'first_job', label: 'Getting my first job' },
  { value: 'just_starting', label: "Nothing yet — I'm just starting" },
  { value: 'other', label: 'Something else' },
];

/** The vocabulary member that admits a written note. Mirrors vocab.JobChallengeOther; the
 *  note field is revealed only while this value is picked, and the server rejects a note
 *  sent without it. */
export const JOB_CHALLENGE_OTHER = 'other';

/** The answer that nothing is hard yet. Mirrors vocab.JobChallengeJustStarting: it stands
 *  alone, so picking it clears the rest and picking anything else clears it. */
export const JOB_CHALLENGE_JUST_STARTING = 'just_starting';
