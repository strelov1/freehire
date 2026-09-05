// What each onboarding step writes, and where.
//
// Extracted from the wizard page and kept Svelte-free so the dispatch is unit-testable:
// which store a step writes to, what body it sends, and — the part that bit — what it
// must NOT drop on the way.
//
// THE RULE THIS FILE EXISTS FOR: `PUT /me/resume/contacts` is a FULL REPLACE. The handler
// decodes the body into a fresh `resume.Owned` and marshals that over
// `users.candidate_contacts` wholesale; there is no merge anywhere on that path. A partial
// body therefore does not update two fields, it deletes every other one — the candidate's
// headline, summary, languages, certifications, education and contact block included. Every
// other caller in this app spreads the current object first (CvSummaryCard, EducationCard);
// so does this one, and it threads the response back so a later step in the same run
// spreads something current rather than what was loaded three screens ago.

import type { CandidateContacts, LocationPreferences, SurveyAnswers } from './types';
import type { Answers as ScreeningAnswers } from './generated/contracts';
import { mergeProfileLinks, type ProfileLinks } from './profileLinks';
import type { StepKind } from './onboardingSteps';

/** Everything the wizard is holding, flattened. */
export interface WizardAnswers {
  specializations: string[];
  skills: string[];
  seniorities: string[];
  excludedSkills: string[];
  location: LocationPreferences | null;
  links: ProfileLinks;
  /** The account's owned résumé overlay as it currently stands. Spread into every contacts
   *  write; see the file header for why a partial body would be destructive. */
  contacts: CandidateContacts;
  /** What the candidate set on the experience step, or null if they never touched it. */
  totalYears: number | null;
  /** What the CV computed. Recorded as the candidate's own when they pass through the step
   *  showing it — agreeing with a pre-filled figure IS answering the question, and the
   *  wizard runs once, so treating it as silence loses the answer for good. */
  derivedTotalYears: number | null;
  currentIncome: number | null;
  desiredSalary: number | null;
  currency: string;
  period: string;
  stage: string | null;
  challenge: string | null;
  challengeNote: string;
}

/** The writes a step may make. Injected so the dispatch can be tested without a server. */
export interface SaveDeps {
  saveProfile: (
    specializations: string[],
    skills: string[],
    seniorities: string[],
    excludedSkills: string[],
    location: LocationPreferences | null,
  ) => Promise<unknown>;
  putResumeContacts: (contacts: CandidateContacts) => Promise<CandidateContacts>;
  updateScreeningAnswers: (patch: Partial<ScreeningAnswers>) => Promise<unknown>;
  updateSurvey: (patch: Partial<SurveyAnswers>) => Promise<unknown>;
}

/** The years figure this step should store: what the candidate set, or — when they left it
 *  alone — what the CV offered them and they did not correct. Null when there is neither. */
export function effectiveTotalYears(a: WizardAnswers): number | null {
  return a.totalYears ?? a.derivedTotalYears;
}

/** A money slider's resting position is not an answer. Zero is also not a salary, and both
 *  the survey and the screening answers reject a non-positive amount outright — so sending
 *  one is a 400 the candidate cannot get past by retrying, only by skipping the step and
 *  losing what they typed. */
function statedAmount(amount: number | null): number | null {
  return amount !== null && amount > 0 ? amount : null;
}

/** Persist the step being left, and return the owned overlay as it now stands so the caller
 *  can keep spreading something current.
 *
 *  A step whose answer is still untouched writes nothing, so skipping is genuinely free.
 *  Throws on failure — the caller keeps the candidate on the step rather than advancing
 *  past an answer that was not stored. */
export async function persistStep(
  kind: StepKind,
  a: WizardAnswers,
  deps: SaveDeps,
): Promise<CandidateContacts> {
  switch (kind) {
    case 'cv':
      // Nothing of its own: the upload and the LinkedIn import both persist as they run.
      return a.contacts;

    case 'confirm': {
      await saveProfileIfSavable(a, deps);
      const merged = mergeProfileLinks(a.links);
      if (merged.length === 0) return a.contacts;
      return deps.putResumeContacts({ ...a.contacts, links: merged });
    }

    case 'experience': {
      const years = effectiveTotalYears(a);
      if (years === null) return a.contacts;
      // total_years_set travels beside the value because 0 is a real answer ("less than a
      // year") and the server cannot otherwise tell it from "never said".
      return deps.putResumeContacts({ ...a.contacts, total_years: years, total_years_set: true });
    }

    case 'skills':
    case 'location':
      await saveProfileIfSavable(a, deps);
      return a.contacts;

    case 'money': {
      const desired = statedAmount(a.desiredSalary);
      const current = statedAmount(a.currentIncome);
      if (desired !== null) {
        await deps.updateScreeningAnswers({
          desired_salary_amount: desired,
          desired_salary_currency: a.currency,
          desired_salary_period: a.period,
        });
      }
      if (current !== null) {
        await deps.updateSurvey({
          current_income_amount: current,
          current_income_currency: a.currency,
          current_income_period: a.period,
        });
      }
      return a.contacts;
    }

    case 'stage':
      if (a.stage === null) return a.contacts;
      await deps.updateSurvey({ job_search_stage: a.stage });
      return a.contacts;

    case 'challenge': {
      if (a.challenge === null) return a.contacts;
      // The note is accepted only alongside "other", and the server rejects any other
      // pairing — so it is sent only when it belongs to the answer.
      const note = a.challenge === 'other' ? a.challengeNote.trim() : '';
      await deps.updateSurvey({
        biggest_challenge: a.challenge,
        ...(note !== '' ? { biggest_challenge_note: note } : {}),
      });
      return a.contacts;
    }
  }
}

/** The profile save endpoint requires a non-empty specialization AND skill set for the
 *  profile to exist at all, so a level-or-location-only save has nowhere to land. Skipped
 *  rather than failed: the candidate can still fill it in from /my/profile, and blocking the
 *  wizard on it would make an optional step mandatory. */
async function saveProfileIfSavable(a: WizardAnswers, deps: SaveDeps): Promise<void> {
  if (a.specializations.length === 0 || a.skills.length === 0) return;
  await deps.saveProfile(a.specializations, a.skills, a.seniorities, a.excludedSkills, a.location);
}
