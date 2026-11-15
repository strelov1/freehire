import { goto } from '$app/navigation';
import { page } from '$app/state';
import { isAuthenticated } from '#lib/auth.svelte.js';
import { bannerVisible, loadOnboardingState, markSeen, type OnboardingLifecycle } from '#lib/onboarding.js';
import { onboardingUrl } from '#lib/onboardingGate.svelte.js';
import { consumePendingAlert } from '#lib/saveSearchAlert.js';

/** JobsView's onboarding nudge: the one-time banner, and the Telegram-alert offer that
 *  follows onboarding. Constructed during component initialisation, since it registers
 *  an effect; `load` reads the stored lifecycle once the component has mounted. */
export class OnboardingNudge {
  readonly #standalone: boolean;
  readonly #filtered: () => boolean;

  // Onboarding: the one-time nudge banner + wizard, standalone-only. The lifecycle
  // lives in localStorage (client-only); seed it at init on the client so a returning
  // (dismissed/completed) visitor never flashes a banner before mount. The banner is
  // the sole entry — once dismissed or completed it retires; there is no persistent
  // re-open control. "Set up" now navigates to /onboarding (registration is a step
  // there, and it carries straight on into role/skills/level) rather than opening a
  // local wizard that wrote to the anonymous filter query.
  // Starts 'unseen' on both sides so the hydrated markup matches the server's; the
  // stored value arrives on mount, by which point app.css has already hidden the
  // banner for anyone who dismissed or completed it. Reading localStorage here
  // instead would make the client disagree with the SSR output on the very first
  // frame. Same shape as CliBanner's `dismissed`.
  phase = $state<OnboardingLifecycle>('unseen');
  // The ephemeral post-onboarding Telegram-alert offer (set after the wizard, or on
  // mount to resume a pending alert after sign-in). Dismissible; not persisted.
  alert = $state<{ query: string; autostart: boolean } | null>(null);
  // Show only to an un-nudged visitor with no active facet filters AND no text query,
  // so a shared search/filter link is never interrupted (activeFilterCount ignores the
  // query, so check it explicitly).
  //
  // Deliberately NOT gated on `browser`. It was, and that is what made this banner
  // the site's largest source of layout shift: the feed rendered without it, then
  // hydration inserted 74px directly above the rows. Lab Lighthouse missed it
  // (0.066) because it never carries a returning visitor's localStorage; CrUX put
  // the mobile home page at CLS 0.28. It is server-rendered now, and hidden before
  // first paint by app.css for a visitor who has already seen it.
  get show(): boolean {
    return this.#standalone && bannerVisible(this.phase, this.#filtered());
  }

  dismiss = () => {
    markSeen();
    this.phase = 'seen';
  };
  open = () => {
    // The nudge has done its job whether or not the visitor follows through — it
    // retires the same way dismissing it does, just via a different exit.
    markSeen();
    this.phase = 'seen';
    // eslint-disable-next-line svelte/no-navigation-without-resolve -- onboardingUrl() wraps resolve('/onboarding'); the rule can't see through the appended ?returnTo= query
    void goto(onboardingUrl(page.url.pathname + page.url.search));
  };

  /** `standalone` is the /jobs list (the only surface that nudges); `filtered` says the
   *  list is narrowed by a facet or a query, which keeps a shared search uninterrupted. */
  constructor(standalone: boolean, filtered: () => boolean) {
    this.#standalone = standalone;
    this.#filtered = filtered;

    // Resume a save the user started before signing in. Reactive on the session so it
    // fires whether auth resolved server-side (an OAuth full-page redirect — mount is
    // already authed) or flipped in-tab (an in-dialog sign-in re-resolves page.data.user
    // without a remount). Consumed exactly once; standalone list only; skipped if a banner
    // is already showing.
    $effect(() => {
      if (!this.#standalone || this.alert) return; // effect: client-only, no browser guard
      if (!isAuthenticated()) return;
      const pending = consumePendingAlert();
      if (pending !== null) this.alert = { query: pending, autostart: true };
    });
  }

  /** Catch up with the stored lifecycle once localStorage is readable (on mount). */
  load() {
    this.phase = loadOnboardingState();
  }
}
