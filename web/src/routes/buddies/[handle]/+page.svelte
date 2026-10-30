<script lang="ts">
  import { BadgeCheck } from '@lucide/svelte';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { ApiError, api } from '#lib/api.js';
  import { buildActivityGridFromCounts } from '#lib/activityGrid.js';
  import { buddyHeading, buddyYearTotals, localDay } from '#lib/buddyPresent.js';
  import { countryLabel, skillLabel } from '#lib/facets.js';
  import { messagingDock } from '#lib/messagingDock.svelte.js';
  import type { BuddyActivity, BuddyFeedItem, BuddyProfile, BuddySimilar } from '#lib/types.js';
  import { Chip, CountryFlag, LoadMore, TabStrip, tabStripId } from '#lib/ui/index.js';
  import ActivityYear from '#lib/components/ActivityYear.svelte';
  import BuddyAvatar from '#lib/components/BuddyAvatar.svelte';
  import BuddyFeedEntry from '#lib/components/BuddyFeedEntry.svelte';
  import BuddyIdentity from '#lib/components/BuddyIdentity.svelte';
  import BuddyMessageButton from '#lib/components/BuddyMessageButton.svelte';
  import CandidateCardSections from '#lib/components/CandidateCardSections.svelte';
  import States from '#lib/components/States.svelte';

  // One interview buddy: who they are (heading, country, handle), their year in numbers,
  // their skills, then their own activity — the buddy feed narrowed to them, under every
  // rule the feed keeps — or their career from the anonymous card. A handle that is
  // unknown, has left, or sits on either side of a block answers 404, and this page does
  // not try to tell those apart.

  const handle = $derived(page.params.handle ?? '');

  let view = $state<'loading' | 'ready' | 'unavailable' | 'error'>('loading');
  let profile = $state<BuddyProfile | null>(null);
  let feed = $state<BuddyFeedItem[]>([]);
  let cursor = $state<string | null>(null);
  let feedBusy = $state(false);
  let feedError = $state(false);
  let similar = $state<BuddySimilar[]>([]);

  // The year grid and the year in numbers both read the activity days, which count exactly
  // what this member's feed lists. A failure to read them costs those two, not the page.
  let activity = $state<BuddyActivity | null>(null);
  const totals = $derived(activity ? buddyYearTotals(activity.days) : null);

  type Tab = 'activity' | 'experience';
  const TABS: readonly { id: Tab; label: string }[] = [
    { id: 'activity', label: 'Activity' },
    { id: 'experience', label: 'Experience' },
  ];
  const PANEL_ID = 'buddy-profile-sections';
  let tab = $state<Tab>('activity');

  const SKILLS_SHOWN = 10;
  let allSkills = $state(false);

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
        const next = await api.listBuddyMemberFeed(current, historyCursor ?? undefined, DAY_PAGE_SIZE);
        if (current !== handle) return;
        history = [...history, ...next.data];
        historyCursor = next.meta.next_cursor;
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
    activity = null; dayView = null; tab = 'activity'; allSkills = false;
    history = []; historyCursor = null; historyPages = 0; historyLoad = Promise.resolve();
    api.getBuddyActivity(current).then(
      (a) => { if (!cancelled) activity = a; },
      () => {},
    );
    // The rail is a suggestion; its failure leaves it empty.
    api.listBuddySimilar(5).then(
      (s) => { if (!cancelled) similar = s.filter((m) => m.handle !== current).slice(0, 4); },
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
        view = err instanceof ApiError && err.status === 404 ? 'unavailable' : 'error';
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

<div class="flex flex-col gap-4 xl:flex-row xl:gap-6">
  <div class="flex min-w-0 flex-1 flex-col gap-4">
    {#if view === 'loading'}<States state="loading" />
    {:else if view === 'unavailable'}<States state="empty" message="This member is not available." />
    {:else if view === 'error'}<States state="error" />
    {:else if profile}
      {@const summary = profile.card_summary}
      <section class="rounded-xl border border-border bg-card">
        <div class="flex flex-col gap-4 p-5">
          <div class="flex flex-wrap items-start gap-4">
            <BuddyAvatar handle={profile.handle} size="lg" class="size-18 shrink-0 text-2xl" />
            <div class="flex min-w-0 flex-1 basis-48 flex-col gap-1">
              <div class="flex flex-wrap items-center gap-2">
                <h1 class="text-2xl font-semibold tracking-tight">{buddyHeading(summary)}</h1>
                {#if summary.country}<CountryFlag code={summary.country} label={countryLabel(summary.country)} />{/if}
                {#if totals && totals.offer > 0}
                  <span class="inline-flex items-center gap-1 rounded-full bg-brand px-2 py-0.5 text-xs font-semibold text-brand-foreground">
                    <BadgeCheck class="size-3.5" aria-hidden="true" />{totals.offer === 1 ? 'Got an offer' : `${totals.offer} offers`}
                  </span>
                {/if}
              </div>
              <p class="flex flex-wrap gap-x-3 gap-y-0.5 text-sm text-muted-foreground">
                <span>{profile.handle}</span>
                {#if summary.country}<span>{countryLabel(summary.country)}</span>{/if}
                {#if profile.card.total_years}<span>{profile.card.total_years} {profile.card.total_years === 1 ? 'year' : 'years'} of experience</span>{/if}
              </p>
            </div>
            <!-- Wraps under the header on a narrow screen rather than squeezing the heading. -->
            <div class="shrink-0 max-sm:w-full">
              <BuddyMessageButton
                handle={profile.handle}
                conversations={messagingDock.conversations}
                size="md"
                oninvite={(h) => messagingDock.messageBuddy(h)}
                onconversation={(id) => messagingDock.openConversation(id)}
              />
            </div>
          </div>

          {#if totals}
            <!-- The same events the year grid counts, so the two never disagree. -->
            <p class="flex flex-wrap gap-x-5 gap-y-1 text-sm tabular-nums">
              <span><b class="font-semibold">{totals.applied}</b> <span class="text-muted-foreground">applied</span></span>
              <span><b class="font-semibold">{totals.interview}</b> <span class="text-muted-foreground">{totals.interview === 1 ? 'interview' : 'interviews'}</span></span>
              <span><b class="font-semibold">{totals.offer}</b> <span class="text-muted-foreground">{totals.offer === 1 ? 'offer' : 'offers'}</span></span>
              <span class="text-muted-foreground">in the last year</span>
            </p>
          {/if}

          {#if profile.card.skills.length}
            <div class="flex flex-wrap gap-1.5">
              {#each allSkills ? profile.card.skills : profile.card.skills.slice(0, SKILLS_SHOWN) as skill (skill)}
                <Chip>{skillLabel(skill)}</Chip>
              {/each}
              {#if !allSkills && profile.card.skills.length > SKILLS_SHOWN}
                <button type="button" class="rounded-full px-2 text-xs font-medium text-muted-foreground hover:bg-muted hover:text-foreground" onclick={() => (allSkills = true)}>
                  +{profile.card.skills.length - SKILLS_SHOWN} more
                </button>
              {/if}
            </div>
          {/if}
        </div>
        <div class="border-t border-border px-3">
          <TabStrip tabs={TABS} active={tab} onSelect={(next) => (tab = next)} label="Member sections" panelId={PANEL_ID} />
        </div>
      </section>

      <div id={PANEL_ID} role="tabpanel" aria-labelledby={tabStripId(PANEL_ID, tab)} class="flex flex-col gap-4">
        {#if tab === 'activity'}
          {#if grid}
            <!-- ActivityYear draws its own card; a second frame around it nests two. -->
            <section>
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
          <section class="flex flex-col gap-1 rounded-xl border border-border bg-card p-4">
            <h2 class="text-sm font-semibold">Activity</h2>
            {#if feed.length === 0}
              <p class="text-sm text-muted-foreground">No application updates yet.</p>
            {:else}
              <ul>{#each feed as item (item.id)}<BuddyFeedEntry {item} showHandle={false} />{/each}</ul>
              {#if cursor}<LoadMore loading={feedBusy} error={feedError} onclick={() => void loadMore()} />{/if}
            {/if}
          </section>
        {:else}
          <!-- The roles are cards of their own; a frame around them would nest them. -->
          <section class="px-1">
            <CandidateCardSections card={profile.card} emptyMessage="This member has not added a profile card yet." />
          </section>
        {/if}
      </div>
    {/if}
  </div>

  {#if similar.length}
    <aside class="xl:sticky xl:top-20 xl:w-68 xl:shrink-0 xl:self-start">
      <section class="rounded-xl border border-border bg-card p-4">
        <div class="mb-2 flex items-baseline justify-between">
          <h2 class="text-sm font-semibold">Similar members</h2>
          <a href={resolve('/buddies/network')} class="text-xs font-medium text-brand-strong hover:underline">See all</a>
        </div>
        <ul class="flex flex-col divide-y divide-border">
          {#each similar as person (person.handle)}
            <li class="py-2.5"><BuddyIdentity handle={person.handle} summary={person.card_summary} size="sm" /></li>
          {/each}
        </ul>
      </section>
    </aside>
  {/if}
</div>
