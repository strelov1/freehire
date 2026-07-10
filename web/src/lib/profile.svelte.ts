// The signed-in user's single profile — one specialization + skills set. Read once from
// GET /api/v1/me/profile (null when the user has none yet); save (PUT) and clear (DELETE)
// call the API and keep the local copy in sync so the view updates without a reload.
//
// SSR-safe and auth-agnostic: `ensureLoaded` is a no-op off the browser, and the profile
// stays null for signed-out users (the view gates the load on auth and renders a sign-in
// prompt instead). Mutations surface API errors to the caller (a bad specialization or
// empty skills is a 400) so the UI can show them. Mirrors savedSearches.svelte.ts.

import { browser } from '$app/environment';
import { getProfile, saveProfile, deleteProfile } from '$lib/api';
import type { UserProfile } from '$lib/types';

class ProfileStore {
  // Reassigned (never mutated in place) on every change, so $state.raw is enough and
  // readers ($derived in the view) re-run on each new value.
  #profile = $state.raw<UserProfile | null>(null);
  // Reactive so readers can distinguish "not loaded yet" from "loaded, no profile"
  // (both leave #profile null) — e.g. the filter modal hides its profile action until
  // the load settles instead of flashing the wrong affordance.
  #loaded = $state(false);
  // The in-flight load, shared so concurrent callers issue one request.
  #loading: Promise<void> | null = null;
  // Bumped by reset(); a load resolving after a reset (a same-tab user handoff) is
  // discarded instead of repopulating with the previous user's profile.
  #generation = 0;

  get profile(): UserProfile | null {
    return this.#profile;
  }

  get loaded(): boolean {
    return this.#loaded;
  }

  /** Load the profile once. Repeat calls reuse the first load (or its in-flight
   *  promise). No-op on the server. A failed load leaves the profile null. */
  async ensureLoaded(): Promise<void> {
    if (!browser || this.#loaded) return;
    if (this.#loading) return this.#loading;
    const gen = this.#generation;
    this.#loading = getProfile()
      .then((row) => {
        if (gen !== this.#generation) return; // reset() ran mid-load — discard stale row.
        this.#profile = row;
        this.#loaded = true;
      })
      .catch(() => {
        // best-effort: a failed load just means the profile stays null.
      })
      .finally(() => {
        if (gen === this.#generation) this.#loading = null;
      });
    return this.#loading;
  }

  /** Create-or-replace the profile. Throws on a bad specialization or empty skills (the
   *  caller shows the error). */
  async save(specializations: string[], skills: string[]): Promise<UserProfile> {
    const row = await saveProfile(specializations, skills);
    this.#profile = row;
    this.#loaded = true;
    return row;
  }

  /** Clear the profile. */
  async clear(): Promise<void> {
    await deleteProfile();
    this.#profile = null;
  }

  /** Drop the cached profile and the loaded flag. The view calls this when the session
   *  ends, so a different user signing in on the same tab loads their own profile. */
  reset() {
    this.#generation++;
    this.#profile = null;
    this.#loaded = false;
    this.#loading = null;
  }
}

export const profileStore = new ProfileStore();
