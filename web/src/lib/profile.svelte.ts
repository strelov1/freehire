// The signed-in user's single profile — one specialization + skills set. Read once from
// GET /api/v1/me/profile (null when the user has none yet); save (PUT) and clear (DELETE)
// call the API and keep the local copy in sync so the view updates without a reload.
//
// SSR-safe and auth-agnostic (see UserResource): the load is a browser-only no-op and
// the profile stays null for signed-out users. Mutations surface API errors to the
// caller (a bad specialization or empty skills is a 400) so the UI can show them.

import { api } from '$lib/api';
import { UserResource } from '$lib/userResource.svelte';
import type { LocationPreferences, UserProfile } from '$lib/types';

class ProfileStore extends UserResource<UserProfile | null> {
  // Reassigned (never mutated in place) on every change, so $state.raw is enough and
  // readers ($derived in the view) re-run on each new value. The base's `loaded` is
  // reactive too, so the filter modal can wait for the load to settle before showing
  // its "Apply my profile" action.
  #profile = $state.raw<UserProfile | null>(null);

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

  /** Create-or-replace the profile. `location` is the optional location-preferences block
   *  (null clears it). Throws on a bad specialization, empty skills, or an out-of-vocabulary
   *  location value (the caller shows the error). */
  async save(
    specializations: string[],
    skills: string[],
    location: LocationPreferences | null,
  ): Promise<UserProfile> {
    const row = await api.saveProfile(specializations, skills, location);
    this.#profile = row;
    this.markLoaded();
    return row;
  }

  /** Clear the profile. */
  async clear(): Promise<void> {
    await api.deleteProfile();
    this.#profile = null;
  }
}

export const profileStore = new ProfileStore();
