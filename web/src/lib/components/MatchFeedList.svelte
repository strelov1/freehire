<script lang="ts">
  import { onMount, tick } from 'svelte';
  import { api } from '#lib/api.js';
  import { ApiError, isPlanRefusal } from '#lib/api-core.js';
  import { feedSession } from '#lib/feedSession.svelte.js';
  import type { FeedCard, FeedMeta } from '#lib/types.js';
  import { ensureDismissedLoaded, isDismissed } from '#lib/dismissedJobs.svelte.js';
  import { feedSource } from '#lib/feedSource.svelte.js';
  import { Badge } from '#lib/ui/index.js';
  import FeedCaughtUp from './FeedCaughtUp.svelte';
  import FeedDegraded from './FeedDegraded.svelte';
  import FeedNoProfile from './FeedNoProfile.svelte';
  import FeedSessionEnded from './FeedSessionEnded.svelte';
  import FeedSourceEmpty from './FeedSourceEmpty.svelte';
  import FeedNewEnd from './FeedNewEnd.svelte';
  import InfiniteScroll from './InfiniteScroll.svelte';
  import JobRow from './JobRow.svelte';
  import MatchBlock from './MatchBlock.svelte';
  import HiddenToast from './HiddenToast.svelte';
  import HideCompanyConfirm from './HideCompanyConfirm.svelte';
  import FeedCompanyMenu from './FeedCompanyMenu.svelte';
  import { HiddenJobUndo } from './hiddenJobUndo.svelte';
  import MatchesUpgrade from './MatchesUpgrade.svelte';
  import States from './States.svelte';

  let { active = true, onChooseSource }: { active?: boolean; onChooseSource?: () => void } = $props();

  // The match feed as a list: every card with its explanation, save and hide in the corner.
  // "Show more" rather than scroll-to-load, like /jobs: an auto-growing page puts the
  // footer out of reach.
  const PAGE = 20;

  // The source revision this instance was mounted for. A response that lands after a pick
  // belongs to the old source: it must not touch the shared source state, retry, or paint.
  const rev = feedSource.version;
  const stale = () => rev !== feedSource.version;
  let list = $state<HTMLUListElement | null>(null);
  let cards = $state<FeedCard[]>([]);
  let meta = $state<FeedMeta | null>(null);
  let loading = $state(true);
  let loadingMore = $state(false);
  let failed = $state(false);
  // The first page of this session came back with no card: a saved search that matches nothing.
  // Not the same as running out of cards later ("You're all caught up").
  let firstEmpty = $state(false);
  // The plan lapsed since the page loaded: the API refuses with 402.
  let paywalled = $state(false);
  // The signed cursor for the next page; absent on the last one.
  let cursor = $state<string | undefined>();
  const hidden = new HiddenJobUndo();
  // The company being hidden, its confirmation, and the button to give focus back to on cancel.
  let hideTarget = $state<{ slug: string; name: string } | null>(null);
  let hideTrigger: HTMLElement | null = null;
  // Numbers every page-1 restart: a reply from before a hide-company reload belongs to the old list.
  let epoch = 0;
  const visible = $derived(cards.filter((c) => !isDismissed(c.public_slug)));

  // New cards come first. Once they run out the rest waits behind a pause screen. The
  // server's seen_cursor is posted once, when the candidate scrolls past the last new card,
  // never just because the list rendered: a list shows every new card at once.
  let restShown = $state(false);
  let seenCursor = $state<string | undefined>();
  // The new phase has ended once the server has handed out a seen_cursor.
  let newEnded = $state(false);
  const newCards = $derived(visible.filter((c) => c.is_new));
  const restCards = $derived(visible.filter((c) => !c.is_new));
  const paused = $derived(newEnded && newCards.length > 0 && !restShown && (restCards.length > 0 || !!cursor));
  // Where the last new card ends: before the pause screen, or the list's end when no rest follows.
  const newEndInView = $derived(!!seenCursor && newCards.length > 0 && (paused || (!cursor && restCards.length === 0)));

  function postSeen() {
    // A list behind the swipe view has not been looked at: it must not move the watermark.
    if (!active) return;
    const c = seenCursor;
    seenCursor = undefined;
    if (c) api.markFeedSeen(c).catch(() => {}); // the watermark only decides what is new next time
  }

  function showRest() {
    postSeen();
    restShown = true;
    if (restCards.length === 0) void showMore();
  }

  async function fetchPage(next?: string): Promise<void> {
    const mine = epoch;
    const outdated = () => stale() || mine !== epoch;
    if (!next) {
      // A restart is a new session: nothing of the old one carries over.
      restShown = false;
      newEnded = false;
      seenCursor = undefined;
    }
    let res;
    try {
      res = await api.meFeed(PAGE, next);
    } catch (e) {
      if (outdated()) return;
      // An expired or rejected cursor (or a source switched in another tab): start over.
      if (next && e instanceof ApiError && e.status === 400) return fetchPage();
      throw e;
    }
    if (outdated()) return;
    if (next && res.meta.reason === 'source_changed') {
      // A changed profile or search can invalidate cards we already loaded.
      cards = [];
      return fetchPage();
    }
    // A restart that lands on another source (switched in another tab): the old cards are not its cards.
    if (meta && (meta.source !== res.meta.source || meta.saved_search_id !== res.meta.saved_search_id)) cards = [];
    meta = res.meta;
    feedSource.fromMeta(res.meta);
    if (!next) firstEmpty = cards.length === 0 && res.data.length === 0 && !res.meta.next_cursor;
    if (res.meta.seen_cursor) {
      seenCursor = res.meta.seen_cursor;
      newEnded = true;
    }
    cursor = res.meta.next_cursor;
    // A card shown again after a restart takes this session's copy: its New mark may have changed.
    const fresh = new Map(res.data.map((c) => [c.public_slug, c]));
    const kept = cards.map((c) => fresh.get(c.public_slug) ?? c);
    const have = new Set(kept.map((c) => c.public_slug));
    cards = [...kept, ...res.data.filter((c) => !have.has(c.public_slug))];
  }

  // A lapsed plan gets the upgrade screen, an ended session goes back through sign-in; the rest is an error.
  function onLoadError(e: unknown) {
    if (isPlanRefusal(e)) paywalled = true;
    else if (!feedSession.lost(e)) failed = true;
  }

  // A restart lands on page 1 again; cards already shown are skipped, not repeated.
  async function showMore() {
    const mine = epoch;
    loadingMore = true;
    try {
      await fetchPage(cursor);
    } catch (e) {
      onLoadError(e);
    } finally {
      if (mine === epoch) loadingMore = false;
    }
  }

  function askHideCompany(card: FeedCard, trigger: HTMLButtonElement) {
    if (!card.company_slug) return;
    hideTrigger = trigger;
    hideTarget = { slug: card.company_slug, name: card.company };
  }

  // Cancelled: focus returns to the button. Hidden: the exclusion list changed, so every card and
  // cursor held is from the old one; start over on the same source and fence replies still in flight.
  async function onHideClosed(result: 'hidden' | 'cancelled') {
    hideTarget = null;
    await tick();
    if (result === 'cancelled') {
      if (active) hideTrigger?.focus();
      return;
    }
    epoch++;
    loadingMore = false;
    cards = [];
    cursor = undefined;
    failed = false;
    loading = true;
    try {
      await fetchPage();
    } catch (e) {
      onLoadError(e);
    } finally {
      if (!stale()) loading = false;
    }
    await tick();
    const target = active && list?.querySelector<HTMLElement>('h3');
    if (target) {
      target.tabIndex = -1;
      target.focus();
    }
  }

  async function start() {
    failed = false;
    loading = true;
    try {
      await fetchPage();
    } catch (e) {
      onLoadError(e);
    } finally {
      loading = false;
      if (!stale() && feedSource.takeCardFocus(rev, 'list')) {
        await tick();
        const heading = active && list?.querySelector('h3');
        if (heading) {
          heading.tabIndex = -1;
          heading.focus();
        }
      }
    }
  }

  onMount(() => {
    void ensureDismissedLoaded();
    void start();
  });
