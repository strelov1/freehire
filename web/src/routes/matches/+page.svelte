<script lang="ts">
  import { page } from '$app/state';
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { Layers } from '@lucide/svelte';
  import type { PageData } from './$types';
  import MatchFeedList from '#lib/components/MatchFeedList.svelte';
  import MatchesUpgrade from '#lib/components/MatchesUpgrade.svelte';
  import MatchSwipe from '#lib/components/MatchSwipe.svelte';

  const { data }: { data: PageData } = $props();

  // The feed is the page; swipe is a full-screen mode over it, kept in the URL so a reload
  // or a shared link reopens it.
  const swipe = $derived(page.url.searchParams.get('mode') === 'swipe');
  const setMode = (m: 'feed' | 'swipe') =>
    goto(resolve(m === 'swipe' ? '/matches?mode=swipe' : '/matches'), { replace: true });
</script>

<svelte:head>
  <title>Matches — freehire</title>
  <meta name="robots" content="noindex" />
</svelte:head>

<div class="mx-auto flex w-full max-w-3xl flex-col gap-4 px-3 py-5 sm:px-4 sm:py-6">
  <header class="flex items-center justify-between gap-3">
    <div class="min-w-0">
      <h1 class="text-2xl font-semibold">Matches</h1>
      <p class="text-sm text-muted-foreground">Jobs that fit your profile, best first</p>
    </div>
    {#if data.paid}
      <button
        type="button"
        class="flex shrink-0 items-center gap-2 rounded-full border border-border bg-card px-4 py-2 text-sm font-medium transition hover:bg-accent"
        onclick={() => setMode('swipe')}
      >
        <Layers class="size-4" aria-hidden="true" />
        Swipe
      </button>
    {/if}
  </header>

  {#if data.paid}
    <MatchFeedList />
  {:else}
    <MatchesUpgrade price={data.price} />
  {/if}
</div>

{#if swipe && data.paid}
  <MatchSwipe onClose={() => setMode('feed')} />
{/if}
