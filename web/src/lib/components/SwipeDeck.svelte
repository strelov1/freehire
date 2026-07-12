<script lang="ts">
  import { onMount, untrack } from 'svelte';
  import { page } from '$app/state';
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { Heart, RotateCcw, SlidersHorizontal, X } from '@lucide/svelte';
  import { api } from '$lib/api';
  import { isAuthenticated } from '$lib/auth.svelte';
  import { cardTags, formatSalary } from '$lib/enrichment';
  import { FilterStore, filtersToParams } from '$lib/filters';
  import { latestOnly } from '$lib/latestOnly';
  import type { Job, FacetCounts } from '$lib/types';
  import { Badge } from '$lib/ui';
  import { markViewed } from '$lib/viewedJobs.svelte';
  import CompanyLogo from './CompanyLogo.svelte';
  import FilterModal from './filters/FilterModal.svelte';
  import JobMatch from './JobMatch.svelte';
  import States from './States.svelte';

  type Judgement = 'save' | 'dismiss';

  // Batch size and the "top up before it runs dry" threshold. COMMIT_PX is how
  // far a card must be dragged before the gesture counts as a decision.
  const LIMIT = 12;
  const PREFETCH_AT = 4;
  const COMMIT_PX = 100;
  // How long the judged card takes to fly off-screen before it's dropped.
  const FLY_MS = 300;

  // The deck honors the /jobs filters carried in the URL. Unlike the old fixed
  // snapshot, the filters are now editable in-session: a FilterStore mirrors them
  // to the URL and the deck reloads whenever the (debounced) filters change, so the
  // top-left "Filters" button can refine the deck without leaving the page.
  const filters = new FilterStore(page.url.searchParams);
  const deckParams = () => filtersToParams(filters.applied);

  // queue holds only UNJUDGED cards; the active one is queue[0]. seen dedups by
  // slug so a prefetch that races a just-sent save/dismiss can't re-add a card.
  // Reassigned wholesale (never mutated in place), so raw skips per-item proxying.
  let queue = $state.raw<Job[]>([]);
  const seen = new Set<string>();
  // Single-step undo: the last judged card and how it was judged.
  let last = $state<{ job: Job; kind: Judgement } | null>(null);

  let loading = $state(false);
  let error = $state(false);
  let exhausted = $state(false);

  // Monotonic reload id. A filter change bumps it so an in-flight fetch started
  // under the old filters is discarded instead of appending stale cards.
  let gen = 0;

  // Live drag offset of the active card (px); 0 when settled.
  let dragX = $state(0);
  let dragging = $state(false);
  let startX = 0;
  // The active card is mid fly-off: guards re-entry and drives the fade-out.
  let exiting = $state(false);

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

  // The card transitions (springs back or flies off) whenever it isn't tracking
  // the finger. The grow-in intro is a CSS keyframe on mount, not a transition —
  // see the `.swipe-card` animation below — so no frame-timing dance is needed.
  const animate = $derived(!dragging);
  // Tilt tracks the drag but is capped so the fly-off doesn't spin wildly; the
  // card fades out as it leaves.
  const rotation = $derived(Math.max(-18, Math.min(18, dragX / 24)));
  const cardOpacity = $derived(exiting ? 0 : 1);
  // Decision stamps fade in with drag distance, reaching full at the commit point.
  const stampOpacity = $derived(Math.min(1, Math.abs(dragX) / COMMIT_PX));

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
    dragX = 0;
    dragging = false;
    exiting = false;
    void loadBatch();
  }

  // Judge the active card: fly it off toward its side, then drop it from the queue
  // and persist optimistically. A failed write restores the card at the front so
  // the decision isn't silently lost. Guarded against re-entry mid-flight.
  function judge(kind: Judgement) {
    const job = queue[0];
    if (!job || exiting) return;

    // Fling the card off toward its side. dragging=false lets the transition run;
    // dragX past the viewport width carries it fully off-screen.
    const dir = kind === 'save' ? 1 : -1;
    exiting = true;
    dragging = false;
    dragX = dir * (window.innerWidth + 200);
    last = { job, kind };

    const send = kind === 'save' ? api.saveJob(job.public_slug) : api.dismissJob(job.public_slug);
    send.catch(() => {
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
      queue = queue.slice(1);
      dragX = 0;
      exiting = false;
      prefetchIfLow();
    }, FLY_MS);
  }

  // Undo the last decision: clear its server mark and put the card back on top.
  function undo() {
    // Bail during the fly-off: `last` is set for the whole FLY_MS animation window,
    // so without the `exiting` gate an undo mid-flight would re-queue the card while
    // judge()'s deferred slice and failed-send restore are still pending — leaving a
    // duplicate. Undo is available once the card has settled (exiting=false).
    if (!last || exiting) return;
    const { job, kind } = last;
    last = null;
    queue = [job, ...queue];
    void (kind === 'save' ? api.unsaveJob(job.public_slug) : api.undismissJob(job.public_slug));
  }

  // Leave the deck. Carry the (possibly refined) filters back to the list so it
  // reflects what the swipe session ended on.
  function close() {
    const qs = deckParams().toString();
    goto(resolve('/') + (qs ? `?${qs}` : ''));
  }

  // --- Pointer drag on the active card (touch + mouse) ---------------------
  // The card now both swipes horizontally AND scrolls its content vertically, so a
  // gesture's axis is locked on first movement: a mostly-vertical drag is left to
  // native scroll (touch-action: pan-y on the scroll area), a horizontal one drives
  // the swipe. We therefore defer setPointerCapture/dragging until we know it's a
  // horizontal swipe — capturing up front would swallow every vertical scroll.
  let startY = 0;
  let axis: 'none' | 'x' | 'y' = 'none';
  let activePointer = -1;
  const AXIS_LOCK_PX = 8;

  function onPointerDown(e: PointerEvent) {
    if (!current || exiting) return;
    // Let clicks on links/buttons inside the card through — capturing the pointer
    // for a drag would swallow their click (e.g. "Open full details").
    if ((e.target as HTMLElement).closest('a, button')) return;
    startX = e.clientX;
    startY = e.clientY;
    axis = 'none';
    activePointer = e.pointerId;
  }
  function onPointerMove(e: PointerEvent) {
    if (e.pointerId !== activePointer || axis === 'y') return;
    const dx = e.clientX - startX;
    const dy = e.clientY - startY;
    if (axis === 'none') {
      // Wait for enough travel to reveal intent before committing to an axis.
      const adx = Math.abs(dx);
      const ady = Math.abs(dy);
      if (adx < AXIS_LOCK_PX && ady < AXIS_LOCK_PX) return;
      if (ady > adx) {
        axis = 'y'; // vertical intent — hand off to native scroll, never swipe
        return;
      }
      axis = 'x';
      dragging = true;
      (e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
    }
    dragX = dx;
  }
  function onPointerUp() {
    activePointer = -1;
    axis = 'none';
    if (!dragging) return;
    if (dragX > COMMIT_PX) judge('save');
    else if (dragX < -COMMIT_PX) judge('dismiss');
    else {
      // Not far enough — spring back.
      dragX = 0;
      dragging = false;
    }
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

  const salary = $derived(current?.enrichment ? formatSalary(current.enrichment) : null);
  const tags = $derived(current ? cardTags(current) : []);
  const skills = $derived(current?.skills?.slice(0, 6) ?? []);
  // The model-written one-line synopsis (only on enriched jobs); shown as the card's
  // lead, with the full description kept below as the fallback / detail.
  const summary = $derived(current?.enrichment?.summary ?? '');
</script>

<svelte:window onkeydown={onKeydown} />

<!-- Full-screen overlay: covers the app chrome like a modal. Safe-area padding
     keeps the top bar clear of the notch and the actions clear of the home bar. -->
<div class="swipe-overlay fixed inset-0 z-50 flex flex-col bg-background text-foreground">
  <!-- Top bar: Filters (left) · Close (right). Safe-area top padding clears the notch. -->
  <header
    class="flex shrink-0 items-center justify-between gap-3 px-4 pb-2 pt-[max(0.75rem,env(safe-area-inset-top))]"
  >
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

    <span class="text-sm font-medium text-muted-foreground">Swipe to triage</span>

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
    <!-- Card stage: the card fills the available height, Tinder-style. -->
    <main class="relative min-h-0 flex-1 px-4 py-3">
      <div class="relative mx-auto h-full w-full max-w-md select-none">
        {#if next}
          <div
            class="absolute inset-0 scale-95 rounded-3xl border border-border bg-card opacity-60"
            aria-hidden="true"
          ></div>
        {/if}

        {#key current.public_slug}
          <div
            role="group"
            aria-label={`Job: ${current.title} at ${current.company || 'unknown company'}`}
            class="swipe-card absolute inset-0 flex flex-col overflow-hidden rounded-3xl border border-border bg-card shadow-xl duration-300 ease-out"
            class:transition={animate}
            style={`transform: translate3d(${dragX}px, 0, 0) rotate(${rotation}deg); opacity: ${cardOpacity};`}
            onpointerdown={onPointerDown}
            onpointermove={onPointerMove}
            onpointerup={onPointerUp}
            onpointercancel={onPointerUp}
          >
            <!-- Swipe-direction stamps (Tinder-style), fading in with drag distance.
                 They anchor to the non-scrolling frame so they stay put as the content
                 below scrolls. -->
            {#if dragX > 20}
              <span
                class="pointer-events-none absolute left-5 top-5 z-10 rotate-[-12deg] rounded-lg border-4 border-brand px-3 py-1 text-2xl font-extrabold uppercase tracking-wider text-brand"
                style={`opacity: ${stampOpacity}`}
              >
                Save
              </span>
            {:else if dragX < -20}
              <span
                class="pointer-events-none absolute right-5 top-5 z-10 rotate-[12deg] rounded-lg border-4 border-rose-500 px-3 py-1 text-2xl font-extrabold uppercase tracking-wider text-rose-500"
                style={`opacity: ${stampOpacity}`}
              >
                Skip
              </span>
            {/if}

            <!-- Scroll container holds the whole card. touch-action: pan-y lets the
                 browser own vertical scrolling while onPointerMove's axis-lock keeps
                 horizontal drags as swipes; overscroll-contain stops the scroll from
                 chaining to the page behind the overlay. -->
            <div
              class="swipe-scroll flex min-h-0 flex-1 touch-pan-y flex-col gap-3 overflow-y-auto overscroll-contain p-5"
            >
              <div class="flex items-center gap-3">
                <CompanyLogo name={current.company} />
                <div class="min-w-0">
                  <div class="truncate font-semibold">{current.company || 'Unknown company'}</div>
                  {#if current.location}
                    <div class="truncate text-sm text-muted-foreground">{current.location}</div>
                  {/if}
                </div>
              </div>

              <h2 class="line-clamp-3 text-2xl font-bold tracking-tight">{current.title}</h2>

              {#if salary}
                <div class="text-lg font-bold tracking-tight text-brand-strong">
                  {salary}
                </div>
              {/if}

              <div class="flex flex-wrap gap-1.5">
                {#each tags as tag (tag)}
                  <Badge variant="secondary">{tag}</Badge>
                {/each}
                {#each skills as skill (skill)}
                  <Badge variant="secondary">{skill}</Badge>
                {/each}
              </div>

              {#if summary}
                <!-- Model-written synopsis: the card's lead. Short (≤400 chars), plain text. -->
                <p class="text-sm leading-relaxed text-foreground">{summary}</p>
              {/if}

              <!-- Personal profile-match block (reuses the detail-page component). Keyed
                   by slug via the {#key} above, so a fresh instance fetches per card. -->
              <JobMatch job={current} />

              {#if current.description}
                <!-- Description is server-sanitized HTML (see internal/sources), safe to
                     render. Now scrolls in full with the rest of the card. -->
                <div class="job-teaser text-sm leading-relaxed text-muted-foreground">
                  <!-- eslint-disable-next-line svelte/no-at-html-tags -- server-sanitized; the rule flags every {@html} regardless -->
                  {@html current.description}
                </div>
              {/if}

              <!-- Opens in a new tab so the deck (and your place in it) is preserved —
                   return by closing/switching back to this tab. -->
              <a
                href={resolve('/jobs/[slug]', { slug: current.public_slug })}
                target="_blank"
                rel="noopener"
                class="pt-1 text-left text-sm font-medium text-foreground underline-offset-4 hover:underline"
              >
                Open full details →
              </a>
            </div>
          </div>
        {/key}
      </div>
    </main>

    <!-- Action buttons (a tap alternative to swiping, and the desktop path). -->
    <footer
      class="flex shrink-0 items-center justify-center gap-6 px-4 pb-[max(1rem,env(safe-area-inset-bottom))] pt-2"
    >
      <button
        type="button"
        aria-label="Skip (dismiss)"
        class="flex h-16 w-16 items-center justify-center rounded-full border border-border bg-card text-rose-500 shadow-md transition hover:bg-accent active:scale-95"
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

    <p
      class="hidden shrink-0 pb-3 text-center text-xs text-muted-foreground md:block"
    >
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

<style>
  /* Grow-in intro for each incoming card. Uses the independent `scale` property
     (not transform: scale) so it composes with the drag/fly-off translate+rotate
     on `transform` instead of fighting it. Re-triggers per card via {#key slug}. */
  .swipe-card {
    animation: card-in 300ms ease-out;
    /* Force a dedicated GPU compositing layer so per-frame drag updates only
       recomposite (no main-thread repaint) — kills the mobile drag jitter. The
       tilt pivots around the card's bottom for a natural Tinder-like arc. */
    will-change: transform;
    transform-origin: center bottom;
    backface-visibility: hidden;
  }

  @keyframes card-in {
    from {
      scale: 0.95;
      opacity: 0.6;
    }
    to {
      scale: 1;
      opacity: 1;
    }
  }

  /* Scraped HTML: long URLs or nbsp-glued words must wrap, not force a scroll. */
  .job-teaser {
    overflow-wrap: break-word;
  }

  /* Flush the teaser's top with the card's gap — drop the first block's margin. */
  .job-teaser > :global(:first-child) {
    margin-top: 0;
  }

  /* Headings would be oversized in a teaser — render them as plain emphasis. */
  .job-teaser :global(h1),
  .job-teaser :global(h2),
  .job-teaser :global(h3),
  .job-teaser :global(h4) {
    margin: 0.5rem 0 0.25rem;
    font-weight: 600;
  }

  .job-teaser :global(p) {
    margin: 0.5rem 0;
  }

  .job-teaser :global(ul),
  .job-teaser :global(ol) {
    margin: 0.5rem 0;
    padding-left: 1.25rem;
  }

  .job-teaser :global(li) {
    display: list-item;
    list-style: disc outside;
    margin: 0.25rem 0;
  }

  /* Some boards wrap each <li> in a <p>; collapse it so the bullet sits inline. */
  .job-teaser :global(li) > :global(p) {
    margin: 0;
  }

  .job-teaser :global(b),
  .job-teaser :global(strong) {
    font-weight: 600;
  }
</style>
