<script lang="ts">
  import { onMount } from 'svelte';
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import { api } from '$lib/api';
  import { cardTags, formatSalary } from '$lib/enrichment';
  import type { Job } from '$lib/types';
  import { Badge } from '$lib/ui';
  import CompanyLogo from './CompanyLogo.svelte';
  import States from './States.svelte';

  type Judgement = 'save' | 'dismiss';

  // Batch size and the "top up before it runs dry" threshold. COMMIT_PX is how
  // far a card must be dragged before the gesture counts as a decision.
  const LIMIT = 12;
  const PREFETCH_AT = 4;
  const COMMIT_PX = 100;
  // How long the judged card takes to fly off-screen before it's dropped.
  const FLY_MS = 300;

  // The deck honors the /jobs filters carried in the URL; unknown params (there
  // are none here beyond facets) are ignored by the endpoint. Captured once —
  // the filters are fixed for a swipe session.
  const facets = new URLSearchParams(page.url.search);

  // queue holds only UNJUDGED cards; the active one is queue[0]. seen dedups by
  // slug so a prefetch that races a just-sent save/dismiss can't re-add a card.
  let queue = $state<Job[]>([]);
  const seen = new Set<string>();
  // Single-step undo: the last judged card and how it was judged.
  let last = $state<{ job: Job; kind: Judgement } | null>(null);

  let loading = $state(false);
  let error = $state(false);
  let exhausted = $state(false);

  // Live drag offset of the active card (px); 0 when settled.
  let dragX = $state(0);
  let dragging = $state(false);
  let startX = 0;
  // The active card is mid fly-off: guards re-entry and drives the fade-out.
  let exiting = $state(false);

  // The card transitions (springs back or flies off) whenever it isn't tracking
  // the finger. The grow-in intro is a CSS keyframe on mount, not a transition —
  // see the `.swipe-card` animation below — so no frame-timing dance is needed.
  const animate = $derived(!dragging);
  // Tilt tracks the drag but is capped so the fly-off doesn't spin wildly; the
  // card fades out as it leaves.
  const rotation = $derived(Math.max(-18, Math.min(18, dragX / 24)));
  const cardOpacity = $derived(exiting ? 0 : 1);

  const current = $derived(queue[0] ?? null);
  const next = $derived(queue[1] ?? null);

  async function loadBatch() {
    if (loading || exhausted) return;
    loading = true;
    error = false;
    try {
      // offset = held-unjudged count: judged cards are already excluded
      // server-side, so this returns the page right after what we hold.
      const slice = await api.swipeDeck(facets, LIMIT, queue.length);
      const fresh = slice.items.filter((j) => !seen.has(j.public_slug));
      for (const j of fresh) seen.add(j.public_slug);
      queue = [...queue, ...fresh];
      // A page with no rows at all means the undecided set is drained.
      if (slice.items.length === 0) exhausted = true;
    } catch {
      error = true;
    } finally {
      loading = false;
    }
  }

  function prefetchIfLow() {
    if (queue.length <= PREFETCH_AT && !exhausted && !loading) void loadBatch();
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

  function openDetail() {
    if (!current) return;
    window.open(resolve('/jobs/[slug]', { slug: current.public_slug }), '_blank', 'noopener');
  }

  // --- Pointer drag on the active card (touch + mouse) ---------------------
  function onPointerDown(e: PointerEvent) {
    if (!current || exiting) return;
    // Let clicks on links/buttons inside the card through — capturing the pointer
    // for a drag would swallow their click (e.g. "Open full details").
    if ((e.target as HTMLElement).closest('a, button')) return;
    dragging = true;
    startX = e.clientX;
    (e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
  }
  function onPointerMove(e: PointerEvent) {
    if (dragging) dragX = e.clientX - startX;
  }
  function onPointerUp() {
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
    }
  }

  onMount(loadBatch);

  const salary = $derived(current?.enrichment ? formatSalary(current.enrichment) : null);
  const tags = $derived(current ? cardTags(current) : []);
  const skills = $derived(current?.skills?.slice(0, 6) ?? []);
</script>

<svelte:window onkeydown={onKeydown} />

<div class="flex flex-col items-center gap-4">
  <p class="text-sm text-muted-foreground">Swipe to triage — right to save, left to skip.</p>

  {#if current}
    <!-- Card stack: the next card peeks behind for depth. -->
    <div class="relative h-[26rem] w-full select-none">
      {#if next}
        <div
          class="absolute inset-0 scale-95 rounded-2xl border border-border bg-card opacity-60"
          aria-hidden="true"
        ></div>
      {/if}

      {#key current.public_slug}
        <div
          role="group"
          aria-label={`Job: ${current.title} at ${current.company || 'unknown company'}`}
          class="swipe-card absolute inset-0 flex touch-none flex-col gap-3 rounded-2xl border border-border bg-card p-5 shadow-lg duration-300 ease-out"
          class:transition={animate}
          style={`transform: translateX(${dragX}px) rotate(${rotation}deg); opacity: ${cardOpacity};`}
          onpointerdown={onPointerDown}
          onpointermove={onPointerMove}
          onpointerup={onPointerUp}
          onpointercancel={onPointerUp}
        >
        <!-- Swipe-direction cues. -->
        {#if dragX > 20}
          <span class="absolute right-4 top-4 rounded-md border-2 border-emerald-500 px-2 py-0.5 text-sm font-bold text-emerald-500">SAVE</span>
        {:else if dragX < -20}
          <span class="absolute right-4 top-4 rounded-md border-2 border-rose-500 px-2 py-0.5 text-sm font-bold text-rose-500">SKIP</span>
        {/if}

        <div class="flex items-center gap-3">
          <CompanyLogo name={current.company} />
          <div class="min-w-0">
            <div class="truncate font-semibold">{current.company || 'Unknown company'}</div>
            {#if current.location}
              <div class="truncate text-sm text-muted-foreground">{current.location}</div>
            {/if}
          </div>
        </div>

        <h2 class="line-clamp-2 text-xl font-semibold tracking-tight">{current.title}</h2>

        {#if salary}
          <div class="text-lg font-bold tracking-tight text-emerald-600 dark:text-emerald-400">{salary}</div>
        {/if}

        <div class="flex flex-wrap gap-1.5">
          {#each tags as tag (tag)}
            <Badge variant="secondary">{tag}</Badge>
          {/each}
          {#each skills as skill (skill)}
            <Badge variant="secondary">{skill}</Badge>
          {/each}
        </div>

        {#if current.description}
          <!-- Description is server-sanitized HTML (see internal/sources), safe to render.
               Fills the leftover space above the link and clips, with a fade cueing more. -->
          <div class="job-teaser relative min-h-0 flex-1 overflow-hidden text-sm leading-relaxed text-muted-foreground">
            <!-- eslint-disable-next-line svelte/no-at-html-tags -- server-sanitized; the rule flags every {@html} regardless -->
            {@html current.description}
            <div class="pointer-events-none absolute inset-x-0 bottom-0 h-10 bg-gradient-to-t from-card to-transparent"></div>
          </div>
        {/if}

        <!-- Opens in a new tab so the deck (and your place in it) is preserved —
             return by closing/switching back to this tab. -->
        <a
          href={resolve('/jobs/[slug]', { slug: current.public_slug })}
          target="_blank"
          rel="noopener"
          class="mt-auto text-left text-sm font-medium text-foreground underline-offset-4 hover:underline"
        >
          Open full details →
        </a>
      </div>
      {/key}
    </div>

    <!-- Action buttons (desktop + a tap alternative to swiping). -->
    <div class="flex items-center gap-5">
      <button
        type="button"
        aria-label="Skip (dismiss)"
        class="flex h-14 w-14 items-center justify-center rounded-full border border-border bg-card text-2xl text-rose-500 transition hover:bg-accent"
        onclick={() => judge('dismiss')}
      >
        ✗
      </button>
      <button
        type="button"
        aria-label="Undo last"
        class="flex h-10 w-10 items-center justify-center rounded-full border border-border bg-card text-base text-muted-foreground transition hover:bg-accent disabled:opacity-40"
        disabled={!last}
        onclick={undo}
      >
        ↩
      </button>
      <button
        type="button"
        aria-label="Save"
        class="flex h-14 w-14 items-center justify-center rounded-full border border-border bg-card text-2xl text-emerald-500 transition hover:bg-accent"
        onclick={() => judge('save')}
      >
        ♥
      </button>
    </div>

    <p class="text-xs text-muted-foreground">
      <kbd class="rounded border border-border px-1">←</kbd> skip ·
      <kbd class="rounded border border-border px-1">→</kbd> save ·
      <kbd class="rounded border border-border px-1">↑</kbd> open ·
      <kbd class="rounded border border-border px-1">U</kbd> undo
    </p>
  {:else if loading}
    <States state="loading" />
  {:else if error}
    <States state="error" message="Failed to load the deck." />
  {:else}
    <div class="flex flex-col items-center gap-3 py-16 text-center">
      <p class="text-lg font-semibold">You're all caught up</p>
      <p class="text-sm text-muted-foreground">No more jobs match these filters.</p>
      <a
        href={resolve('/jobs')}
        class="text-sm font-medium text-foreground underline underline-offset-4"
      >
        Back to all jobs →
      </a>
    </div>
  {/if}
</div>

<style>
  /* Grow-in intro for each incoming card. Uses the independent `scale` property
     (not transform: scale) so it composes with the drag/fly-off translate+rotate
     on `transform` instead of fighting it. Re-triggers per card via {#key slug}. */
  .swipe-card {
    animation: card-in 300ms ease-out;
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