</script>

{#if meta?.degraded === 'no_match_vector'}
  <FeedDegraded />
{/if}

{#if feedSession.ended}
  <!-- Only the visible list acts: swipe mode mounts its own over this one. -->
  {#if active}<FeedSessionEnded />{/if}
{:else if paywalled}
  <MatchesUpgrade price={null} />
{:else if loading}
  <States state="loading" />
{:else if failed && cards.length === 0}
  <States state="error" message="Failed to load your matches." onRetry={start} />
{:else if meta?.reason === 'search_deleted'}
  <FeedSourceEmpty kind="deleted" focus={active} />
{:else if meta?.source === 'saved_search' && meta.reason}
  <FeedSourceEmpty kind="unusable" focus={active} />
{:else if meta?.source === 'saved_search' && firstEmpty}
  <FeedSourceEmpty kind="no_matches" focus={active} />
{:else if meta?.reason}
  <FeedNoProfile focus={active} />
{:else if visible.length === 0 && !cursor}
  <FeedCaughtUp focus={active} {onChooseSource} />
{:else}
  <ul class="flex flex-col gap-3" bind:this={list}>
    {#each paused ? newCards : visible as card (card.public_slug)}
      <li>
        <JobRow job={card} onHide={hidden.onHide} onUnauthorized={feedSession.end}>
          {#snippet footer()}
            {#if card.is_new}<Badge variant="brand" class="mb-2">New</Badge>{/if}
            <MatchBlock match={card.match} />
            {#if card.company_slug}
              <FeedCompanyMenu
                company={card.company || card.company_slug}
                onHide={(trigger) => askHideCompany(card, trigger)}
              />
            {/if}
          {/snippet}
        </JobRow>
      </li>
    {/each}
  </ul>
  <InfiniteScroll enabled={newEndInView && !loading && active} onLoad={postSeen} />
  {#if paused}
    <FeedNewEnd onContinue={showRest} focus={false} />
  {:else if cursor}
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

{#if hideTarget && !feedSession.ended}
  <HideCompanyConfirm slug={hideTarget.slug} name={hideTarget.name} onclose={onHideClosed} />
{/if}

{#if hidden.pending}
  {#key hidden.pending.slug}
    <HiddenToast onUndo={hidden.undo} onClose={hidden.close} />
  {/key}
{/if}
