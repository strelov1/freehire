<script lang="ts">
  import { ArrowLeft } from '@lucide/svelte';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { ApiError, api } from '#lib/api.js';
  import { buddyDock } from '#lib/buddyDock.svelte.js';
  import BuddyFeedEntry from '#lib/components/BuddyFeedEntry.svelte';
  import BuddyMessageButton from '#lib/components/BuddyMessageButton.svelte';
  import CandidateCardView from '#lib/components/CandidateCardView.svelte';
  import States from '#lib/components/States.svelte';
  import type { BuddyFeedItem, BuddyProfile } from '#lib/types.js';
  import { Button } from '#lib/ui/index.js';

  // One interview buddy: their anonymous card and, under it, their own activity — the
  // buddy feed narrowed to them, under every rule the feed keeps. A handle that is
  // unknown, has left, or sits on either side of a block answers 404, and this page does
  // not try to tell those apart.

  const handle = $derived(page.params.handle ?? '');

  let view = $state<'loading' | 'ready' | 'unavailable' | 'not-member' | 'error'>('loading');
  let profile = $state<BuddyProfile | null>(null);
  let feed = $state<BuddyFeedItem[]>([]);
  let cursor = $state<string | null>(null);
  let feedBusy = $state(false);
  let feedError = $state('');

  $effect(() => {
    const current = handle;
    let cancelled = false;
    view = 'loading'; profile = null; feed = []; cursor = null; feedError = '';
    void (async () => {
      try {
        // The conversation list is the shared one the messaging dock reads, so "Message"
        // here and the dock agree on whether a conversation with this member exists.
        const [p, first] = await Promise.all([
          api.getBuddyProfile(current),
          api.listBuddyMemberFeed(current),
          buddyDock.refresh(),
        ]);
        if (cancelled) return;
        profile = p; feed = first.data; cursor = first.meta.next_cursor;
        view = 'ready';
      } catch (err) {
        if (cancelled) return;
        if (err instanceof ApiError && err.status === 404) view = 'unavailable';
        else if (err instanceof ApiError && err.status === 403) view = 'not-member';
        else view = 'error';
      }
    })();
    return () => { cancelled = true; };
  });

  async function loadMore() {
    if (feedBusy || !cursor) return;
    const current = handle;
    feedBusy = true; feedError = '';
    try {
      const next = await api.listBuddyMemberFeed(current, cursor);
      if (current !== handle) return;
      feed = [...feed, ...next.data];
      cursor = next.meta.next_cursor;
    } catch (err) {
      if (err instanceof ApiError && err.status === 404) view = 'unavailable';
      else feedError = 'Could not load more activity.';
    } finally { feedBusy = false; }
  }

</script>

<svelte:head><title>{handle} · Interview buddies</title></svelte:head>

<div class="flex max-w-5xl flex-col gap-5">
  <a href={resolve('/my/buddies')} class="flex w-fit items-center gap-1.5 text-sm text-muted-foreground hover:text-foreground">
    <ArrowLeft class="size-4" aria-hidden="true" />
    Interview buddies
  </a>

  {#if view === 'loading'}<States state="loading" />
  {:else if view === 'unavailable'}<States state="empty" message="This member is not available." />
  {:else if view === 'not-member'}
    <section class="rounded-lg border border-border p-6 text-sm">
      <p>Join interview buddies to see other members.</p>
      <a href={resolve('/my/buddies')} class="mt-3 inline-block font-medium underline hover:no-underline">Go to interview buddies</a>
    </section>
  {:else if view === 'error'}<States state="error" />
  {:else if profile}
    <CandidateCardView card={profile.card} title={profile.handle} skillsSide="right" emptyMessage="This member has not added a profile card yet.">
      {#snippet actions()}
        <BuddyMessageButton
          handle={profile?.handle ?? ''}
          conversations={buddyDock.conversations}
          size="md"
          oninvite={(h) => buddyDock.openNew(h)}
          onconversation={(id) => buddyDock.openConversation(id)}
        />
      {/snippet}
      <section class="flex flex-col gap-2">
        <h2 class="text-sm font-medium">Activity</h2>
        {#if feed.length === 0}
          <p class="text-sm text-muted-foreground">No application updates yet.</p>
        {:else}
          <ul>{#each feed as item (item.id)}<BuddyFeedEntry {item} showHandle={false} />{/each}</ul>
          {#if feedError}<p role="alert" class="text-sm text-destructive">{feedError}</p>{/if}
          {#if cursor}<Button variant="outline" class="w-fit" onclick={loadMore} disabled={feedBusy}>Load more</Button>{/if}
        {/if}
      </section>
    </CandidateCardView>
  {/if}
</div>
