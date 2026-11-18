// Carrying what the onboarding flow found into an account, once one exists: the CV, the
// search profile built from the answers, the profile alert over email, the "in your way"
// answers (the onboarding survey) and the onboarding mark. Each goes through the endpoint that already does it for a signed-in account, so
// nothing about the CV is held server-side before sign-up (design D6).
//
// The steps are independent on purpose. The account exists by now and stays signed in
// whatever fails; a failure in one step must not keep the others from running, and the
// visitor is told exactly which part did not save. The stores behind each step are wired
// in by the page — this module only orders the steps and reads their outcomes.

import { asProfile, profileFromAnswers, type Answers, type ProfileDraft } from './onboardingFlow';
import { JOB_CHALLENGE_OTHER } from './surveyOptions';
import type { OnboardingParse, SurveyAnswers, UserProfile } from './types';

/** What the flow holds: the CV as the visitor dropped it, what was read and the answers. */
export interface CarryInput {
  file: Blob;
  fileName: string;
  parse: OnboardingParse;
  answers: Answers;
}

export interface CarryDeps {
  uploadCv(file: File): Promise<unknown>;
  saveProfile(draft: ProfileDraft): Promise<UserProfile>;
  enableAlert(profile: UserProfile): Promise<void>;
  completeOnboarding(): Promise<void>;
  saveSurvey(patch: Partial<SurveyAnswers>): Promise<unknown>;
}

/** The steps a visitor can be told about. Marking onboarding complete is not one of them:
 *  a failed mark only means the flow is offered once more. */
export type CarryStep = 'cv' | 'profile' | 'alert' | 'survey';
const CARRY_STEPS: CarryStep[] = ['cv', 'profile', 'alert', 'survey'];

/** The survey patch for the challenge answers, or null when nothing was picked. The note
 *  goes only beside `other`: the server refuses it anywhere else. */
function surveyPatch(answers: Answers): Partial<SurveyAnswers> | null {
  const picked = answers.challenge ?? [];
  if (picked.length === 0) return null;
  const note = answers.challengeNote?.trim();
  return picked.includes(JOB_CHALLENGE_OTHER) && note
    ? { biggest_challenges: picked, biggest_challenge_note: note }
    : { biggest_challenges: picked };
}

/** Run `steps` (every step by default) and return the ones that failed. */
export async function carryInto(
  input: CarryInput,
  deps: CarryDeps,
  steps: CarryStep[] = CARRY_STEPS,
): Promise<CarryStep[]> {
  const draft = profileFromAnswers(input.parse, input.answers);
  const run = async (step: CarryStep, fn: () => Promise<unknown>): Promise<CarryStep | null> => {
    if (!steps.includes(step)) return null;
    try {
      await fn();
      return null;
    } catch {
      return step;
    }
  };

  const cv = run('cv', () =>
    deps.uploadCv(new File([input.file], input.fileName, { type: input.file.type || 'application/pdf' })),
  );
  // The alert follows the profile as the server saved it; when the save failed, it is still
  // built from the same answers, so it alerts on the search the visitor was shown.
  let saved: UserProfile | null = null;
  const profileThenAlert = (async () => [
    await run('profile', async () => {
      saved = await deps.saveProfile(draft);
    }),
    await run('alert', () => deps.enableAlert(saved ?? asProfile(draft))),
  ])();
  const patch = surveyPatch(input.answers);
  const survey = patch ? run('survey', () => deps.saveSurvey(patch)) : Promise.resolve(null);
  const complete = deps.completeOnboarding().catch(() => {});

  const [cvFailed, [profileFailed, alertFailed], surveyFailed] = await Promise.all([
    cv,
    profileThenAlert,
    survey,
    complete,
  ]);
  return [cvFailed, profileFailed, alertFailed, surveyFailed].filter((s): s is CarryStep => s !== null);
}

const STEP_NAMES: Record<CarryStep, string> = {
  cv: 'your CV',
  profile: 'your search profile',
  alert: 'the email alert',
  survey: "your answers about what's in your way",
};

/** The sentence telling the visitor which parts did not save. */
export function carryFailureMessage(failed: CarryStep[]): string {
  const names = failed.map((s) => STEP_NAMES[s]);
  const list = names.length <= 1 ? (names[0] ?? '') : `${names.slice(0, -1).join(', ')} and ${names.at(-1)}`;
  return `Your account is ready, but we couldn't save ${list}.`;
}
