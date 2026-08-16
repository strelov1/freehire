// The signed-in user's single profile — one specialization + skills set. Read once from
// GET /api/v1/me/profile (null when the user has none yet); save (PUT) and clear (DELETE)
// call the API and keep the local copy in sync so the view updates without a reload.
//
// SSR-safe and auth-agnostic (see UserResource): the load is a browser-only no-op and
// the profile stays null for signed-out users. Mutations surface API errors to the
// caller (a bad specialization or empty skills is a 400) so the UI can show them.

import { api } from '$lib/api';
import {
  withSkill,
  withSkills,
  withoutSkill,
  withAvoidedSkill,
  withoutAvoidedSkill,
  type ProfileSkillSets,
} from '$lib/profileSkills';
import { serialQueue } from '$lib/serialQueue';
import { UserResource } from '$lib/userResource.svelte';
import type { LocationPreferences, UserProfile } from '$lib/types';

class ProfileStore extends UserResource<UserProfile | null> {
  // Reassigned (never mutated in place) on every change, so $state.raw is enough and
  // readers ($derived in the view) re-run on each new value. The base's `loaded` is
  // reactive too, so the filter modal can wait for the load to settle before showing
  // its "Apply my profile" action.
  #profile = $state.raw<UserProfile | null>(null);

  // Single-skill edits go one at a time: the endpoint replaces the whole row, so two
  // claims made a moment apart would otherwise both be assembled from the same
  // pre-claim skill list and the second would drop the first.
  #queue = serialQueue();

  get profile(): UserProfile | null {
    return this.#profile;
  }

  protected load(): Promise<UserProfile | null> {
    return api.getProfile();
  }

  protected apply(row: UserProfile | null) {
    this.#profile = row;
  }

  protected clearState() {
    this.#profile = null;
  }

  /** Re-fetch the profile from the server without going through the write path — what a
   *  CV delete/upload triggers, since those change server-derived fields (`cv`) that no
   *  profile-store write method touches. Best-effort: a failure leaves the previous copy
   *  in place. */
  async refresh(): Promise<void> {
    try {
      this.#profile = await api.getProfile();
      this.markLoaded();
    } catch {
      // best-effort — keep whatever was last read.
    }
  }

  /** Create-or-replace the profile. `excludedSkills` are the skills to avoid (may be empty).
   *  `location` is the optional location-preferences block (null clears it). Throws on a bad
   *  specialization, empty skills, or an out-of-vocabulary location value (the caller shows
   *  the error). */
  async save(
    specializations: string[],
    skills: string[],
    excludedSkills: string[],
    location: LocationPreferences | null,
  ): Promise<UserProfile> {
    const row = await api.saveProfile(specializations, skills, excludedSkills, location);
    this.#profile = row;
    this.markLoaded();
    return row;
  }

  /** Add one skill to the profile — what the job-match block writes when the viewer says
   *  they have a skill it listed as missing. Also stops excluding that skill. Rejects when
   *  there is no profile to add to (the block only offers this to a profiled viewer) or
   *  when the write is refused, leaving the stored copy untouched either way. */
  addSkill(skill: string): Promise<UserProfile> {
    return this.#queue(() => this.#writeSkills((sets) => withSkill(sets, skill)));
  }

  /** Merge résumé-extracted skills into the profile, replacing its specializations with
   *  `specializations` in the same write — what a résumé upload against an already-existing
   *  profile does with both fields the extraction resolved (the set-up form instead merges
   *  both into its own unsaved fields, before there is a profile to write to). Both land in
   *  one PUT rather than two, so a specialization the extraction also found never sits in
   *  the caller's local state alone — a second, skills-only write would trigger the same
   *  reseed-from-profile the caller's own save does, discarding an unsaved specialization
   *  before the caller ever gets to persist it separately. Reads `#profile` at call time,
   *  inside the queue, so a concurrent skill edit elsewhere isn't clobbered. Rejects when
   *  there is no profile, same as `addSkill`. */
  mergeResumeExtraction(newSkills: string[], specializations: string[]): Promise<UserProfile> {
    return this.#queue(() => {
      const current = this.#profile;
      if (!current) return Promise.reject(new Error('No profile to edit.'));
      const sets = withSkills(current, newSkills);
      return this.save(specializations, sets.skills, sets.excluded_skills, current.location_preferences);
    });
  }

  /** Take one skill back out — undoing a claim. Subtracts only that skill, so a claim made
   *  after it survives. */
  removeSkill(skill: string): Promise<UserProfile> {
    return this.#queue(() => this.#writeSkills((sets) => withoutSkill(sets, skill)));
  }

  /** Record a skill as one to avoid — the match block's other answer to a missing skill. Also
   *  drops it from the held skills, so the profile never claims and avoids the same token. */
  avoidSkill(skill: string): Promise<UserProfile> {
    return this.#queue(() => this.#writeSkills((sets) => withAvoidedSkill(sets, skill)));
  }

  /** Stop avoiding a skill. Lifts the exclusion only — it does not claim the skill. */
  unavoidSkill(skill: string): Promise<UserProfile> {
    return this.#queue(() => this.#writeSkills((sets) => withoutAvoidedSkill(sets, skill)));
  }

  /** Re-save the profile with an edited specializations list — what the Roles card
   *  writes when you add or remove one. Rejects when there is no profile. */
  updateSpecializations(specializations: string[]): Promise<UserProfile> {
    return this.#queue(() => {
      const current = this.#profile;
      if (!current) return Promise.reject(new Error('No profile to edit.'));
      return this.save(specializations, current.skills, current.excluded_skills, current.location_preferences);
    });
  }

  /** Re-save the profile with an edited location-preferences block — what the Location
   *  view writes on every change. Rejects when there is no profile. */
  updateLocation(location: LocationPreferences | null): Promise<UserProfile> {
    return this.#queue(() => {
      const current = this.#profile;
      if (!current) return Promise.reject(new Error('No profile to edit.'));
      return this.save(current.specializations, current.skills, current.excluded_skills, location);
    });
  }

  /** Re-save the profile with edited skill sets. Reads `#profile` at call time — inside the
   *  queue — so it sees whatever the preceding write applied rather than a copy that
   *  predates it. */
  #writeSkills(edit: (sets: ProfileSkillSets) => ProfileSkillSets): Promise<UserProfile> {
    const current = this.#profile;
    if (!current) return Promise.reject(new Error('No profile to edit.'));
    const next = edit(current);
    return this.save(
      current.specializations,
      next.skills,
      next.excluded_skills,
      current.location_preferences,
    );
  }
}

export const profileStore = new ProfileStore();
