// The signed-in candidate's screening answers — the handful of questions that repeat
// across ATS forms (visa, salary, notice period, relocation, …) and no CV states.
//
// A shared store rather than a per-page fetch because two surfaces now read the same
// answer: the Screening section edits it, and profile completeness reads whether it has
// been filled in at all. One GET serves both, and the profile layout warms it beside the
// profile and résumé so the completeness summary can count it without a second request.
//
// SSR-safe and auth-agnostic (see UserResource): the load is a browser-only no-op and
// the answers stay null for signed-out users.

import { api } from '#lib/api.js';
import type { Answers } from '#lib/generated/contracts.js';
import { serialQueue } from '#lib/serialQueue.js';
import { UserResource } from '#lib/userResource.svelte.js';

class ScreeningStore extends UserResource<Answers | null> {
  // Reassigned (never mutated in place) on every change, so $state.raw is enough.
  #answers = $state.raw<Answers | null>(null);
  // Saves go one at a time, so two dropdowns picked a moment apart reach the server in the
  // order they were picked and the last pick is the one that stays.
  #queue = serialQueue();

  get answers(): Answers | null {
    return this.#answers;
  }

  // The endpoint is best-effort on the profile: a failure must not error the page.
  // Resolving with the copy already held (instead of letting the base swallow the
  // rejection and leave `loaded` false) lets completeness settle on "not filled" rather
  // than hiding its summary forever, and keeps a failed refresh after a save from
  // blanking the form, which re-seeds itself from this value.
  protected async load(): Promise<Answers | null> {
    try {
      return await api.getScreeningAnswers();
    } catch (err) {
      console.warn('screening answers: load failed', err);
      return this.#answers;
    }
  }

  protected apply(row: Answers | null) {
    this.#answers = row;
  }

  /** Save part of the answers. The PUT answers with the whole stored record, so that
   *  becomes the current copy with no re-read after it. An answer that lands after the
   *  account signed out is dropped, the same guard the reads carry: it belongs to whoever
   *  was signed in when it was sent. */
  save(patch: Partial<Answers>): Promise<void> {
    const generation = this.generation;
    return this.#queue(async () => {
      const answers = await api.updateScreeningAnswers(patch);
      if (generation !== this.generation) return;
      this.#answers = answers;
      this.markLoaded();
    });
  }

  protected clearState() {
    this.#answers = null;
  }
}

export const screeningStore = new ScreeningStore();
