// Binds the completeness rule (profileCompleteness.ts) to the stores that already hold
// its inputs, the same split accountSetup.svelte.ts keeps for the account card: the rule
// stays pure and unit-tested, this only feeds it what the profile layout has loaded.
//
// No store of its own and no endpoint — every input is a UserResource the layout warms
// anyway, so a second reader costs nothing.

import { getProfileCompleteness, type ProfileCompleteness } from './profileCompleteness';
import { experienceBankStore } from './experienceBank.svelte';
import { profileStore } from './profile.svelte';
import { resumeStore } from './resume.svelte';
import { screeningStore } from './screening.svelte';

/** The section marks and the "x/6" count, or null until every input has settled.
 *
 *  Null rather than a partial answer: every store starts empty, so a half-loaded profile
 *  looks exactly like a brand-new one, and telling somebody who has filled everything in
 *  that most sections are missing — for the moment the résumé request takes — is worse
 *  than showing the indicator a beat later. */
export function profileCompleteness(): ProfileCompleteness | null {
  if (
    !profileStore.loaded ||
    !resumeStore.loaded ||
    !screeningStore.loaded ||
    !experienceBankStore.loaded
  )
    return null;
  return getProfileCompleteness({
    profile: profileStore.profile,
    resume: resumeStore.meta,
    screening: screeningStore.answers,
    experience: experienceBankStore.bank,
  });
}
