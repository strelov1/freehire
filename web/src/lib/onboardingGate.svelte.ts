import { onboardingUrl } from '#lib/signin.js';

export { onboardingUrl };

// Shared "has this visit already been sent through /onboarding and left" flag. A
// module-level $state singleton — a client-only UI concern, safe under SSR since it
// stays false on the server.
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

// A guest who reaches /onboarding for an action and leaves it to authenticate (skipping the CV,
// or signing in to an existing account) has been through it. The mark outlives a full-page OAuth
// round trip (in-memory state does not), so it rides the tab's sessionStorage until an account
// appears and the layout takes it. It carries only a timestamp, and goes stale: abandoned intent
// must not excuse some later account from the flow.
const GUEST_SKIP_KEY = 'fh:onboarding-guest-skip';
const GUEST_SKIP_TTL_MS = 30 * 60 * 1000;

export const onboardingGate = {
  get dismissed() {
    return dismissedThisVisit;
  },
  dismiss() {
    dismissedThisVisit = true;
  },
  /** The session ended: the visit's dismissal and any pending guest skip go with it. */
  reset() {
    dismissedThisVisit = false;
    try {
      sessionStorage.removeItem(GUEST_SKIP_KEY);
    } catch {
      // Storage blocked: nothing was kept.
    }
  },
  /** A guest is leaving /onboarding to authenticate: remember it for the account that appears. */
  skipAsGuest(now = Date.now()) {
    try {
      sessionStorage.setItem(GUEST_SKIP_KEY, String(now));
    } catch {
      // Storage blocked: the account is routed through onboarding once, as any new one is.
    }
  },
  /** Consumes the pending guest skip: true once, for the first account to appear within the TTL. */
  takeGuestSkip(now = Date.now()): boolean {
    try {
      const at = Number(sessionStorage.getItem(GUEST_SKIP_KEY));
      sessionStorage.removeItem(GUEST_SKIP_KEY);
      return at > 0 && at <= now && now - at <= GUEST_SKIP_TTL_MS;
    } catch {
      return false;
    }
  },
};

/** The layout's routing for a signed-in account. A pending guest skip belongs to the first account
 *  that appears (whether or not it needs onboarding), so it is consumed here, not left for the
 *  next one. An account that has not been onboarded goes to /onboarding once — unless it just came
 *  out of that page as a guest, which counts as having seen it. Not on /onboarding itself, nor on
 *  /signin, which navigates to its own returnTo the moment the session flips. */
export function routeAccount(
  path: string,
  search: string,
  needsOnboarding: boolean,
  deps: { complete: () => Promise<unknown>; go: (url: string) => void },
): void {
  if (path === '/onboarding' || path === '/signin') return;
  const skipped = onboardingGate.takeGuestSkip();
  if (!needsOnboarding) return;
  if (skipped) {
    dismissedThisVisit = true;
    void deps.complete().catch(() => {});
  } else if (!dismissedThisVisit) {
    deps.go(onboardingUrl(path + search));
  }
}
