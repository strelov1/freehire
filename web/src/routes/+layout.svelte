<script lang="ts">
  import { navigating, page, updated } from '$app/state';
  import { afterNavigate, beforeNavigate } from '$app/navigation';
  import { onMount } from 'svelte';
  import { initTheme } from '$lib/theme.svelte';
  import { isAuthenticated } from '$lib/auth.svelte';
  import { resetUserStores } from '$lib/userResource.svelte';
  import {
    capturePageview,
    syncReplayForRoute,
    identifyUser,
    resetIdentity,
    trackSignupIfNew,
  } from '$lib/analytics';
  import TopBar from '$lib/components/TopBar.svelte';
  import CliBanner from '$lib/components/CliBanner.svelte';
  import EmailVerificationBanner from '$lib/components/EmailVerificationBanner.svelte';
  import Footer from '$lib/components/Footer.svelte';
  import CookieConsent from '$lib/components/CookieConsent.svelte';
  import ConfirmTailorDialog from '$lib/components/ConfirmTailorDialog.svelte';
  import CvRefreshDialog from '$lib/components/CvRefreshDialog.svelte';
  import SupportToast from '$lib/components/SupportToast.svelte';
  import { ConfirmDialog } from '$lib/ui';
  import '../app.css';
  // Country-flag icon sheet (used by $lib/components/Flag.svelte). References its
  // SVGs by URL, so the browser only fetches flags actually rendered.
  import 'flag-icons/css/flag-icons.min.css';

  let { children } = $props();

  // The account area (/my/*) is an app-like surface with its own sidebar nav —
  // the marketing footer with its link columns doesn't belong there.
  const hideFooter = $derived(
    page.url.pathname === '/my' ||
      page.url.pathname.startsWith('/my/') ||
      page.url.pathname.startsWith('/tailor/'),
  );

  // Re-apply the persisted theme once mounted (the singleton may have been
  // constructed on the server, defaulting to light). A no-FOUC inline script
  // in app.html already set the class before paint.
  onMount(() => {
    initTheme();
    registerPwaServiceWorker();
  });

  // hooks.server.ts sets `<html lang>` on the initial SSR response, but a live
  // language switch (updateLanguage() -> invalidateAll(), no reload) only
  // refreshes `page.data` — it doesn't re-run SSR, so the attribute would
  // otherwise go stale the moment the translated content flips.
  $effect(() => {
    document.documentElement.lang = page.data.locale;
  });

  // `injectRegister: null` in vite.config.ts (the CSP has no 'unsafe-inline'
  // script-src, so an auto-injected inline registration script would be blocked)
  // — register from this real module import instead. In dev, `devOptions` is
  // left disabled in vite.config.ts, so this resolves to a no-op there.
  //
  // `registerType: 'autoUpdate'` skips the browser's own update prompt, and
  // vite-plugin-pwa's default behaviour for that mode is to force
  // `window.location.reload()` on every open tab the instant a new build
  // activates — including a tab mid-edit in the CV editor, an application
  // form, or an assistant chat. `onNeedReload` replaces that unconditional
  // reload with a confirmation, so a new deploy never silently drops unsaved
  // work.
  let showReloadPrompt = $state(false);

  async function registerPwaServiceWorker() {
    const { registerSW } = await import('virtual:pwa-register');
    registerSW({
      immediate: true,
      onNeedReload() {
        showReloadPrompt = true;
      },
    });
  }

  function reloadForUpdate() {
    window.location.reload();
  }

  // Drop every per-user store the moment the session ends. logout() re-resolves via
  // invalidateAll() — a soft client navigation, so these module-singleton stores
  // survive it and would otherwise show the previous user's data to whoever signs in
  // next on the same tab. resetUserStores() sweeps every registered UserResource, so
  // a new per-user store participates automatically (no list here to forget). Fired
  // from the always-mounted root layout so it covers every route; idempotent while
  // signed out.
  $effect(() => {
    if (!isAuthenticated()) resetUserStores();
  });

  // A tab open across a deploy still holds the old build's route manifest, naming
  // _app/immutable chunks the release has deleted. Its next client-side navigation
  // import()s one, gets a 404, and SvelteKit draws the 500 page over an HTTP 200 —
  // 265 of those reached Sentry in a day. `updated.current` flips once the version
  // poll (see svelte.config.js) sees a new build; leaving through a full page load
  // instead of a client-side one fetches the new manifest and the chunk resolves.
  //
  // Not a second copy of the service worker's onNeedReload prompt above: that one
  // asks a tab sitting still whether to reload NOW, and takes a confirmation because
  // saying yes drops whatever is half-typed. This one waits until the reader is
  // already leaving, where nothing they were looking at survives the navigation
  // either way — so it needs no prompt, and costs only the round trip. It also
  // covers the tab whose service worker never registered, which is how these 404s
  // outlived the `paths.relative` fix. `willUnload` means the browser is handling
  // the navigation itself (external link, reload) and there is nothing to intercept.
  beforeNavigate(({ willUnload, to }) => {
    if (updated.current && !willUnload && to?.url) location.href = to.url.href;
  });

  afterNavigate(() => {
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

<div class="flex min-h-svh flex-col">
  <TopBar />

  <!-- Self-gating: renders only for a signed-in, unverified account. -->
  <EmailVerificationBanner />

  <!-- Self-gating: renders until dismissed. Below the verification prompt on purpose
       — a promo strip must not push a security notice further from the header. -->
  <CliBanner />

  <main class="flex-1">
    {@render children()}
  </main>

  {#if !hideFooter}
    <Footer />
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

<ConfirmTailorDialog />

<ConfirmDialog
  bind:open={showReloadPrompt}
  title="A new version of freehire is available"
  confirmLabel="Reload now"
  onConfirm={reloadForUpdate}
/>

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
