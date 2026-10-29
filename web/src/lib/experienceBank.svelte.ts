// The signed-in candidate's experience bank, shared so the profile's completeness
// summary reads the same list the Experience tab shows. Counting the CV parse instead
// told somebody who had added a role by hand that the section was not filled.
//
// ExperienceBankView owns its own copy (it re-reads after every edit and after each
// assistant turn) and publishes each fresh read here through `accept`, so the badge
// follows every change without a second request.
//
// SSR-safe and auth-agnostic (see UserResource): the load is a browser-only no-op and
// the bank stays null for signed-out users.

import { api } from '#lib/api.js';
import type { ExperienceBank } from '#lib/types.js';
import { UserResource } from '#lib/userResource.svelte.js';

class ExperienceBankStore extends UserResource<ExperienceBank | null> {
  // Reassigned (never mutated in place) on every change, so $state.raw is enough.
  #bank = $state.raw<ExperienceBank | null>(null);

  get bank(): ExperienceBank | null {
    return this.#bank;
  }

  protected load(): Promise<ExperienceBank | null> {
    return api.getExperience();
  }

  protected apply(bank: ExperienceBank | null) {
    this.#bank = bank;
  }

  protected clearState() {
    this.#bank = null;
  }

  /** Take a read the bank view already made, so the badge follows it for free. */
  accept(bank: ExperienceBank): void {
    this.#bank = bank;
    this.markLoaded();
  }
}

export const experienceBankStore = new ExperienceBankStore();
