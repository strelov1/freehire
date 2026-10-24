<script lang="ts">
  import { navigating, page, updated } from '$app/state';
  import { afterNavigate, beforeNavigate, goto } from '$app/navigation';
  import { gotoPath } from '#lib/gotoPath.js';
  import { resolve } from '$app/paths';
  import { onMount, untrack } from 'svelte';
  import { initTheme } from '#lib/theme.svelte.js';
  import { currentUser, isAuthenticated } from '#lib/auth.svelte.js';
  import { confirmTailorDialog, settleConfirmTailorDialog } from '#lib/confirmTailorDialog.svelte.js';
  import { onboardingGate, onboardingUrl } from '#lib/onboardingGate.svelte.js';
  import { isFullBleedRoute } from '#lib/shellLayout.js';
  import { safeRedirect } from '#lib/safeRedirect.js';
  import { resetUserStores } from '#lib/userResource.svelte.js';
  import {
    capturePageview,
    syncReplayForRoute,
    identifyUser,
    resetIdentity,
    trackSignupIfNew,
  } from '#lib/analytics.js';
  import TopBar from '#lib/components/TopBar.svelte';
  import CliBanner from '#lib/components/CliBanner.svelte';
  import EmailVerificationBanner from '#lib/components/EmailVerificationBanner.svelte';
  import Footer from '#lib/components/Footer.svelte';
  import CookieConsent from '#lib/components/CookieConsent.svelte';
  import CvRefreshDialog from '#lib/components/CvRefreshDialog.svelte';
  import SupportToast from '#lib/components/SupportToast.svelte';
  import SkipLink from '#lib/components/SkipLink.svelte';
  import '../app.css';
  // Country-flag icon sheet for the design system's CountryFlag, which leaves loading it
  // to the app. References its SVGs by URL, so the browser only fetches flags actually
  // rendered. Global rather than per route: the job lists on most pages draw flags, and
  // the sheet ships inside the same stylesheet as app.css, so it costs no extra request.
  import 'flag-icons/css/flag-icons.min.css';

  let { children } = $props();

  // The tailor pre-flight dialog renders skill icons, which pull the whole techmark sheet
  // (~115KB brotli) into whatever chunk imports it. Statically imported here, that was
  // every page's cost for a dialog most visits never open. It is fetched on the first ask
  // instead and stays mounted afterwards.
  // No race with the ask's promise: the controller holds the open state and the resolver
  // at module level, so the component mounting late reads `open` as already true and
  // settling goes through the same controller. A chunk that fails to load must still
  // answer the ask, or the caller would await forever; the next ask retries the import.
  let confirmTailorModule = $state<Promise<typeof import('#lib/components/ConfirmTailorDialog.svelte')> | null>(
    null,
  );
  $effect(() => {
    if (!confirmTailorDialog.open || confirmTailorModule) return;
    const load = import('#lib/components/ConfirmTailorDialog.svelte');
    confirmTailorModule = load;
    load.catch(() => {
      confirmTailorModule = null;
      settleConfirmTailorDialog(false);
    });
  });

  // The account area (/my/*) is an app-like surface with its own sidebar nav — the
  // marketing footer's link columns don't belong there, but a compact one (just
  // copyright/cookie settings/social/open-source note) still anchors the page.
  // The full-bleed account routes (the assistant chat, CV tailoring — see
  // isFullBleedRoute) size themselves to the exact viewport height and get no
  // footer at all, same as /onboarding and /signin, both their own full-screen
  // pages (a fixed inset-0 overlay covers TopBar too).
  const isAccountRoute = $derived(
    page.url.pathname === '/my' || page.url.pathname.startsWith('/my/'),
  );
  const hideFooter = $derived(
    isFullBleedRoute(page.url.pathname) ||
      page.url.pathname === '/onboarding' ||
      page.url.pathname === '/signin',
  );
  const compactFooter = $derived(isAccountRoute && !hideFooter);

  // The onboarding gate: auto-redirect a signed-in visitor who has never been through the
  // wizard to /onboarding (which bounces an anonymous visitor straight on to /signin — see
  // that page's own gate).
  //
  // The condition is one explicit account fact, `onboarding_completed_at`. It used to be
  // "does this account have a CV", which was a fair proxy while the wizard was about the
  // CV — and became wrong the moment it started asking about experience, money and the
  // shape of the candidate's search, none of which a stored PDF answers. Under the old
  // rule every existing account (nearly all of which have a CV) would have skipped every
  // new question permanently.
  //
  // It rides on the user read the layout already makes, so there is no separate fetch to
  // wait for. `onboardingGate.dismissed` is what keeps this from re-firing within one
  // visit for someone who navigated away without finishing; they are asked again next
  // visit, which is the same behaviour the CV rule had.
  const needsOnboarding = $derived(currentUser()?.onboarding_completed_at == null);
  $effect(() => {
    if (isAuthenticated() && needsOnboarding && !onboardingGate.dismissed && page.url.pathname !== '/onboarding') {
      // eslint-disable-next-line svelte/no-navigation-without-resolve -- onboardingUrl() wraps resolve('/onboarding'); the rule can't see through the appended ?returnTo= query
      void goto(onboardingUrl(page.url.pathname + page.url.search));
    }
  });

  // The other direction — bounce AWAY from /onboarding once there is truly nothing to do
  // there (signed in AND already marked complete) — is checked only at ARRIVAL: a direct,
  // bookmarked or stale visit. The page itself always navigates elsewhere on its own way
  // out, and it is the page that writes the completion marker, so reacting to that write
  // live would yank the candidate off the wizard the instant they finished their last step
  // instead of letting it navigate them. Hence untrack().
  //
  // Honors the same `returnTo` the page itself would have used, so a stale link at least
  // lands the visitor back where they started rather than home.
  $effect(() => {
    if (page.url.pathname !== '/onboarding') return;
    untrack(() => {
      if (isAuthenticated() && currentUser()?.onboarding_completed_at != null) {
        void gotoPath(safeRedirect(page.url.searchParams.get('returnTo')) ?? resolve('/'));
      }
    });
  });

  // Re-apply the persisted theme once mounted (the singleton may have been
  // constructed on the server, defaulting to light). A no-FOUC inline script
  // in app.html already set the class before paint.
  onMount(() => {
    initTheme();
  });

  // hooks.server.ts sets `<html lang>` on the initial SSR response, but a live
  // language switch (updateLanguage() -> invalidateAll(), no reload) only
  // refreshes `page.data` — it doesn't re-run SSR, so the attribute would
  // otherwise go stale the moment the translated content flips.
  $effect(() => {
    document.documentElement.lang = page.data.locale;
  });

  // Drop every per-user store the moment the session ends. logout() re-resolves via
  // invalidateAll() — a soft client navigation, so these module-singleton stores
  // survive it and would otherwise show the previous user's data to whoever signs in
  // next on the same tab. resetUserStores() sweeps every registered UserResource, so
  // a new per-user store participates automatically (no list here to forget). Fired
  // from the always-mounted root layout so it covers every route; idempotent while
  // signed out.
  $effect(() => {
    if (!isAuthenticated()) {
      resetUserStores();
      onboardingGate.reset();
    }
  });

  // A tab open across a deploy still holds the old build's route manifest, naming
  // _app/immutable chunks the release has deleted. Its next client-side navigation
  // import()s one, gets a 404, and SvelteKit draws the 500 page over an HTTP 200 —
  // 265 of those reached Sentry in a day. `updated.current` flips once the version
  // poll (see vite.config.ts) sees a new build; leaving through a full page load
  // instead of a client-side one fetches the new manifest and the chunk resolves.
  //
  // It waits until the reader is already leaving, where nothing they were looking at
  // survives the navigation either way — so it needs no prompt, and costs only the
  // round trip. `willUnload` means the browser is handling the navigation itself
  // (external link, reload) and there is nothing to intercept.
  beforeNavigate(({ willUnload, to, shallow }) => {
    // Shallow routing fires this too in SvelteKit 3; a filter's URL write is not the
    // reader leaving, and reloading under them mid-keystroke would be.
    if (shallow) return;
    if (updated.current && !willUnload && to?.url) location.href = to.url.href;
  });

  // The tailor pre-flight dialog lives in this layout, so a navigation does not unmount it:
  // following its upgrade link would land on /my/plan with the dialog still covering the
  // page. Leaving the page is a cancel — the candidate went somewhere else instead of
  // tailoring. Registered here rather than in the dialog because the dialog loads lazily,
  // and a navigation during that load must still cancel the ask.
  beforeNavigate(({ shallow }) => {
    if (shallow) return; // SvelteKit 3 fires this for shallow routing too — not leaving
    if (confirmTailorDialog.open) settleConfirmTailorDialog(false);
  });

  afterNavigate(({ shallow }) => {
    // SvelteKit 3 fires this for shallow routing too; a filter's URL write is not a
    // pageview.
    if (shallow) return;
    capturePageview();
    syncReplayForRoute(page.url.pathname);
  });

  // Bind/clear analytics identity only on a real session transition. page.data is
  // a fresh object per navigation, so acting on every run would call posthog.reset()
  // on each anonymous navigation — reset() mints a new anonymous id each time and
  // would fragment one visitor into many. Tracking the last identified id makes
  // identify fire once at sign-in and reset once at sign-out.
  let lastIdentified: number | null = null;
  $effect(() => {
    const id = page.data.user?.id ?? null;
    if (id === lastIdentified) return;
    lastIdentified = id;
    if (page.data.user) {
      identifyUser(page.data.user);
      // Identity binding is the only place a sign-up is visible: OAuth returns
      // through a full-page redirect, so the app cannot tell a first-ever sign-in
      // from any other. A just-created account is the signal; the call itself is
      // idempotent per account.
      trackSignupIfNew(page.data.user);
    } else resetIdentity();
  });
