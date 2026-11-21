<script lang="ts">
  import { onMount } from 'svelte';
  import { resolve } from '$app/paths';
  import { api } from '#lib/api.js';
  import { isPlanRefusal } from '#lib/api-core.js';
  import type { FeedCard, FeedMeta } from '#lib/types.js';
  import { ensureDismissedLoaded, isDismissed } from '#lib/dismissedJobs.svelte.js';
  import { isSaved } from '#lib/savedJobs.svelte.js';
  import { hasViewed } from '#lib/viewedJobs.svelte.js';
  import JobRow from './JobRow.svelte';
  import MatchBlock from './MatchBlock.svelte';
  import HiddenToast from './HiddenToast.svelte';
  import { HiddenJobUndo } from './hiddenJobUndo.svelte';
  import MatchesUpgrade from './MatchesUpgrade.svelte';
  import States from './States.svelte';

  // The match feed as a list: every card with its explanation, save and hide in the corner.
  // "Show more" rather than scroll-to-load, like /jobs: an auto-growing page puts the
  // footer out of reach.
  const PAGE = 20;

  let cards = $state<FeedCard[]>([]);
  let meta = $state<FeedMeta | null>(null);
  let loading = $state(true);
  let loadingMore = $state(false);
  let failed = $state(false);
  // The plan lapsed since the page loaded: the API refuses with 402.
  let paywalled = $state(false);
  let more = $state(false);
  const hidden = new HiddenJobUndo();
  const visible = $derived(cards.filter((c) => !isDismissed(c.public_slug)));

  async function fetchPage(offset: number) {
    const res = await api.meFeed(PAGE, offset);
    meta = res.meta;
    more = res.data.length === PAGE;
    const have = new Set(cards.map((c) => c.public_slug));
    cards = [...cards, ...res.data.filter((c) => !have.has(c.public_slug))];
  }

  // The server leaves out every job the reader has touched since (saved, hidden, opened),
  // so the next page starts after only the loaded cards still untouched.
  async function showMore() {
    loadingMore = true;
    try {
      const untouched = cards.filter(
        (c) => !isSaved(c.public_slug) && !isDismissed(c.public_slug) && !hasViewed(c.public_slug),
      ).length;
      await fetchPage(untouched);
    } catch (e) {
      if (isPlanRefusal(e)) paywalled = true;
      else failed = true;
    } finally {
      loadingMore = false;
    }
  }

  onMount(async () => {
    void ensureDismissedLoaded();
    try {
      await fetchPage(0);
    } catch (e) {
      if (isPlanRefusal(e)) paywalled = true;
      else failed = true;
    } finally {
      loading = false;
    }
  });
</script>

{#if meta?.degraded === 'no_match_vector'}
  <p class="rounded-lg border border-border bg-card px-4 py-3 text-sm text-muted-foreground">
    Your profile has no skills, so these are the newest jobs that fit your filters, not ranked by match.
    <a class="underline" href={resolve('/my/profile')}>Add skills</a>
  </p>
{/if}

{#if paywalled}
  <MatchesUpgrade price={null} />
{:else if loading}
  <States state="loading" />
{:else if failed && cards.length === 0}
  <States state="error" message="Failed to load your matches." />
{:else if meta?.reason}
  <div class="flex flex-col items-center gap-3 py-16 text-center">
    <p class="text-lg font-semibold">Tell us what you are looking for</p>
    <p class="text-sm text-muted-foreground">
      Matches are built from your profile: skills, level and where you want to work.
    </p>
    <a
      class="rounded-lg border border-border bg-card px-4 py-2 text-sm font-medium hover:bg-accent"
      href={resolve('/my/profile')}>Fill in my profile</a
    >
  </div>
{:else if visible.length === 0}
  <div class="flex flex-col items-center gap-3 py-16 text-center">
    <p class="text-lg font-semibold">You're all caught up</p>
    <p class="text-sm text-muted-foreground">No more jobs match your profile right now.</p>
  </div>
{:else}
  <ul class="flex flex-col gap-3">
    {#each visible as card (card.public_slug)}
      <li>
        <JobRow job={card} onHide={hidden.onHide}>
          {#snippet footer()}
            <MatchBlock match={card.match} />
          {/snippet}
        </JobRow>
      </li>
    {/each}
  </ul>
  {#if more}
    <button
      type="button"
      class="mx-auto rounded-lg border border-border bg-card px-5 py-2.5 text-sm font-medium transition hover:bg-accent disabled:opacity-50"
      disabled={loadingMore}
      onclick={showMore}
    >
      {loadingMore ? 'Loading…' : 'Show more matches'}
    </button>
  {/if}
{/if}

{#if hidden.pending}
  {#key hidden.pending.slug}
    <HiddenToast onUndo={hidden.undo} onClose={hidden.close} />
  {/key}
{/if}
