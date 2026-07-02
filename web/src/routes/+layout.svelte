<script lang="ts">
  import { navigating } from '$app/state';
  import { onMount } from 'svelte';
  import { initTheme } from '$lib/theme.svelte';
  import { isAuthenticated } from '$lib/auth.svelte';
  import { resetViewedJobs } from '$lib/viewedJobs.svelte';
  import { savedSearches } from '$lib/savedSearches.svelte';
  import { searchProfiles } from '$lib/searchProfiles.svelte';
  import { notifications } from '$lib/notifications.svelte';
  import TopBar from '$lib/components/TopBar.svelte';
  import Footer from '$lib/components/Footer.svelte';
  import '../app.css';

  let { children } = $props();

  // Apply the persisted theme and start tracking the OS preference once mounted.
  // A no-FOUC inline script in app.html already set the class before paint.
  onMount(() => initTheme());

  // Drop every per-user store the moment the session ends. logout() re-resolves via
  // invalidateAll() — a soft client navigation, so these module-singleton stores
  // survive it and would otherwise show the previous user's data to whoever signs in
  // next on the same tab. Centralized in the always-mounted root layout so it fires
  // regardless of route (the per-component load effects only cover their own store
  // while mounted; viewedJobs, used on /jobs, had no reset at all). Idempotent — a
  // reset of already-empty stores while signed out is a no-op.
  $effect(() => {
    if (!isAuthenticated()) {
      resetViewedJobs();
      savedSearches.reset();
      searchProfiles.reset();
      notifications.reset();
    }
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
    class="fixed inset-x-0 top-0 z-50 h-0.5 overflow-hidden bg-primary/20"
    role="progressbar"
    aria-label="Loading"
    aria-busy="true"
  >
    <div class="nav-progress-bar h-full w-2/5 bg-primary"></div>
  </div>
{/if}

<div class="flex min-h-svh flex-col">
  <TopBar />

  <main class="flex-1">
    {@render children()}
  </main>

  <Footer />
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
