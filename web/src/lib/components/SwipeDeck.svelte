<script lang="ts">
  import { onMount, untrack } from 'svelte';
  import { page } from '$app/state';
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { Heart, RotateCcw, SlidersHorizontal, X } from '@lucide/svelte';
  import { ApiError, api } from '#lib/api.js';
  import { isAuthenticated } from '#lib/auth.svelte.js';
  import { FilterStore, filtersToParams } from '#lib/filters.js';
  import { toSearchString } from '#lib/urlSearchString.js';
  import { latestOnly } from '#lib/latestOnly.js';
  import type { Job, FacetCounts } from '#lib/types.js';
  import { markViewed } from '#lib/viewedJobs.svelte.js';
  import { focusTrap } from '#lib/actions/focusTrap.js';
  import FilterModal from './filters/FilterModal.svelte';
  import SwipeCard from './SwipeCard.svelte';
  import SwipeCardBody from './SwipeCardBody.svelte';
  import { FLY_MS, SwipeGesture } from './swipeGesture.svelte.js';
  import States from './States.svelte';

  type Judgement = 'save' | 'dismiss';

  // Batch size and the "top up before it runs dry" threshold.
  const LIMIT = 12;
  const PREFETCH_AT = 4;

  // The deck honors the /jobs filters carried in the URL. Unlike the old fixed
  // snapshot, the filters are now editable in-session: a FilterStore mirrors them
  // to the URL and the deck reloads whenever the (debounced) filters change, so the
  // top-left "Filters" button can refine the deck without leaving the page.
  const filters = new FilterStore(new URLSearchParams(page.url.search));
  const deckParams = () => filtersToParams(filters.applied);

  // queue holds only UNJUDGED cards; the active one is queue[0]. seen dedups by
  // slug so a prefetch that races a just-sent save/dismiss can't re-add a card.
  // Reassigned wholesale (never mutated in place), so raw skips per-item proxying.
  let queue = $state.raw<Job[]>([]);
  // eslint-disable-next-line svelte/prefer-svelte-reactivity -- deliberately non-reactive dedup registry; never read in a reactive context
  const seen = new Set<string>();
  // Single-step undo: the last judged card and how it was judged.
  let last = $state<{ job: Job; kind: Judgement } | null>(null);

  let loading = $state(false);
  let error = $state(false);
  let exhausted = $state(false);

  // Monotonic reload id. A filter change bumps it so an in-flight fetch started
  // under the old filters is discarded instead of appending stale cards.
  let gen = 0;

  // Drag state of the active card; SwipeCard renders it.
  const gesture = new SwipeGesture();

  // The filter drawer (reuses the shared job FilterModal).
  let modalOpen = $state(false);

  // Live facet distribution feeding the modal's dynamic selects + "Show N jobs"
  // preview. latestOnly discards a slow earlier response, mirroring JobsView.
  let counts = $state.raw<FacetCounts | null>(null);
  const refreshCounts = latestOnly(
    () => api.facetCounts(deckParams()),
    (c) => (counts = c),
  );
  const previewCount = (params: URLSearchParams) => api.facetCounts(params).then((c) => c.total);

  const current = $derived(queue[0] ?? null);
  const next = $derived(queue[1] ?? null);

  async function loadBatch() {
    if (loading || exhausted) return;
    loading = true;
    error = false;
    const myGen = gen;
    try {
      // Exclusion-driven pagination: every shown card is recorded as viewed and
      // thus excluded server-side, so each fetch returns the head of the un-seen
      // deck (offset 0). Held-but-unshown cards aren't excluded yet, so they can
      // reappear in the page — `seen` dedups them client-side.
      const slice = await api.swipeDeck(deckParams(), LIMIT);
      if (myGen !== gen) return; // filters changed mid-flight — discard this page
      const fresh = slice.items.filter((j) => !seen.has(j.public_slug));
      for (const j of fresh) seen.add(j.public_slug);
      queue = [...queue, ...fresh];
      // A page with no rows at all means the un-seen set is drained.
      if (slice.items.length === 0) exhausted = true;
    } catch {
      if (myGen === gen) error = true;
    } finally {
      if (myGen === gen) loading = false;
    }
  }

  function prefetchIfLow() {
    if (queue.length <= PREFETCH_AT && !exhausted && !loading) void loadBatch();
  }

  // Reset the deck to an empty state and refetch under the current filters. Bumps
  // `gen` first so any in-flight loadBatch discards its result and its finally
  // can't clear our fresh `loading`.
  function resetDeck() {
    gen++;
    queue = [];
    seen.clear();
    last = null;
    loading = false;
    error = false;
    exhausted = false;
    gesture.reset();
    void loadBatch();
  }

  // Judge the active card: fly it off toward its side, then drop it from the queue
  // and persist optimistically. A failed write restores the card at the front so
  // the decision isn't silently lost. Guarded against re-entry mid-flight.
  function judge(kind: Judgement) {
    const job = queue[0];
    if (!job || gesture.exiting) return;

    // Fling the card off toward its side (fade only under reduced motion).
    gesture.fling(kind);
    last = { job, kind };
    // Snapshot the reload generation: a resetDeck() mid-flight bumps `gen` and
    // replaces `queue` wholesale, so the deferred slice below must not touch a
    // queue that no longer belongs to this swipe.
    const myGen = gen;

    const send = kind === 'save' ? api.saveJob(job.public_slug) : api.dismissJob(job.public_slug);
    send.catch((err) => {
      // A 404 means the job is gone: the source renamed it and the slug was
      // re-minted, so this card can never be judged. Restoring it would trap
      // the user on a card they can never get past, so it stays dropped - and
      // `last` is cleared, because undo must not bring a gone job back either.
      if (err instanceof ApiError && err.status === 404) {
        if (last?.job.public_slug === job.public_slug) last = null;
        return;
      }
      // Restore the card only if it hasn't since been undone (undo() clears `last`)
      // or superseded by a later judge() — re-queuing unconditionally would put back
      // a card the user already restored, leaving a permanent duplicate in the deck.
      if (last?.job.public_slug === job.public_slug) {
        queue = [job, ...queue];
        last = null;
      }
    });

    // After the fly-off, drop the card. The next one is a fresh DOM node (keyed by
    // slug), so it mounts centered at dragX 0 — no slide from the edge — and plays
    // the grow-in keyframe on mount.
    setTimeout(() => {
      if (myGen !== gen) return; // resetDeck() already replaced the queue and its own state
      queue = queue.slice(1);
      gesture.settle();
      prefetchIfLow();
    }, FLY_MS);
  }

  // Undo the last decision: clear its server mark and put the card back on top.
  function undo() {
    // Bail during the fly-off: `last` is set for the whole FLY_MS animation window,
    // so without the `exiting` gate an undo mid-flight would re-queue the card while
    // judge()'s deferred slice and failed-send restore are still pending — leaving a
    // duplicate. Undo is available once the card has settled (exiting=false).
    if (!last || gesture.exiting) return;
    const { job, kind } = last;
    last = null;
    queue = [job, ...queue];
    void (kind === 'save' ? api.unsaveJob(job.public_slug) : api.undismissJob(job.public_slug));
  }

  // Leave the deck. Carry the (possibly refined) filters back to the list so it
  // reflects what the swipe session ended on.
  function close() {
    const qs = toSearchString(deckParams());
    // eslint-disable-next-line svelte/no-navigation-without-resolve -- resolve() applied to the path; the rule can't see through the appended query string
    goto(resolve('/jobs') + (qs ? `?${qs}` : ''));
  }

  // --- Keyboard (desktop) --------------------------------------------------
  function onKeydown(e: KeyboardEvent) {
    const t = e.target as HTMLElement | null;
    if (t && (t.tagName === 'INPUT' || t.tagName === 'TEXTAREA')) return;
    if (modalOpen) return; // let the filter drawer own the keyboard while it's open
    switch (e.key) {
      case 'ArrowRight':
        e.preventDefault();
        judge('save');
        break;
      case 'ArrowLeft':
        e.preventDefault();
        judge('dismiss');
        break;
      case 'ArrowUp':
        e.preventDefault();
        openDetail();
        break;
      case 'u':
      case 'U':
        undo();
        break;
      case 'Escape':
        close();
        break;
    }
  }

  function openDetail() {
    if (!current) return;
    window.open(resolve('/jobs/[slug]', { slug: current.public_slug }), '_blank', 'noopener');
  }

  // Load the deck + counts, and reload both whenever the (debounced) filters change
  // — the top-left Filters drawer commits to the store, which mirrors the URL and
  // lands here. The first run IS the initial load: resetDeck() on the already-empty
  // deck is just a plain load, so one path serves mount and every later change.
  $effect(() => {
    void filters.applied; // track the debounced snapshot
    // Only `filters.applied` is a real dependency. resetDeck()/loadBatch() read and
    // write loading/exhausted/queue, so tracking them here would self-invalidate the
    // effect into an infinite reload loop — untrack the imperative work (mirrors
    // JobsView's reload effect).
    untrack(() => {
      refreshCounts();
      resetDeck();
    });
  });

  // Cancel the store's pending debounce when the deck unmounts.
  onMount(() => () => filters.dispose());

  // Record a view the moment a card becomes the active one — silent, best-effort
  // history (a failed write must never break the deck). This marks the card viewed
  // for the browse-list dimming and, with the server-side deck exclusion, keeps a
  // card from re-appearing across sessions. Re-firing on undo (which restores a
  // judged card to the front) just touches viewed_at again — idempotent.
  $effect(() => {
    const slug = current?.public_slug;
    if (!slug || !isAuthenticated()) return;
    api
      .recordJobView(slug)
      .then(() => markViewed(slug))
      .catch(() => {});
  });
