<script lang="ts">
  import { page } from '$app/state';
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { onMount, tick } from 'svelte';
  import { Layers } from '@lucide/svelte';
  import type { PageData } from './$types';
  import FeedSourceMenu from '#lib/components/FeedSourceMenu.svelte';
  import { feedSession } from '#lib/feedSession.svelte.js';
  import { feedSource } from '#lib/feedSource.svelte.js';
  import MatchFeedList from '#lib/components/MatchFeedList.svelte';
  import MatchesSignedOut from '#lib/components/MatchesSignedOut.svelte';
  import MatchesUpgrade from '#lib/components/MatchesUpgrade.svelte';
  import MatchSwipe from '#lib/components/MatchSwipe.svelte';

  const { data }: { data: PageData } = $props();

  // The feed is the page; swipe is a full-screen mode over it, kept in the URL so a reload
  // or a shared link reopens it.
  const swipe = $derived(page.url.searchParams.get('mode') === 'swipe');
  // A fresh visit starts with a live session, and leaving drops a verdict that came too late.
  onMount(() => {
    feedSession.reset();
    return () => feedSession.reset();
  });
  let listRevision = $state(0);
  let listStale = $state(false);
  $effect.pre(() => {
    if (swipe || !listStale) return;
    listStale = false;
    listRevision++;
  });
  // Each mode has one source menu: the page's in the feed, swipe's own in swipe.
  let sourceMenu = $state<ReturnType<typeof FeedSourceMenu>>();
  let swipeView = $state<ReturnType<typeof MatchSwipe>>();
  // A pick remounts swipe: focus the live pill, unless the user has left swipe meanwhile.
  async function swipePicked() {
    await tick();
    if (swipe) swipeView?.focusSourcePill();
  }
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
    </div>
    {#if data.paid && !feedSession.ended}
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
    {#if !feedSession.ended && !swipe}<FeedSourceMenu bind:this={sourceMenu} />{/if}
    {#key feedSource.version}
      {#key listRevision}
        <MatchFeedList active={!swipe} onChooseSource={() => sourceMenu?.openMenu()} />
      {/key}
    {/key}
  {:else if data.signedOut}
    <MatchesSignedOut />
  {:else}
    <MatchesUpgrade price={data.price} />
  {/if}
</div>

{#if swipe && data.paid}
  {#key feedSource.version}
    <MatchSwipe
      bind:this={swipeView}
      onSourcePicked={swipePicked}
      onClose={() => setMode('feed')}
      onCompanyHidden={() => (listStale = true)}
    />
  {/key}
{/if}
