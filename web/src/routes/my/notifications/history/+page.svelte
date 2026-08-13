<script lang="ts">
  import { onMount } from 'svelte';
  import { resolve } from '$app/paths';
  import { api } from '$lib/api';
  import { notificationCenter } from '$lib/notificationCenter.svelte';
  import { Paginator } from '$lib/paginated.svelte';
  import NotificationCard from '$lib/components/NotificationCard.svelte';
  import LoadMore from '$lib/components/LoadMore.svelte';
  import States from '$lib/components/States.svelte';
  import type { NotificationItem } from '$lib/types';

  // The full notification history — every subscription-digest/reminder/nudge
  // delivery ever recorded, not just the header bell's most-recent page. Own
  // Paginator rather than the shared notificationCenter singleton (which only
  // ever holds one fixed-size page for the dropdown): this view genuinely pages
  // through everything, so it needs its own item list and offset.

  const pager = new Paginator<NotificationItem>(
    async (limit, offset) => {
      const res = await api.getNotifications(limit, offset);
      return { items: res.data, total: res.meta.total, hasMore: offset + res.data.length < res.meta.total };
    },
    { keyOf: (item) => item.id },
  );

  onMount(() => {
    void pager.start();
  });

  async function loadMore() {
    await pager.loadMore();
  }

  // A card's own click already tells the shared notificationCenter (so the bell's
  // badge stays in sync); this page separately patches its own list so the row
  // stops looking unread without a full re-fetch.
  function onCardActivate(item: NotificationItem) {
    if (item.read_at != null) return;
    const readAt = new Date().toISOString();
    pager.items = pager.items.map((n) => (n.id === item.id ? { ...n, read_at: readAt } : n));
  }

  async function markAllRead() {
    await notificationCenter.markAllRead();
    const readAt = new Date().toISOString();
    pager.items = pager.items.map((n) => (n.read_at == null ? { ...n, read_at: readAt } : n));
  }

  const hasUnread = $derived(pager.items.some((n) => n.read_at == null));
</script>

<svelte:head>
  <title>Notification history — freehire</title>
</svelte:head>

<div class="max-w-2xl">
  <div class="mb-4 flex items-center justify-between">
    <div>
      <h1 class="text-lg font-semibold">Notification history</h1>
      <p class="text-sm text-muted-foreground">
        Every new-job match, reminder, and application nudge you've been sent — see the
        <a href={resolve('/my/notifications')} class="underline underline-offset-2 hover:text-foreground">
          delivery settings
        </a>
        to change what you get.
      </p>
    </div>
    {#if hasUnread}
      <button
        type="button"
        onclick={markAllRead}
        class="shrink-0 text-xs font-medium text-muted-foreground transition-colors hover:text-foreground"
      >
        Mark all read
      </button>
    {/if}
  </div>

  {#if pager.status === 'loading'}
    <States state="loading" rows={6} />
  {:else if pager.status === 'error'}
    <States state="error" />
  {:else if pager.items.length === 0}
    <States state="empty" message="No notifications yet." />
  {:else}
    <ul class="overflow-hidden rounded-md border border-border">
      {#each pager.items as item (item.id)}
        <NotificationCard {item} onactivate={() => onCardActivate(item)} />
      {/each}
    </ul>

    {#if pager.hasMore}
      <LoadMore loading={pager.loadingMore} error={pager.loadMoreError} onclick={loadMore} />
    {/if}
  {/if}
</div>
