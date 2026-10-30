<script lang="ts">
  import { untrack } from 'svelte';
  import { ApiError, api } from '#lib/api.js';
  import { messagingDock } from '#lib/messagingDock.svelte.js';
  import type { BuddyFeedItem } from '#lib/types.js';
  import { LoadMore, Tabs } from '#lib/ui/index.js';
  import BuddyFeedEntry from './BuddyFeedEntry.svelte';
  import BuddyMessageButton from './BuddyMessageButton.svelte';
  import States from './States.svelte';

  // The buddy feed with its two views (similar to you, everyone) and keyset paging. Message
  // goes through the messaging dock, which is on every page of the space but Chats.
  let mode = $state<'similar' | 'all'>('similar');
  let feed = $state<BuddyFeedItem[]>([]);
  let cursor = $state<string | null>(null);
  let feedBusy = $state(false);
  // Which read failed: the first page (shown in place of the list, with a retry) or a later
  // one (shown under the list by LoadMore).
  let feedError = $state<'first' | 'more' | null>(null);

  async function loadFeed(next = false, requestedMode = mode) {
    if (feedBusy) return;
    feedBusy = true;
    feedError = null;
    try {
      const page = await api.listBuddyFeed(requestedMode, next ? cursor ?? undefined : undefined);
      if (requestedMode !== mode) return;
      feed = next ? [...feed, ...page.data] : page.data;
      cursor = page.meta.next_cursor;
    } catch (err) {
      // A stale cursor answers 400: start over from the first page rather than failing.
      if (next && err instanceof ApiError && err.status === 400) {
        try {
          const firstPage = await api.listBuddyFeed(requestedMode);
          if (requestedMode !== mode) return;
          feed = firstPage.data;
          cursor = firstPage.meta.next_cursor;
        } catch { if (requestedMode === mode) feedError = 'more'; }
      } else if (requestedMode === mode) feedError = next ? 'more' : 'first';
    } finally {
      feedBusy = false;
      if (requestedMode !== mode) { feed = []; cursor = null; void loadFeed(false, mode); }
    }
  }

  $effect(() => { const currentMode = mode; untrack(() => { feed = []; cursor = null; void loadFeed(false, currentMode); }); });
</script>

<Tabs bind:value={() => mode, (next) => (mode = next as 'similar' | 'all')} tabs={[{ value: 'similar', label: 'Similar to you' }, { value: 'all', label: 'Everyone' }]}>
  {#if feed.length === 0 && feedError === 'first'}<States state="error" message="Could not load the feed." onRetry={() => void loadFeed()} />
  {:else if feed.length === 0 && feedBusy}<States state="loading" />
  {:else if feed.length === 0}<States state="empty" message="No application updates yet." />
  {:else}
    <ul class="flex flex-col gap-3">{#each feed as item (item.id)}<BuddyFeedEntry {item}>{#snippet actions()}<BuddyMessageButton handle={item.handle} conversations={messagingDock.conversations} oninvite={(h) => messagingDock.messageBuddy(h)} onconversation={(id) => messagingDock.openConversation(id)} />{/snippet}</BuddyFeedEntry>{/each}</ul>
    {#if cursor}<LoadMore loading={feedBusy} error={feedError === 'more'} onclick={() => void loadFeed(true)} />{/if}
  {/if}
</Tabs>
