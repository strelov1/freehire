<script lang="ts">
  import { navigating, page } from '$app/state';
  import { afterNavigate } from '$app/navigation';
  import { onMount } from 'svelte';
  import { initTheme } from '$lib/theme.svelte';
  import { isAuthenticated } from '$lib/auth.svelte';
  import { resetUserStores } from '$lib/userResource.svelte';
  import {
    capturePageview,
    syncReplayForRoute,
    identifyUser,
    resetIdentity,
  } from '$lib/analytics';
  import TopBar from '$lib/components/TopBar.svelte';
  import Footer from '$lib/components/Footer.svelte';
  import '../app.css';

  let { children } = $props();

  // The account area (/my/*) is an app-like surface with its own sidebar nav —
  // the marketing footer with its link columns doesn't belong there.
  const hideFooter = $derived(
    page.url.pathname === '/my' || page.url.pathname.startsWith('/my/'),
  );

  // Apply the persisted theme and start tracking the OS preference once mounted.
  // A no-FOUC inline script in app.html already set the class before paint.
  onMount(() => initTheme());

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

  // Analytics is inert unless PostHog was initialized (see hooks.client.ts), so
  // every call below is a no-op when the key is absent. afterNavigate also fires
  // on the initial load, so the pageview and the replay privacy toggle cover a
  // hard-loaded route too — replay is stopped on /my/* before it can leak.
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
    if (page.data.user) identifyUser(page.data.user);
    else resetIdentity();
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

  <main class="flex-1">
    {@render children()}
  </main>

  {#if !hideFooter}
    <Footer />
  {/if}
</div>

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
