<script lang="ts">
  import { onMount } from 'svelte';
  import { Heart, RotateCcw, X } from '@lucide/svelte';
  import { api } from '#lib/api.js';
  import { isPlanRefusal } from '#lib/api-core.js';
  import type { FeedCard } from '#lib/types.js';
  import { focusTrap } from '#lib/actions/focusTrap.js';
  import SwipeCard from './SwipeCard.svelte';
  import SwipeCardBody from './SwipeCardBody.svelte';
  import { FLY_MS, SwipeGesture } from './swipeGesture.svelte.js';
  import MatchBlock from './MatchBlock.svelte';
  import MatchesUpgrade from './MatchesUpgrade.svelte';
  import States from './States.svelte';

  // The match feed one card at a time, full screen like the swipe deck. Skip and Save write
  // the same tracking records; the server excludes them, so an empty page just refetches.
  let { onClose }: { onClose: () => void } = $props();

  const PAGE = 20;
  let cards = $state<FeedCard[]>([]);
  let loading = $state(true);
  let failed = $state(false);
  // The plan lapsed while swipe was open: the API refuses with 402.
  let paywalled = $state(false);
  let last = $state<{ card: FeedCard; action: 'save' | 'dismiss' } | null>(null);
  const current = $derived(cards[0]);
  const gesture = new SwipeGesture();

  async function load() {
    loading = true;
    failed = false;
    try {
      cards = (await api.meFeed(PAGE)).data;
    } catch (e) {
      if (isPlanRefusal(e)) paywalled = true;
      else failed = true;
    } finally {
      loading = false;
    }
  }

  // Fling the card off, drop it after the fly-off and write optimistically. A failed write
  // puts the card back so nothing is lost silently.
  async function judge(action: 'save' | 'dismiss') {
    const card = current;
    if (!card || gesture.exiting) return;
    gesture.fling(action);
    last = { card, action };
    const write = action === 'save' ? api.saveJob(card.public_slug) : api.dismissJob(card.public_slug);
    // Settled either way so a rejection is handled below, not as an unhandled one here.
    const written = write.then(
      () => true,
      () => false,
    );
    await new Promise((r) => setTimeout(r, FLY_MS));
    cards = cards.filter((c) => c !== card);
    gesture.settle();
    if (!(await written)) {
      // Restore only if not since undone (undo clears `last`), or the card would be queued twice.
      if (last?.card === card) {
        cards = [card, ...cards];
        last = null;
      }
      return;
    }
    if (cards.length === 0) await load();
  }

  async function undo() {
    if (!last || gesture.exiting) return;
    const { card, action } = last;
    last = null;
    await (action === 'save' ? api.unsaveJob(card.public_slug) : api.undismissJob(card.public_slug));
    cards = [card, ...cards];
  }

  // ← skip, → save, U undo, Esc back to the feed; never with a modifier.
  function onKey(e: KeyboardEvent) {
    if (e.ctrlKey || e.metaKey || e.altKey) return;
    if (e.key === 'ArrowLeft') judge('dismiss');
    else if (e.key === 'ArrowRight') judge('save');
    else if (e.key.toLowerCase() === 'u') undo();
    else if (e.key === 'Escape') onClose();
  }

  onMount(load);
</script>

<svelte:window onkeydown={onKey} />

<div
  class="fixed inset-0 z-50 flex flex-col bg-background text-foreground"
  role="dialog"
  aria-modal="true"
  aria-labelledby="match-swipe-title"
  {@attach focusTrap()}
>
  <header class="flex shrink-0 items-center justify-between gap-3 px-4 pb-2 pt-safe">
    <h1 id="match-swipe-title" class="text-sm font-medium text-muted-foreground">Matches · by my profile</h1>
    <button
      type="button"
      aria-label="Back to the feed"
      class="flex h-11 w-11 items-center justify-center rounded-full border border-border bg-card transition hover:bg-accent"
      onclick={onClose}
    >
      <X class="size-5" />
    </button>
  </header>

  {#if current}
    {#key current.public_slug}
      <SwipeCard
        {gesture}
        label={`Job: ${current.title} at ${current.company || 'unknown company'}`}
        hasNext={cards.length > 1}
        onCommit={judge}
      >
        <SwipeCardBody job={current}>
          {#snippet match()}
            <MatchBlock match={current.match} />
          {/snippet}
        </SwipeCardBody>
      </SwipeCard>
    {/key}

    <footer class="flex shrink-0 items-center justify-center gap-6 px-4 pb-safe pt-2">
      <button
        type="button"
        aria-label="Skip"
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
  {:else if paywalled}
    <div class="flex flex-1 items-center justify-center"><MatchesUpgrade price={null} /></div>
  {:else if loading}
    <div class="flex flex-1 items-center justify-center"><States state="loading" /></div>
  {:else if failed}
    <div class="flex flex-1 items-center justify-center">
      <States state="error" message="Failed to load your matches." />
    </div>
  {:else}
    <div class="flex flex-1 flex-col items-center justify-center gap-3 px-6 text-center">
      <p class="text-lg font-semibold">You're all caught up</p>
      <button
        type="button"
        class="rounded-lg border border-border bg-card px-4 py-2 text-sm font-medium hover:bg-accent"
        onclick={onClose}
      >
        Back to the feed
      </button>
    </div>
  {/if}
</div>
