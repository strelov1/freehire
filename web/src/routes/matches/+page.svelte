<script lang="ts">
  import { onMount } from 'svelte';
  import { resolve } from '$app/paths';
  import { Heart, RotateCcw, X } from '@lucide/svelte';
  import { api } from '#lib/api.js';
  import type { FeedCard, FeedMeta } from '#lib/types.js';
  import SwipeCardBody from '#lib/components/SwipeCardBody.svelte';
  import States from '#lib/components/States.svelte';

  // The match feed: one card at a time from GET /me/feed. Skip and Save write the same
  // tracking records as the swipe deck; the server excludes them, so when the page runs
  // out the next fetch simply returns the next best matches.
  const PAGE = 20;
  const UNKNOWN_LABEL: Record<string, string> = {
    salary: 'salary not stated',
    work_mode: 'remote or office not stated',
    seniority: 'level not stated',
  };

  let cards = $state<FeedCard[]>([]);
  let meta = $state<FeedMeta | null>(null);
  let loading = $state(true);
  let failed = $state(false);
  let last = $state<{ card: FeedCard; action: 'save' | 'dismiss' } | null>(null);
  const current = $derived(cards[0]);

  async function load() {
    loading = true;
    failed = false;
    try {
      const res = await api.meFeed(PAGE);
      cards = res.data;
      meta = res.meta;
    } catch {
      failed = true;
    } finally {
      loading = false;
    }
  }

  async function judge(action: 'save' | 'dismiss') {
    const card = current;
    if (!card) return;
    cards = cards.slice(1);
    last = { card, action };
    try {
      await (action === 'save' ? api.saveJob(card.public_slug) : api.dismissJob(card.public_slug));
    } catch {
      // The write failed: put the card back so nothing is lost silently.
      cards = [card, ...cards];
      last = null;
      return;
    }
    if (cards.length === 0) await load();
  }

  async function undo() {
    if (!last) return;
    const { card, action } = last;
    last = null;
    await (action === 'save' ? api.unsaveJob(card.public_slug) : api.undismissJob(card.public_slug));
    cards = [card, ...cards];
  }

  // ← skip, → save, U undo; never with a modifier, never while typing.
  function onKey(e: KeyboardEvent) {
    if (e.ctrlKey || e.metaKey || e.altKey || (e.target as HTMLElement).closest('input, textarea')) return;
    if (e.key === 'ArrowLeft') judge('dismiss');
    else if (e.key === 'ArrowRight') judge('save');
    else if (e.key.toLowerCase() === 'u') undo();
  }

  onMount(load);
</script>

<svelte:head>
  <title>Matches — freehire</title>
  <meta name="robots" content="noindex" />
</svelte:head>

<svelte:window onkeydown={onKey} />

<div class="mx-auto flex w-full max-w-2xl flex-col gap-4 px-4 py-6">
  <header class="flex items-baseline justify-between gap-3">
    <h1 class="text-2xl font-semibold">Matches</h1>
    <span class="rounded-full border border-border px-3 py-1 text-sm text-muted-foreground">By my profile</span>
  </header>

  {#if meta?.degraded === 'no_match_vector'}
    <p class="rounded-lg border border-border bg-card px-4 py-3 text-sm text-muted-foreground">
      Your profile has no skills, so these are the newest jobs that fit your filters, not ranked by match.
      <a class="underline" href={resolve('/my/profile')}>Add skills</a>
    </p>
  {/if}

  {#if current}
    <article
      aria-label={`Job: ${current.title} at ${current.company || 'unknown company'}`}
      class="flex flex-col overflow-hidden rounded-3xl border border-border bg-card shadow-xl"
    >
      <dl class="grid gap-1 border-b border-border px-5 pb-3 pt-4 text-sm">
        {#if current.match.why.length}
          <div>
            <dt class="inline font-medium text-brand">Why it fits ·</dt>
            <dd class="inline">{current.match.why.join(', ')}</dd>
          </div>
        {/if}
        {#if current.match.may_not_fit.length}
          <div>
            <dt class="inline font-medium text-amber-600 dark:text-amber-400">May not fit ·</dt>
            <dd class="inline">{current.match.may_not_fit.join(', ')} not on your profile</dd>
          </div>
        {/if}
        {#if current.match.unknown.length}
          <div>
            <dt class="inline font-medium text-muted-foreground">Unknown ·</dt>
            <dd class="inline">{current.match.unknown.map((u) => UNKNOWN_LABEL[u] ?? u).join(', ')}</dd>
          </div>
        {/if}
      </dl>
      <div class="flex max-h-[42dvh] flex-col md:max-h-[60vh]">
        <SwipeCardBody job={current} />
      </div>
    </article>

    <div class="flex items-center justify-center gap-6">
      <button
        type="button"
        aria-label="Skip"
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
    </div>
    <p class="hidden text-center text-xs text-muted-foreground md:block">
      <kbd class="rounded border border-border px-1">←</kbd> skip ·
      <kbd class="rounded border border-border px-1">→</kbd> save ·
      <kbd class="rounded border border-border px-1">U</kbd> undo
      {#if meta}· {meta.total} matches{/if}
    </p>
  {:else if loading}
    <States state="loading" />
  {:else if failed}
    <States state="error" message="Failed to load your matches." />
  {:else if meta?.reason}
    <div class="flex flex-col items-center gap-3 py-16 text-center">
      <p class="text-lg font-semibold">Tell us what you are looking for</p>
      <p class="text-sm text-muted-foreground">
        Matches are built from your profile: skills, level and where you want to work.
      </p>
      <a
        class="rounded-lg border border-border bg-card px-4 py-2 text-sm font-medium hover:bg-accent"
        href={resolve('/my/profile')}>Fill in my profile</a
      >
    </div>
  {:else}
    <div class="flex flex-col items-center gap-3 py-16 text-center">
      <p class="text-lg font-semibold">You're all caught up</p>
      <p class="text-sm text-muted-foreground">No more jobs match your profile right now.</p>
      <a
        class="rounded-lg border border-border bg-card px-4 py-2 text-sm font-medium hover:bg-accent"
        href={resolve('/my/activity')}>See saved jobs</a
      >
    </div>
  {/if}
</div>
