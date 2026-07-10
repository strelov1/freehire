// Shared base for the per-user "load once, reset on sign-out" module-singleton
// stores (savedSearches, profile, notifications, viewedJobs). Each is a module-level
// singleton that survives the soft invalidateAll() on logout, so it MUST be dropped
// when the session ends — otherwise the next user signing in on the same tab sees the
// previous user's data. This base owns the load-once + generation-guard + reset
// scaffolding so every store shares one correct implementation, and auto-registers
// each instance so the sign-out sweep can't forget a new store (the cause of a past
// viewed-jobs leak: the reset list lived apart from the store definitions).

import { browser } from '$app/environment';

// Every UserResource registers here on construction; resetUserStores() (called from
// the root layout on sign-out) drops them all, so a new per-user store participates
// automatically — there is no hand-maintained list to forget.
const registry: UserResource<unknown>[] = [];

/** Drop every per-user store's cached data. Called on sign-out; idempotent. */
export function resetUserStores(): void {
  for (const store of registry) store.reset();
}

export abstract class UserResource<T> {
  // Reactive so readers can distinguish "not loaded yet" from "loaded, empty" (both
  // may leave the resource blank) — e.g. the filter modal hides its profile action
  // until the load settles instead of flashing the wrong affordance.
  #loaded = $state(false);
  // The in-flight load, shared so concurrent callers issue one request.
  #loading: Promise<void> | null = null;
  // Bumped by reset(); a load resolving after a reset (a same-tab user handoff) is
  // discarded instead of repopulating with the previous user's data.
  #generation = 0;

  constructor() {
    registry.push(this as UserResource<unknown>);
  }

  /** True once the resource has been loaded (or populated by a mutation). */
  get loaded(): boolean {
    return this.#loaded;
  }

  /** Fetch this resource's data (one or more API calls). */
  protected abstract load(): Promise<T>;
  /** Copy fetched data into reactive state. */
  protected abstract apply(data: T): void;
  /** Clear reactive state back to its signed-out default. */
  protected abstract clearState(): void;

  /** Mark the resource loaded after a mutation populated it fresh (so a later
   *  ensureLoaded() is a no-op instead of a redundant fetch). */
  protected markLoaded(): void {
    this.#loaded = true;
  }

  /** Load once. Repeat calls reuse the first load (or its in-flight promise). No-op
   *  on the server; a failed load leaves the resource in its empty/default state. */
  async ensureLoaded(): Promise<void> {
    if (!browser || this.#loaded) return;
    if (this.#loading) return this.#loading;
    const gen = this.#generation;
    this.#loading = this.load()
      .then((data) => {
        if (gen !== this.#generation) return; // reset() ran mid-load — discard stale data.
        this.apply(data);
        this.#loaded = true;
      })
      .catch(() => {
        // best-effort: a failed load just leaves the empty/default state.
      })
      .finally(() => {
        if (gen === this.#generation) this.#loading = null;
      });
    return this.#loading;
  }

  /** Drop cached data + the loaded flag on sign-out, so the next user loads their own. */
  reset(): void {
    this.#generation++;
    this.clearState();
    this.#loaded = false;
    this.#loading = null;
  }
}
