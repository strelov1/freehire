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

  /** Clear the profile. */
  async clear(): Promise<void> {
    await api.deleteProfile();
    this.#profile = null;
  }
}

export const profileStore = new ProfileStore();
