// The signed-in user's single profile — one specialization + skills set. Read once from
// GET /api/v1/me/profile (null when the user has none yet); save (PUT) and clear (DELETE)
// call the API and keep the local copy in sync so the view updates without a reload.
//
// SSR-safe and auth-agnostic (see UserResource): the load is a browser-only no-op and
// the profile stays null for signed-out users. Mutations surface API errors to the
// caller (a bad specialization or empty skills is a 400) so the UI can show them.

import { api } from '#lib/api.js';
import {
  withSkill,
  withSkills,
  withoutSkill,
  withAvoidedSkill,
  withoutAvoidedSkill,
  type ProfileSkillSets,
} from '#lib/profileSkills.js';
import { withExcluded, withoutExcluded } from '#lib/profileExclusions.js';
import { serialQueue } from '#lib/serialQueue.js';
import { UserResource } from '#lib/userResource.svelte.js';
import type { LocationPreferences, UserProfile } from '#lib/types.js';

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

  /** Create-or-replace the profile. `seniorities` are the desired experience levels (may be
   *  empty). `excludedSkills`/`excludedSources`/`excludedCompanies` are what to avoid (each
   *  may be empty). `location` is the optional location-preferences block (null clears it).
   *  Throws on a bad specialization, empty skills, an unknown seniority, an over-cap
   *  excluded set, or an out-of-vocabulary location value (the caller shows the error). */
  async save(
    specializations: string[],
    skills: string[],
    seniorities: string[],
    excludedSkills: string[],
    excludedSources: string[],
    excludedCompanies: string[],
    location: LocationPreferences | null,
  ): Promise<UserProfile> {
    const row = await api.saveProfile(
      specializations,
      skills,
      seniorities,
      excludedSkills,
      excludedSources,
      excludedCompanies,
      location,
    );
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
      return this.#saveFrom(current, {
        specializations,
        skills: sets.skills,
        excluded_skills: sets.excluded_skills,
      });
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

  /** Record a source (a `jobs.source` value) as one to avoid. Unlike skills, there is no
   *  "wanted sources" list to reconcile against — this only ever adds to the one list. */
  avoidSource(source: string): Promise<UserProfile> {
    return this.#queue(() => this.#writeExcludedSources((sources) => withExcluded(sources, source)));
  }

  /** Stop avoiding a source. */
  unavoidSource(source: string): Promise<UserProfile> {
    return this.#queue(() => this.#writeExcludedSources((sources) => withoutExcluded(sources, source)));
  }

  /** Record a company (by slug) as one to avoid. */
  avoidCompany(company: string): Promise<UserProfile> {
    return this.#queue(() => this.#writeExcludedCompanies((companies) => withExcluded(companies, company)));
  }

  /** Stop avoiding a company. */
  unavoidCompany(company: string): Promise<UserProfile> {
    return this.#queue(() => this.#writeExcludedCompanies((companies) => withoutExcluded(companies, company)));
  }

  /** Re-save the profile with an edited specializations list — what the Roles card
   *  writes when you add or remove one. Rejects when there is no profile. */
  updateSpecializations(specializations: string[]): Promise<UserProfile> {
    return this.#queue(() => {
      const current = this.#profile;
      if (!current) return Promise.reject(new Error('No profile to edit.'));
      return this.#saveFrom(current, { specializations });
    });
  }

  /** Re-save the profile with an edited seniority list — what the Roles card's Level row
   *  writes when you add or remove one. Rejects when there is no profile. */
  updateSeniorities(seniorities: string[]): Promise<UserProfile> {
    return this.#queue(() => {
      const current = this.#profile;
      if (!current) return Promise.reject(new Error('No profile to edit.'));
      return this.#saveFrom(current, { seniorities });
    });
  }

  /** Re-save the profile with an edited location-preferences block — what the Location
   *  view writes on every change. Rejects when there is no profile. */
  updateLocation(location: LocationPreferences | null): Promise<UserProfile> {
    return this.#queue(() => {
      const current = this.#profile;
      if (!current) return Promise.reject(new Error('No profile to edit.'));
      return this.#saveFrom(current, { location_preferences: location });
    });
  }

  /** Re-save the profile with edited skill sets. Reads `#profile` at call time — inside the
   *  queue — so it sees whatever the preceding write applied rather than a copy that
   *  predates it. */
  #writeSkills(edit: (sets: ProfileSkillSets) => ProfileSkillSets): Promise<UserProfile> {
    const current = this.#profile;
    if (!current) return Promise.reject(new Error('No profile to edit.'));
    const next = edit(current);
    return this.#saveFrom(current, { skills: next.skills, excluded_skills: next.excluded_skills });
  }

  /** Re-save the profile with an edited excluded_sources list. Mirrors `#writeSkills`. */
  #writeExcludedSources(edit: (sources: string[]) => string[]): Promise<UserProfile> {
    const current = this.#profile;
    if (!current) return Promise.reject(new Error('No profile to edit.'));
    return this.#saveFrom(current, { excluded_sources: edit(current.excluded_sources) });
  }

  /** Re-save the profile with an edited excluded_companies list. Mirrors `#writeSkills`. */
  #writeExcludedCompanies(edit: (companies: string[]) => string[]): Promise<UserProfile> {
    const current = this.#profile;
    if (!current) return Promise.reject(new Error('No profile to edit.'));
    return this.#saveFrom(current, { excluded_companies: edit(current.excluded_companies) });
  }

  /** Re-saves `current` with `overrides` applied field by field — the shared tail every
   *  write method above ends with, so `save()`'s positional argument list has exactly one
   *  place that maps it from a profile shape, not one per caller. */
  #saveFrom(
    current: UserProfile,
    overrides: Partial<
      Pick<
        UserProfile,
        | 'specializations'
        | 'skills'
        | 'seniorities'
        | 'excluded_skills'
        | 'excluded_sources'
        | 'excluded_companies'
        | 'location_preferences'
      >
    >,
  ): Promise<UserProfile> {
    const next = { ...current, ...overrides };
    return this.save(
      next.specializations,
      next.skills,
      next.seniorities,
      next.excluded_skills,
      next.excluded_sources,
      next.excluded_companies,
      next.location_preferences,
    );
  }
}

export const profileStore = new ProfileStore();