</script>

<!-- Column layout with min-h-svh (small-viewport height, so the mobile address
     bar never forces extra scroll) keeps the footer pinned to the bottom on
     sparse pages while letting main grow. Each route owns its own width/padding. -->
<!-- Global navigation progress bar: shown the moment a client-side navigation
     starts (navigating.to is set) and until it settles, so the user gets instant
     feedback even while a route's `load` is still streaming. -->
{#if navigating.to}
  <div
    class="fixed inset-x-0 top-0 z-50 h-0.5 overflow-hidden bg-brand/20"
    role="progressbar"
    aria-label="Loading"
    aria-busy="true"
  >
    <div class="nav-progress-bar h-full w-2/5 bg-brand"></div>
  </div>
{/if}

<SkipLink target="main-content" />

<div class="flex min-h-svh flex-col">
  <TopBar />

  <!-- Self-gating: renders only for a signed-in, unverified account. -->
  <EmailVerificationBanner />

  <!-- Self-gating: renders until dismissed. Below the verification prompt on purpose
       — a promo strip must not push a security notice further from the header. -->
  <CliBanner />

  <main id="main-content" tabindex="-1" class="flex-1 focus:outline-none">
    {@render children()}
  </main>

  {#if !hideFooter}
    <Footer compact={compactFooter} />
  {/if}
</div>

<!-- Consent banner: fixed-position, self-gating (renders only for a
     consent-required visitor with no choice, or when re-opened from the footer). -->
<CookieConsent />

<!-- Open-source support toast: fixed-position, so it belongs here rather than beside
     <CliBanner /> up in the flow. Self-gating — it yields this same corner to the
     consent banner above, and retires for good once answered. -->
<SupportToast />

<CvRefreshDialog />

{#if confirmTailorModule}
  {#await confirmTailorModule then { default: ConfirmTailorDialog }}
    <ConfirmTailorDialog />
  {:catch}
    <!-- The effect above already answered the ask; nothing to render. -->
  {/await}
{/if}

<style>
  /* Indeterminate sweep: the segment slides across the track on repeat while a
     navigation is in flight. Disabled under reduced-motion, where a static bar
     still signals activity. */
  .nav-progress-bar {
    animation: nav-progress 1.1s ease-in-out infinite;
  }

  @keyframes nav-progress {
    0% {
      transform: translateX(-110%);
    }
    100% {
      transform: translateX(320%);
    }
  }

  @media (prefers-reduced-motion: reduce) {
    .nav-progress-bar {
      animation: none;
      width: 100%;
    }
  }
</style>
