<script lang="ts">
  import { untrack } from 'svelte';
  import { ApiError, api } from '#lib/api.js';
  import { buddyFeedDays, buddyFeedPosts } from '#lib/buddyFeed.js';
  import { buddyRelativeDay, localDay } from '#lib/buddyPresent.js';
  import type { BuddyFeedItem } from '#lib/types.js';
  import { LoadMore, Tabs } from '#lib/ui/index.js';
  import BuddyPost from './BuddyPost.svelte';
  import States from './States.svelte';

  // The buddy feed with its two views (similar to you, everyone) and keyset paging, read as
  // posts under day headings (buddyFeed.ts). Message goes through the messaging dock, which
  // is on every page of the space but Chats.
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

  // Regrouped over everything loaded, so a later page can still fold into a day already shown.
  const days = $derived(buddyFeedDays(buddyFeedPosts(feed)));
  const today = localDay();
  const dayLabel = (day: string) => {
    const label = buddyRelativeDay(day, today);
    return label.charAt(0).toUpperCase() + label.slice(1);
  };

  $effect(() => { const currentMode = mode; untrack(() => { feed = []; cursor = null; void loadFeed(false, currentMode); }); });
</script>

<Tabs bind:value={() => mode, (next) => (mode = next as 'similar' | 'all')} tabs={[{ value: 'similar', label: 'Similar to you' }, { value: 'all', label: 'Everyone' }]}>
  {#if feed.length === 0 && feedError === 'first'}<States state="error" message="Could not load the feed." onRetry={() => void loadFeed()} />
  {:else if feed.length === 0 && feedBusy}<States state="loading" />
  {:else if feed.length === 0}<States state="empty" message="No application updates yet." />
  {:else}
    <div class="flex flex-col gap-3">
      {#each days as group (group.day)}
        <h2 class="flex items-center gap-3 pt-1 text-xs font-medium text-muted-foreground after:h-px after:flex-1 after:bg-border">
          <time datetime={group.day}>{dayLabel(group.day)}</time>
        </h2>
        {#each group.posts as post (post.key)}<BuddyPost {post} />{/each}
      {/each}
    </div>
    {#if cursor}<LoadMore loading={feedBusy} error={feedError === 'more'} onclick={() => void loadFeed(true)} />{/if}
  {/if}
</Tabs>
