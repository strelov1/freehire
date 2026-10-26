<script lang="ts">
  import { ArrowLeft } from '@lucide/svelte';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { ApiError, api } from '#lib/api.js';
  import { messagingDock } from '#lib/messagingDock.svelte.js';
  import BuddyFeedEntry from '#lib/components/BuddyFeedEntry.svelte';
  import BuddyMessageButton from '#lib/components/BuddyMessageButton.svelte';
  import CandidateCardView from '#lib/components/CandidateCardView.svelte';
  import States from '#lib/components/States.svelte';
  import ActivityYear from '#lib/components/ActivityYear.svelte';
  import { buildActivityGridFromCounts } from '#lib/activityGrid.js';
  import type { BuddyActivity, BuddyFeedItem, BuddyProfile } from '#lib/types.js';
  import { LoadMore } from '#lib/ui/index.js';
  import { BUDDY_HEADING_FALLBACK, localDay } from '#lib/buddyPresent.js';

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
  let feedError = $state(false);

  // The year grid counts exactly what this member's feed lists (the server holds it to the
  // feed's own visibility rule), by UTC day. A failure to read it costs the grid, not the page.
  let activity = $state<BuddyActivity | null>(null);

  // A UTC day can run ahead of the reader's calendar west of Greenwich. The feed already
  // reads such a day as "today" (buddyRelativeDay), so the grid files it on today's square
  // rather than dropping it past the window's end.
  const today = localDay();
  const gridDay = (day: string) => (day > today ? today : day);

  const grid = $derived(
    activity ? buildActivityGridFromCounts(activity.days.map((d) => [gridDay(d.day), d.count] as const)) : null,
  );

  // A selected day lists that day's entries from the member's feed, read newest first until
  // the pages run past the day — and no further than DAY_PAGES pages, so one click on a day
  // a year back cannot walk an unbounded history. The pages are kept for the next click.
  const DAY_PAGE_SIZE = 50;
  const DAY_PAGES = 10;
  let history: BuddyFeedItem[] = [];
  let historyCursor: string | null = null;
  let historyPages = 0;
  // Loads are chained, so two quick clicks never append the same page twice.
  let historyLoad: Promise<void> = Promise.resolve();
  let dayView = $state<{ key: string; status: 'loading' | 'ready' | 'error'; entries: BuddyFeedItem[]; complete: boolean } | null>(null);

  function readThrough(key: string, current: string): Promise<void> {
    historyLoad = historyLoad.then(async () => {
      while (historyPages < DAY_PAGES && (historyPages === 0 || (historyCursor && (history[history.length - 1]?.day ?? '') >= key))) {
        const page = await api.listBuddyMemberFeed(current, historyCursor ?? undefined, DAY_PAGE_SIZE);
        if (current !== handle) return;
        history = [...history, ...page.data];
        historyCursor = page.meta.next_cursor;
        historyPages += 1;
      }
    });
    return historyLoad;
  }

  async function showDay(key: string, count: number) {
    if (count === 0) {
      dayView = { key, status: 'ready', entries: [], complete: true };
      return;
    }
    const current = handle;
    dayView = { key, status: 'loading', entries: [], complete: false };
    try {
      await readThrough(key, current);
    } catch {
      // A failed page leaves the chain usable for the next click.
      historyLoad = Promise.resolve();
      if (current === handle && dayView?.key === key) dayView = { key, status: 'error', entries: [], complete: false };
      return;
    }
    if (current !== handle || dayView?.key !== key) return;
    const complete = !historyCursor || (history[history.length - 1]?.day ?? '') < key;
    dayView = { key, status: 'ready', entries: history.filter((e) => gridDay(e.day) === key), complete };
  }

  $effect(() => {
    const current = handle;
    let cancelled = false;
    view = 'loading'; profile = null; feed = []; cursor = null; feedError = false;
    activity = null; dayView = null;
    history = []; historyCursor = null; historyPages = 0; historyLoad = Promise.resolve();
    api.getBuddyActivity(current).then(
      (a) => { if (!cancelled) activity = a; },
      () => {},
    );
    void (async () => {
      try {
        // The conversation list is the shared one the messaging dock reads, so "Message"
        // here and the dock agree on whether a conversation with this member exists.
        const [p, first] = await Promise.all([
          api.getBuddyProfile(current),
          api.listBuddyMemberFeed(current),
          // A failed list read keeps the last one; it must not take the page down.
          messagingDock.refresh().catch(() => {}),
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
    feedBusy = true; feedError = false;
    try {
      const next = await api.listBuddyMemberFeed(current, cursor);
      if (current !== handle) return;
      feed = [...feed, ...next.data];
      cursor = next.meta.next_cursor;
    } catch (err) {
      if (err instanceof ApiError && err.status === 404) view = 'unavailable';
      else feedError = true;
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
    <!-- No photoSrc, ever: a buddy is drawn as the silhouette (see BuddyIdentity). -->
    <CandidateCardView card={profile.card} subtitle={profile.handle} headingFallback={BUDDY_HEADING_FALLBACK} skillsSide="inline" emptyMessage="This member has not added a profile card yet.">
      {#snippet actions()}
        <BuddyMessageButton
          handle={profile?.handle ?? ''}
          conversations={messagingDock.conversations}
          size="md"
          oninvite={(h) => messagingDock.messageBuddy(h)}
          onconversation={(id) => messagingDock.openConversation(id)}
        />
      {/snippet}
      {#if grid}
        <section class="flex flex-col gap-4">
          <ActivityYear
            {grid}
            heading="Their last year"
            caption="Applications and interview or offer moves this member shares in the feed."
            onselect={(day) => showDay(day.key, day.count)}
          >
            {#snippet panel(selected)}
              {#if selected.count === 0}
                <p class="text-sm text-muted-foreground">Nothing shared on this day.</p>
              {:else if dayView?.key !== selected.key || dayView.status === 'loading'}
                <States state="loading" rows={2} />
              {:else if dayView.status === 'error'}
                <p role="alert" class="text-sm text-destructive">Could not load this day.</p>
              {:else}
                {#if dayView.entries.length > 0}
                  <ul>{#each dayView.entries as item (item.id)}<BuddyFeedEntry {item} showHandle={false} />{/each}</ul>
                {/if}
                {#if !dayView.complete}
                  <p class="text-sm text-muted-foreground">
                    {dayView.entries.length > 0 ? 'Not every entry of this day can be listed here.' : 'This day is too far back to list here.'}
                  </p>
                {/if}
              {/if}
            {/snippet}
          </ActivityYear>
        </section>
      {/if}
      <section class="flex flex-col gap-2">
        <h2 class="text-sm font-medium">Activity</h2>
        {#if feed.length === 0}
          <p class="text-sm text-muted-foreground">No application updates yet.</p>
        {:else}
          <ul>{#each feed as item (item.id)}<BuddyFeedEntry {item} showHandle={false} />{/each}</ul>
          {#if cursor}<LoadMore loading={feedBusy} error={feedError} onclick={() => void loadMore()} />{/if}
        {/if}
      </section>
    </CandidateCardView>
  {/if}
</div>
