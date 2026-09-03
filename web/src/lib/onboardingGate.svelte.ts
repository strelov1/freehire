import { resolve } from '$app/paths';

// Shared "has this visit already been sent through /onboarding and left" flag. Module-
// level $state singleton (mirrors auth-dialog.svelte.ts) — a client-only UI concern,
// safe under SSR since it stays false on the server.
//
// Without this, the root layout's redirect effect would bounce a user right back to
// /onboarding the instant they navigate away after finishing or skipping it, even within
// the same visit (the CV can still be absent — everything on /onboarding is skippable).
// This lets that one visit's choice stick; a later hard reload (a genuinely new visit)
// resets it, so a still-CV-less account is asked again, per the gate's actual re-trigger
// rule ("every visit until a CV exists").
//
// Not a UserResource (it fetches nothing to load/clear), but it MUST still be dropped on
// sign-out like the rest of them: without `reset()`, account A dismissing onboarding and
// then signing out leaves `dismissedThisVisit` true for whoever signs in next on the same
// tab — a still-CV-less account B would silently skip the gate that is supposed to catch
// it "every visit". The root layout's existing sign-out sweep calls this alongside
// resetUserStores().
let dismissedThisVisit = $state(false);

export const onboardingGate = {
  get dismissed() {
    return dismissedThisVisit;
  },
  dismiss() {
    dismissedThisVisit = true;
  },
  reset() {
    dismissedThisVisit = false;
  },
};

/** The URL for /onboarding carrying `returnTo` — where the wizard sends the visitor
 *  back once they leave it (skip the auth step, or finish/skip the rest). Every entry
 *  point (AuthDialog's "Create one", a signed-out gate's "Sign up", the layout's
 *  no-CV auto-redirect) builds this the same way, so they share one place to change
 *  the query param name or add validation. */
export function onboardingUrl(returnTo: string): string {
  return `${resolve('/onboarding')}?returnTo=${encodeURIComponent(returnTo)}`;
}
