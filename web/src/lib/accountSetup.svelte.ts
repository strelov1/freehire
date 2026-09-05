// Binds the completeness rules (accountCompleteness.ts) to the three per-user stores
// that already hold their inputs, so the card on the tracking page and the dot in the
// header ask the same question of the same data.
//
// No store of its own and no endpoint: every input is a UserResource singleton that
// loads once per session and is shared, so a second reader costs nothing. Keeping the
// rules pure next door is what lets them be unit-tested without any of this.
//
// The cost this DOES add, recorded because it is easy to miss: the header asks for the
// dot on every page, so a signed-in user who never opens an account page now warms the
// résumé, profile and notification stores anyway. That is five GETs, once per session
// (the notification store fetches Telegram status and the webhook alongside the
// subscriptions this needs), not five per navigation. The alternative — showing the dot
// only where the data happened to already be loaded — is a nudge that appears on the one
// page whose visitor least needs it.

import { accountSteps, outstandingOf, type CompletenessStep } from './accountCompleteness';
import { isAuthenticated } from './auth.svelte';
import { notifications } from './notifications.svelte';
import { profileStore } from './profile.svelte';
import { resumeStore } from './resume.svelte';

/** Start the three loads. Idempotent and safe to call from several components.
 *
 *  The signed-in check lives here rather than at each call site: there are two of them
 *  and they had the same `if` written out twice, so a condition that later grows (a
 *  verified address, say) would have had two places to grow in. */
export function ensureAccountSetupLoaded(): void {
  if (!isAuthenticated()) return;
  void resumeStore.ensureLoaded();
  void profileStore.ensureLoaded();
  void notifications.ensureLoaded();
}

/** True once all three inputs have settled.
 *
 *  Not exported: callers get an empty step list until this is true, which is the same
 *  answer in the shape they already handle. Both surfaces depend on it rather than
 *  rendering from a partial read — every store starts empty, so a half-loaded account
 *  looks exactly like a brand-new one, and telling somebody who finished setting up that
 *  they have five steps left, for the moment it takes the profile to arrive, is worse
 *  than showing nothing. */
function accountSetupReady(): boolean {
  return resumeStore.loaded && profileStore.loaded && notifications.loaded;
}

function input() {
  return {
    hasCv: resumeStore.present,
    profile: profileStore.profile,
    // Active ones only. A paused subscription delivers nothing, so counting it would
    // tell someone their alerts are set up while no job is being sent to them — the
    // exact thing this step exists to notice.
    alertCount: notifications.subscriptions.filter((s) => s.active).length,
  };
}

/** The setup steps and whether each is done. Empty until the inputs have settled. */
export function setupSteps(): CompletenessStep[] {
  return accountSetupReady() ? accountSteps(input()) : [];
}

/** How many steps are still open, or 0 while the inputs are still loading. */
export function setupOutstanding(): number {
  return outstandingOf(setupSteps()).length;
}
