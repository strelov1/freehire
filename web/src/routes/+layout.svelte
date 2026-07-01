<script lang="ts">
  import { resolve } from '$app/paths';
  import { navigating } from '$app/state';
  import { onMount } from 'svelte';
  import { initTheme } from '$lib/theme.svelte';
  import TopBar from '$lib/components/TopBar.svelte';
  import ProviderIcon from '$lib/components/ProviderIcon.svelte';
  import '../app.css';

  let { children } = $props();

  // Apply the persisted theme and start tracking the OS preference once mounted.
  // A no-FOUC inline script in app.html already set the class before paint.
  onMount(() => initTheme());
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

  <footer class="border-t border-border">
    <div
      class="mx-auto flex w-full max-w-6xl items-center justify-between gap-3 px-4 py-3 text-xs text-muted-foreground"
    >
      <p>Free, open-source IT job aggregator.</p>
      <div class="flex shrink-0 items-center gap-4">
        <a
          href={resolve('/cli')}
          class="shrink-0 font-medium text-foreground transition-colors hover:text-muted-foreground"
        >
          CLI
        </a>
        <a
          href={resolve('/docs/api')}
          class="shrink-0 font-medium text-foreground transition-colors hover:text-muted-foreground"
        >
          API
        </a>
        <a
          href="https://t.me/freehiredev"
          target="_blank"
          rel="noopener noreferrer"
          class="inline-flex shrink-0 items-center gap-1.5 font-medium text-foreground transition-colors hover:text-muted-foreground"
        >
          <ProviderIcon provider="telegram" /> Ask a question
        </a>
        <a
          href="https://github.com/strelov1/freehire"
          target="_blank"
          rel="noopener noreferrer"
          class="inline-flex shrink-0 items-center gap-1.5 font-medium text-foreground transition-colors hover:text-muted-foreground"
        >
          <ProviderIcon provider="github" /> GitHub
        </a>
      </div>
    </div>
  </footer>
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
