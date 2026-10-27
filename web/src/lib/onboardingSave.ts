// What each onboarding step writes, and where.
//
// Extracted from the tour page and kept Svelte-free so the dispatch is unit-testable.
// Only two steps write: the feed step saves the search profile, the challenge step the
// survey answer. Every other step shows a feature and stores nothing of its own — the CV
// upload and the alert switches persist as they run.

import type { LocationPreferences, SurveyAnswers } from './types';
import type { StepKind } from './onboardingSteps';
import { JOB_CHALLENGE_OTHER } from './surveyOptions';

/** Everything the tour is holding that a step may write. */
export interface TourAnswers {
  specializations: string[];
  skills: string[];
  seniorities: string[];
  excludedSkills: string[];
  excludedSources: string[];
  excludedCompanies: string[];
  location: LocationPreferences | null;
  challenges: string[];
  challengeNote: string;
}

/** The writes a step may make. Injected so the dispatch can be tested without a server. */
export interface SaveDeps {
  saveProfile: (
    specializations: string[],
    skills: string[],
    seniorities: string[],
    excludedSkills: string[],
    excludedSources: string[],
    excludedCompanies: string[],
    location: LocationPreferences | null,
  ) => Promise<unknown>;
  updateSurvey: (patch: Partial<SurveyAnswers>) => Promise<unknown>;
}

/** Persist the step being left. A step whose answer is untouched writes nothing, so
 *  skipping is genuinely free. Throws on failure — the caller keeps the candidate on the
 *  step rather than advancing past an answer that was not stored. */
export async function persistStep(kind: StepKind, a: TourAnswers, deps: SaveDeps): Promise<void> {
  switch (kind) {
    case 'feed':
      // The profile save endpoint requires a non-empty specialization AND skill set for
      // the profile to exist at all, so a level-or-location-only save has nowhere to land.
      // Skipped rather than failed: blocking the tour on it would make an optional step
      // mandatory, and /my/profile is still there.
      if (a.specializations.length === 0 || a.skills.length === 0) return;
      await deps.saveProfile(
        a.specializations,
        a.skills,
        a.seniorities,
        a.excludedSkills,
        a.excludedSources,
        a.excludedCompanies,
        a.location,
      );
      return;

    case 'challenge': {
      if (a.challenges.length === 0) return;
      // The note is accepted only while "other" is picked, and the server rejects it
      // otherwise — so it is sent only when it belongs to the answers.
      const note = a.challenges.includes(JOB_CHALLENGE_OTHER) ? a.challengeNote.trim() : '';
      await deps.updateSurvey({
        biggest_challenges: a.challenges,
        ...(note !== '' ? { biggest_challenge_note: note } : {}),
      });
      return;
    }

    default:
      return;
  }
}
