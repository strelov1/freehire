<script lang="ts">
  import type { Snippet } from 'svelte';
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import { isAuthenticated } from '$lib/auth.svelte';
  import { cn } from '$lib/utils';

  let { children }: { children: Snippet } = $props();

  // Each view is its own URL so it is linkable, bookmarkable, and survives a
  // reload. Board is the index route; Pipeline/History/AI fit get their own paths.
  const path = $derived(page.url.pathname);
  // Board (index) matches exactly so it is not also active on the child routes.
  const boardActive = $derived(path === '/my/tracking');
  const pipelineActive = $derived(path.startsWith('/my/tracking/pipeline'));
  const historyActive = $derived(path.startsWith('/my/tracking/history'));
  const analysesActive = $derived(path.startsWith('/my/tracking/analyses'));

  const tabClass = (active: boolean) =>
    cn(
      'rounded-md px-3 py-1.5 text-sm transition-colors',
      active
        ? 'bg-secondary font-medium text-secondary-foreground'
        : 'text-muted-foreground hover:bg-accent hover:text-accent-foreground',
    );
</script>

<svelte:head>
  <!-- Base title (the child pages override it with their view name); set here so
       it is present even in the signed-out state, where children do not render. -->
  <title>Tracking — freehire</title>
  <!-- Personal pages: keep them out of search results. -->
  <meta name="robots" content="noindex" />
</svelte:head>

<div class="mx-auto w-full max-w-6xl px-4 py-6">
  {#if !isAuthenticated()}
    <p class="py-12 text-center text-sm text-muted-foreground">
      Sign in to see the jobs you saved, applied to, and are tracking.
    </p>
  {:else}
    <div class="flex flex-col gap-4">
      <h1 class="text-2xl font-semibold tracking-tight">Tracking</h1>

      <div role="tablist" aria-label="Tracking view" class="flex items-center gap-1">
        <a role="tab" aria-selected={boardActive} href={resolve('/my/tracking')} class={tabClass(boardActive)}>
          Board
        </a>
        <a
          role="tab"
          aria-selected={pipelineActive}
          href={resolve('/my/tracking/pipeline')}
          class={tabClass(pipelineActive)}
        >
          Pipeline
        </a>
        <a
          role="tab"
          aria-selected={historyActive}
          href={resolve('/my/tracking/history')}
          class={tabClass(historyActive)}
        >
          History
        </a>
        <a
          role="tab"
          aria-selected={analysesActive}
          href={resolve('/my/tracking/analyses')}
          class={tabClass(analysesActive)}
        >
          Matches
        </a>
      </div>

      {@render children()}
    </div>
  {/if}
</div>
