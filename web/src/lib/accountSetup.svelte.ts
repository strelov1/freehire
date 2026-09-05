// Binds the completeness rules (accountCompleteness.ts) to the three per-user stores
// that already hold their inputs, so the card on the tracking page and the dot in the
// header ask the same question of the same data.
//
// No store of its own and no endpoint: every input is a UserResource singleton that
// loads once per session and is shared, so a second reader costs nothing. Keeping the
// rules pure next door is what lets them be unit-tested without any of this.

import { accountSteps, stepsOutstanding, type CompletenessStep } from './accountCompleteness';
import { notifications } from './notifications.svelte';
import { profileStore } from './profile.svelte';
import { resumeStore } from './resume.svelte';

/** Start the three loads. Idempotent and safe to call from several components. */
export function ensureAccountSetupLoaded(): void {
  void resumeStore.ensureLoaded();
  void profileStore.ensureLoaded();
  void notifications.ensureLoaded();
}

/** True once all three inputs have settled.
 *
 *  Both surfaces wait for this rather than rendering from a partial read: every store
 *  starts empty, so a half-loaded account looks exactly like a brand-new one — and
 *  telling somebody who finished setting up that they have five steps left, for the
 *  moment it takes the profile to arrive, is worse than showing nothing. */
export function accountSetupReady(): boolean {
  return resumeStore.loaded && profileStore.loaded && notifications.loaded;
}

function input() {
  return {
    hasCv: resumeStore.present,
    profile: profileStore.profile,
    alertCount: notifications.subscriptions.length,
  };
}

/** The setup steps and whether each is done. Empty until the inputs have settled. */
export function setupSteps(): CompletenessStep[] {
  return accountSetupReady() ? accountSteps(input()) : [];
}

/** How many steps are still open, or 0 while the inputs are still loading. */
export function setupOutstanding(): number {
  return accountSetupReady() ? stepsOutstanding(input()) : 0;
}
