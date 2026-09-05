// Which questions the onboarding wizard asks this account, and in what order.
//
// Pure and Svelte-free (mirrors onboarding.ts / facetModel.ts), so the rule that decides
// whether an existing account is asked something is unit-testable in plain Node rather than
// only observable by signing in as somebody.
//
// The wizard runs ONCE per account — `users.onboarding_completed_at` is set when it ends,
// and a marked account is never routed back. That is why this trims conservatively: asking
// a question the candidate has effectively answered costs one skippable screen in a
// one-time run, while wrongly deciding a question is answered means it is never asked at
// all.

/** One screen of the wizard. */
export type StepKind =
  | 'cv'
  | 'confirm'
  | 'experience'
  | 'skills'
  | 'location'
  | 'money'
  | 'stage'
  | 'challenge';

/** Every step, in the order they are asked.
 *
 *  Ordered by what the answer DOES, not by how easy it is to answer: the steps whose
 *  answers change what the candidate is shown (their role, skills, geography, salary floor)
 *  come first, and the two that only describe them to us come last. A run abandoned
 *  half-way has then answered the questions worth having. */
export const ORDERED_STEPS: StepKind[] = [
  'cv',
  'confirm',
  'experience',
  'skills',
  'location',
  'money',
  'stage',
  'challenge',
];

/** What this account has already told us, one flag per step. Assembled by the page from the
 *  profile, résumé, screening answers and survey it has loaded — kept as plain booleans so
 *  the ordering rule below has nothing to know about any of those shapes. */
export interface OnboardingAnswered {
  hasResume: boolean;
  hasSpecializations: boolean;
  hasProfileLinks: boolean;
  hasTotalYears: boolean;
  hasSkills: boolean;
  hasLocation: boolean;
  /** The two money questions are tracked apart because they live in different stores and
   *  are independently answerable — see the AND rule in stepIsUnanswered. */
  hasDesiredSalary: boolean;
  hasCurrentIncome: boolean;
  hasStage: boolean;
  hasChallenge: boolean;
}

/** Whether a given step still has something to ask.
 *
 *  Two steps carry two independent questions each, and both count as answered only once
 *  BOTH are stored: `confirm` (specializations and profile links) and `money` (the desired
 *  salary, which lives in the screening answers, and today's income, which lives in the
 *  survey). Calling either done on one half is how an existing account goes from never
 *  having been asked for its LinkedIn — or what it earns — to never being asked at all. */
function stepIsUnanswered(step: StepKind, a: OnboardingAnswered): boolean {
  switch (step) {
    case 'cv':
      return !a.hasResume;
    case 'confirm':
      return !a.hasSpecializations || !a.hasProfileLinks;
    case 'experience':
      return !a.hasTotalYears;
    case 'skills':
      return !a.hasSkills;
    case 'location':
      return !a.hasLocation;
    case 'money':
      return !a.hasDesiredSalary || !a.hasCurrentIncome;
    case 'stage':
      return !a.hasStage;
    case 'challenge':
      return !a.hasChallenge;
  }
}

/** The steps this account is actually shown: every step it has not answered, in canonical
 *  order. An account that has answered everything gets an empty plan — the caller treats
 *  that as "nothing to ask" and marks onboarding complete without rendering a screen. */
export function plannedSteps(answered: OnboardingAnswered): StepKind[] {
  return ORDERED_STEPS.filter((step) => stepIsUnanswered(step, answered));
}
