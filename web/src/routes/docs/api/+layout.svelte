<script lang="ts">
  // The API docs shell: a persistent two-column frame (filterable nav rail +
  // content) shared by the landing page and every per-endpoint page, so the nav
  // never re-renders on navigation and its active state tracks the URL. Below the
  // `lg` breakpoint the rail collapses into a disclosure drawer, since the fixed
  // sidebar has no room — it closes itself on navigation.
  import { afterNavigate } from '$app/navigation';
  import DocsNav from '$lib/components/DocsNav.svelte';

  let { children } = $props();

  let navOpen = $state(false);
  afterNavigate(() => (navOpen = false));
</script>

<div class="mx-auto w-full max-w-6xl px-4 py-6">
  <!-- Mobile: collapsible nav drawer. -->
  <div class="mb-6 lg:hidden">
    <button
      type="button"
      onclick={() => (navOpen = !navOpen)}
      aria-expanded={navOpen}
      class="flex w-full items-center justify-between rounded-lg border border-border bg-secondary/40 px-3 py-2 text-sm font-medium text-foreground"
    >
      <span class="flex items-center gap-2">
        <svg class="size-4 text-muted-foreground" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true">
          <path d="M4 6h16M4 12h16M4 18h16" stroke-linecap="round" />
        </svg>
        Endpoints
      </span>
      <svg
        class={`size-4 text-muted-foreground transition-transform ${navOpen ? 'rotate-180' : ''}`}
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        stroke-width="2"
        aria-hidden="true"
      >
        <path d="m6 9 6 6 6-6" stroke-linecap="round" stroke-linejoin="round" />
      </svg>
    </button>
    {#if navOpen}
      <div class="mt-2 rounded-lg border border-border bg-background p-2">
        <DocsNav sticky={false} />
      </div>
    {/if}
  </div>

  <div class="grid gap-x-12 lg:grid-cols-[15rem_minmax(0,1fr)]">
    <aside class="hidden lg:block">
      <DocsNav />
    </aside>
    <div class="min-w-0">
      {@render children()}
    </div>
  </div>
</div>
