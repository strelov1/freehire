// How complete the candidate profile is, decided in one place.
//
// Keyed by SECTION rather than by tab: the profile's routes and its content do not
// line up one-to-one (the default section holds the CV, the role and the summary),
// and a section is what a caller actually wants to report on.
//
// Pure by design (no Svelte, no network): every input is data the profile layout has
// already loaded, which is also why this needs no endpoint of its own. The one input
// that is not part of that layout's own reads — the screening answers — comes from the
// shared ScreeningStore the layout warms alongside the profile and résumé.

import type { Answers } from './generated/contracts.js';
import type { ExperienceBank, ResumeMeta, UserProfile } from './types.js';

/** The sections a profile is measured against. Location and Avoid are deliberately
 *  absent: both are optional, so an empty one must never read as a missing part of a
 *  profile. */
const SECTIONS = ['contacts', 'skills', 'experience', 'education', 'screening', 'summary'] as const;

export type CompletenessSection = (typeof SECTIONS)[number];

/** The denominator the "x/6 sections" summary uses. */
export const PROFILE_SECTION_TOTAL = SECTIONS.length;

export interface ProfileCompleteness extends Record<CompletenessSection, boolean> {
  /** How many sections are satisfied, out of `total`. */
  filled: number;
  total: number;
}

export interface ProfileCompletenessInput {
  profile: UserProfile | null;
  /** `GET /me/resume`: the contact block and the structured CV the sections read. */
  resume?: ResumeMeta | null;
  /** `GET /me/screening-answers`: the answer bank behind the Screening section. */
  screening?: Answers | null;
  /** `GET /me/experience`: the bank the Experience tab shows. */
  experience?: ExperienceBank | null;
}

function nonEmpty(value: string | undefined | null): boolean {
  return typeof value === 'string' && value.trim() !== '';
}

/** Any stated answer counts. `false` is a real answer ("no visa sponsorship needed"), so
 *  presence is tested against undefined rather than truthiness. */
function hasAnyScreeningAnswer(answers: Answers | null): boolean {
  if (!answers) return false;
  return Object.values(answers).some((value) =>
    Array.isArray(value) ? value.length > 0 : value !== undefined && value !== null,
  );
}

/** What each section needs before it reads as filled, from data already loaded. */
export function getProfileCompleteness({
  profile,
  resume = null,
  screening = null,
  experience = null,
}: ProfileCompletenessInput): ProfileCompleteness {
  // The CV structure travels beside the profile without its contact fields; fall back to
  // the résumé's own copy when the profile has no current structure.
  const cv = profile?.cv ?? resume?.structured ?? null;
  const contacts = resume?.contacts ?? null;
  const structured = resume?.structured ?? null;

  // "Name + email": the candidate-owned block wins, because the Contacts editor writes
  // it — but a CV that has been parsed and not yet edited only has the structured copy.
  const name = nonEmpty(contacts?.full_name) ? contacts?.full_name : structured?.full_name;
  const email = nonEmpty(contacts?.email) ? contacts?.email : structured?.email;

  const sections: Record<CompletenessSection, boolean> = {
    contacts: nonEmpty(name) && nonEmpty(email),
    skills: (profile?.skills.length ?? 0) > 0,
    // The bank, not the CV parse: it is what the Experience tab lists, and a role added
    // there by hand exists in no parse.
    experience: (experience?.employments.length ?? 0) > 0 || (experience?.unplaced.length ?? 0) > 0,
    // Education and the summary are candidate-owned on the contacts block and fall back to
    // the CV parse for a résumé nobody has corrected yet — unless the candidate cleared
    // the field, which the `*_set` flag records (an emptied field leaves no value behind).
    education:
      (contacts?.education?.length ?? 0) > 0 ||
      (!contacts?.education_set && (cv?.education?.length ?? 0) > 0),
    screening: hasAnyScreeningAnswer(screening),
    summary: nonEmpty(contacts?.summary) || (!contacts?.summary_set && nonEmpty(cv?.summary)),
  };

  return {
    ...sections,
    filled: SECTIONS.filter((section) => sections[section]).length,
    total: PROFILE_SECTION_TOTAL,
  };
}
