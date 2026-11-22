<script lang="ts">
  import { onMount } from 'svelte';
  import { Heart, RotateCcw, X } from '@lucide/svelte';
  import { api } from '#lib/api.js';
  import { ApiError, isPlanRefusal } from '#lib/api-core.js';
  import type { FeedCard } from '#lib/types.js';
  import { focusTrap } from '#lib/actions/focusTrap.js';
  import { announce } from '#lib/announce.svelte.js';
  import FeedToast from './FeedToast.svelte';
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
  type Outcome = 'ok' | 'gone' | 'failed';
  // The last judgment and its write: Undo waits for the write so its DELETE cannot overtake it.
  let last = $state<{ card: FeedCard; action: 'save' | 'dismiss'; written: Promise<Outcome> } | null>(null);
  // An Undo in flight: a new judgment now could land before its inverse and be erased by it.
  let undoing = $state(false);
  const current = $derived(cards[0]);
  const gesture = new SwipeGesture();

  async function load() {
    loading = true;
    failed = false;
    try {
      const data = (await api.meFeed(PAGE)).data;
      // Append, not replace: an Undo during the request may have put a card back.
      const have = new Set(cards.map((c) => c.public_slug));
      cards = [...cards, ...data.filter((c) => !have.has(c.public_slug))];
    } catch (e) {
      if (isPlanRefusal(e)) paywalled = true;
      else failed = true;
    } finally {
      loading = false;
    }
  }

  const toastCopy = {
    save: "Didn't save — try again",
    dismiss: "Didn't skip — try again",
    undo: "Didn't undo — try again",
  };
  let toast = $state<{ message: string; id: number } | null>(null);
  function fail(message: string) {
    toast = { message, id: (toast?.id ?? 0) + 1 };
    announce(message, 'assertive');
  }
  const describe = (c: FeedCard) => `${c.title} at ${c.company || 'unknown company'}`;

  // Fling the card off, drop it after the fly-off and write optimistically. A failed write
  // puts the card back so nothing is lost silently; a 404 (job gone) leaves it dropped.
  async function judge(action: 'save' | 'dismiss') {
    const card = current;
    if (!card || gesture.exiting || undoing) return;
    gesture.fling(action);
    const write = action === 'save' ? api.saveJob(card.public_slug) : api.dismissJob(card.public_slug);
    // Settled either way so a rejection is handled below, not as an unhandled one here.
    const written: Promise<Outcome> = write.then(
      () => 'ok',
      (e) => (e instanceof ApiError && e.status === 404 ? 'gone' : 'failed'),
    );
    last = { card, action, written };
    await new Promise((r) => setTimeout(r, FLY_MS));
    cards = cards.filter((c) => c !== card);
    gesture.settle();
    const outcome = await written;
    if (outcome === 'failed') {
      // Restore unless undo already put it back, or the card would be queued twice.
      if (!cards.includes(card)) cards = [card, ...cards];
      if (last?.card === card) last = null;
      fail(toastCopy[action]);
      return;
    }
    if (outcome === 'gone') {
      if (last?.card === card) last = null; // undo must not bring a gone job back
      if (cards.length === 0) await load();
      return;
    }
    const verb = action === 'save' ? 'Saved' : 'Skipped';
    announce(cards[0] ? `${verb}. Next: ${describe(cards[0])}` : `${verb}.`);
    if (cards.length === 0) await load();
  }

  // The card returns at once; if the write fails it leaves again and Undo stays available.
  async function undo() {
    if (!last || gesture.exiting) return;
    const { card, action, written } = last;
    last = null;
    cards = [card, ...cards];
    undoing = true;
    try {
      // judge() handles a failed or gone write first (it awaited earlier): the card stays back
      // with its toast, or leaves for good.
      const outcome = await written;
      if (outcome === 'gone') {
        cards = cards.filter((c) => c !== card);
        if (cards.length === 0) await load();
      }
      if (outcome !== 'ok') return;
      await (action === 'save' ? api.unsaveJob(card.public_slug) : api.undismissJob(card.public_slug));
    } catch {
      cards = cards.filter((c) => c !== card);
      last = { card, action, written };
      fail(toastCopy.undo);
      return;
    } finally {
      undoing = false;
    }
    announce(`Undone. ${card.title} is back.`);
  }

  // ← skip, → save, U undo, Esc back to the feed. Bound to the overlay root, so it only
  // fires with focus inside it (or on body, when nothing is focused); fields, menus and
  // nested dialogs keep their own keys.
  function onKey(e: KeyboardEvent) {
    if (e.ctrlKey || e.metaKey || e.altKey || e.repeat || e.defaultPrevented) return;
    const t = e.target as HTMLElement | null;
    if (t?.closest('input,textarea,select,[contenteditable],[role=menu],[role^=menuitem]')) return;
    const dialog = t?.closest('[role=dialog]');
    if (dialog && dialog !== e.currentTarget) return;
    if (e.key === 'ArrowLeft') judge('dismiss');
    else if (e.key === 'ArrowRight') judge('save');
    else if (e.key.toLowerCase() === 'u') undo();
    else if (e.key === 'Escape') onClose();
    else return;
    if (e.key.startsWith('Arrow')) e.preventDefault();
  }

  onMount(load);
</script>

<svelte:window onkeydown={(e) => e.target === document.body && onKey(e)} />

<div
  class="fixed inset-0 z-50 flex flex-col bg-background text-foreground"
  role="dialog"
  aria-modal="true"
  aria-labelledby="match-swipe-title"
  tabindex="-1"
  onkeydown={onKey}
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
        aria-keyshortcuts="ArrowLeft"
        class="flex h-16 w-16 items-center justify-center rounded-full border border-border bg-card text-destructive shadow-md transition hover:bg-accent active:scale-95"
        onclick={() => judge('dismiss')}
      >
        <X class="size-7" />
      </button>
      <button
        type="button"
        aria-label="Undo last"
        aria-keyshortcuts="U"
        class="flex h-12 w-12 items-center justify-center rounded-full border border-border bg-card text-muted-foreground shadow-md transition hover:bg-accent active:scale-95 disabled:opacity-40"
        disabled={!last}
        onclick={undo}
      >
        <RotateCcw class="size-5" />
      </button>
      <button
        type="button"
        aria-label="Save"
        aria-keyshortcuts="ArrowRight"
        class="flex h-16 w-16 items-center justify-center rounded-full border border-border bg-card text-brand shadow-md transition hover:bg-accent active:scale-95"
        onclick={() => judge('save')}
      >
        <Heart class="size-7" />
      </button>
    </footer>

    <p class="hidden shrink-0 pb-3 text-center text-xs text-muted-foreground md:block">
      <kbd class="rounded border border-border px-1">←</kbd> skip ·
      <kbd class="rounded border border-border px-1">→</kbd> save ·
      <kbd class="rounded border border-border px-1">U</kbd> undo ·
      <kbd class="rounded border border-border px-1">Esc</kbd> close
    </p>
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
      {#if last}
        <button
          type="button"
          class="rounded-lg border border-border bg-card px-4 py-2 text-sm font-medium hover:bg-accent"
          onclick={undo}
        >
          Undo last
        </button>
      {/if}
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

{#if toast}
  {#key toast.id}
    <FeedToast class="mb-28" message={toast.message} onClose={() => (toast = null)} />
  {/key}
{/if}
