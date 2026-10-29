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
import { UserResource } from '#lib/userResource.svelte.js';

class ScreeningStore extends UserResource<Answers | null> {
  // Reassigned (never mutated in place) on every change, so $state.raw is enough.
  #answers = $state.raw<Answers | null>(null);

  get answers(): Answers | null {
    return this.#answers;
  }

  // The endpoint is best-effort on the profile: a failure must leave the section blank
  // rather than error the page. Returning null (instead of letting the base swallow the
  // rejection and continue leaving `loaded` false) lets completeness settle on "not
  // filled" instead of hiding its summary forever.
  protected async load(): Promise<Answers | null> {
    try {
      return await api.getScreeningAnswers();
    } catch {
      return null;
    }
  }

  protected apply(row: Answers | null) {
    this.#answers = row;
  }

  protected clearState() {
    this.#answers = null;
  }

  /** Re-fetch after a write. `ensureLoaded` no-ops once loaded, so a save has to say so
   *  explicitly. Best-effort: a failure keeps the previous copy rather than blanking it. */
  async refresh(): Promise<void> {
    try {
      this.#answers = await api.getScreeningAnswers();
      this.markLoaded();
    } catch {
      // best-effort: keep whatever was last read.
    }
  }
}

export const screeningStore = new ScreeningStore();