</script>

<svelte:window onkeydown={onKeydown} />

<!-- Full-screen overlay: covers the app chrome, so it IS a modal — focusTrap keeps Tab
     off the TopBar/Footer behind it. Safe-area padding keeps the top bar clear of the
     notch and the actions clear of the home bar. -->
<div
  class="swipe-overlay fixed inset-0 z-50 flex flex-col bg-background text-foreground"
  role="dialog"
  aria-modal="true"
  aria-labelledby="swipe-deck-title"
  {@attach focusTrap()}
>
  <!-- Top bar: Filters (left) · Close (right). Safe-area top padding clears the notch. -->
  <header class="flex shrink-0 items-center justify-between gap-3 px-4 pb-2 pt-safe">
    <button
      type="button"
      onclick={() => (modalOpen = true)}
      aria-label="Filters"
      title="Filters"
      class="relative flex h-11 w-11 items-center justify-center rounded-full border border-border bg-card text-foreground shadow-sm transition hover:bg-accent"
    >
      <SlidersHorizontal class="size-5" />
      {#if filters.active > 0}
        <span
          class="absolute -right-1 -top-1 flex h-5 min-w-5 items-center justify-center rounded-full bg-brand px-1 text-[11px] font-semibold leading-none text-brand-foreground"
        >
          {filters.active}
        </span>
      {/if}
    </button>

    <!-- The route's only h1 (the card title below is an h2). -->
    <h1 id="swipe-deck-title" class="text-sm font-medium text-muted-foreground">Swipe to triage</h1>

    <button
      type="button"
      onclick={close}
      aria-label="Close swipe mode"
      title="Close"
      class="flex h-11 w-11 items-center justify-center rounded-full border border-border bg-card text-foreground shadow-sm transition hover:bg-accent"
    >
      <X class="size-5" />
    </button>
  </header>

  {#if current}
    {#key current.public_slug}
      <SwipeCard
        {gesture}
        label={`Job: ${current.title} at ${current.company || 'unknown company'}`}
        hasNext={!!next}
        onCommit={judge}
      >
        <SwipeCardBody job={current} />
      </SwipeCard>
    {/key}

    <!-- Action buttons (a tap alternative to swiping, and the desktop path). -->
    <footer class="flex shrink-0 items-center justify-center gap-6 px-4 pb-safe pt-2">
      <button
        type="button"
        aria-label="Skip (dismiss)"
        class="flex h-16 w-16 items-center justify-center rounded-full border border-border bg-card text-destructive shadow-md transition hover:bg-accent active:scale-95"
        onclick={() => judge('dismiss')}
      >
        <X class="size-7" />
      </button>
      <button
        type="button"
        aria-label="Undo last"
        class="flex h-12 w-12 items-center justify-center rounded-full border border-border bg-card text-muted-foreground shadow-md transition hover:bg-accent active:scale-95 disabled:opacity-40"
        disabled={!last}
        onclick={undo}
      >
        <RotateCcw class="size-5" />
      </button>
      <button
        type="button"
        aria-label="Save"
        class="flex h-16 w-16 items-center justify-center rounded-full border border-border bg-card text-brand shadow-md transition hover:bg-accent active:scale-95"
        onclick={() => judge('save')}
      >
        <Heart class="size-7" />
      </button>
    </footer>

    <p class="hidden shrink-0 pb-3 text-center text-xs text-muted-foreground md:block">
      <kbd class="rounded border border-border px-1">←</kbd> skip ·
      <kbd class="rounded border border-border px-1">→</kbd> save ·
      <kbd class="rounded border border-border px-1">↑</kbd> open ·
      <kbd class="rounded border border-border px-1">U</kbd> undo ·
      <kbd class="rounded border border-border px-1">Esc</kbd> close
    </p>
  {:else if loading}
    <div class="flex flex-1 items-center justify-center">
      <States state="loading" />
    </div>
  {:else if error}
    <div class="flex flex-1 items-center justify-center">
      <States state="error" message="Failed to load the deck." />
    </div>
  {:else}
    <div class="flex flex-1 flex-col items-center justify-center gap-3 px-6 text-center">
      <p class="text-lg font-semibold">You're all caught up</p>
      <p class="text-sm text-muted-foreground">No more jobs match these filters.</p>
      <div class="mt-2 flex items-center gap-3">
        <button
          type="button"
          onclick={() => (modalOpen = true)}
          class="rounded-lg border border-border bg-card px-4 py-2 text-sm font-medium transition hover:bg-accent"
        >
          Adjust filters
        </button>
        <button
          type="button"
          onclick={close}
          class="rounded-lg border border-border bg-card px-4 py-2 text-sm font-medium transition hover:bg-accent"
        >
          Back to all jobs
        </button>
      </div>
    </div>
  {/if}
</div>

<!-- The shared job filter drawer. Applying commits to the FilterStore, which
     mirrors the URL and resets the deck via the effect below. -->
<FilterModal
  store={filters}
  {counts}
  savedSearches
  open={modalOpen}
  onClose={() => (modalOpen = false)}
  {previewCount}
/>
