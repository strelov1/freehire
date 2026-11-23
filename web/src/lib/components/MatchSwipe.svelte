<script lang="ts">
  import { onMount, tick } from 'svelte';
  import { ExternalLink, Heart, MoreHorizontal, RotateCcw, X } from '@lucide/svelte';
  import { resolve } from '$app/paths';
  import { api } from '#lib/api.js';
  import { ApiError, isPlanRefusal } from '#lib/api-core.js';
  import type { FeedCard, FeedMeta } from '#lib/types.js';
  import { feedSource } from '#lib/feedSource.svelte.js';
  import { Badge } from '#lib/ui/index.js';
  import { focusTrap } from '#lib/actions/focusTrap.js';
  import { menuNav } from '#lib/actions/menuNav.js';
  import { track } from '#lib/analytics.js';
  import { announce } from '#lib/announce.svelte.js';
  import FeedToast from './FeedToast.svelte';
  import SwipeCard from './SwipeCard.svelte';
  import SwipeCardBody from './SwipeCardBody.svelte';
  import { FLY_MS, SwipeGesture } from './swipeGesture.svelte.js';
  import FeedNewEnd from './FeedNewEnd.svelte';
  import FeedSourceEmpty from './FeedSourceEmpty.svelte';
  import FeedSourceMenu from './FeedSourceMenu.svelte';
  import HideCompanyConfirm from './HideCompanyConfirm.svelte';
  import MatchBlock from './MatchBlock.svelte';
  import MatchesUpgrade from './MatchesUpgrade.svelte';
  import States from './States.svelte';

  // The match feed one card at a time, full screen like the swipe deck. Skip and Save write
  // the same tracking records; the server excludes them, so an empty page just refetches.
  let { onClose, onCompanyHidden }: { onClose: () => void; onCompanyHidden?: () => void } = $props();

  const PAGE = 20;
  // The source revision this instance was mounted for: a response that lands after a pick
  // belongs to the old source and must not touch the shared source state.
  const rev = feedSource.version;
  const stale = () => rev !== feedSource.version;
  let sourceRevision = 0;
  const sourceChanged = (revision: number) => stale() || revision !== sourceRevision;
  let overlay = $state<HTMLDivElement | null>(null);
  let cards = $state<FeedCard[]>([]);
  let loading = $state(true);
  let failed = $state(false);
  let meta = $state<FeedMeta | null>(null);
  // The session's first page had no card: a saved search that matches nothing, not a spent feed.
  let firstEmpty = $state(false);
  // The plan lapsed while swipe was open: the API refuses with 402.
  let paywalled = $state(false);
  type Outcome = 'ok' | 'gone' | 'failed';
  // The last judgment and its write: Undo waits for the write so its DELETE cannot overtake it.
  // `ref.id` is the application an apply wrote, filled when its write lands, so Undo can delete just it.
  type Action = 'save' | 'dismiss' | 'apply';
  type Last = { card: FeedCard; action: Action; written: Promise<Outcome>; ref: { id?: string } };
  let last = $state<Last | null>(null);
  // An apply card is on screen for FLY_MS without the swipe sweep; no second judgment in that window.
  let applying = $state(false);
  let menuOpen = $state(false);
  let menuRoot = $state<HTMLElement | null>(null);
  let menuTrigger = $state<HTMLButtonElement | null>(null);
  // The company being hidden, while its confirmation is open.
  let hideTarget = $state<{ slug: string; name: string } | null>(null);
  // Numbers every load: a reply that lands after a hide-company reload belongs to the old list.
  let loadEpoch = 0;
  // An Undo in flight: a new judgment now could land before its inverse and be erased by it.
  let undoing = $state(false);
  const current = $derived(cards[0]);
  // New cards come first; between the last one and the first rest card there is a pause screen.
  // The server's seen_cursor is posted once, when the candidate gets past the last new card.
  let hadNew = $state(false);
  let hadLoaded = false;
  let restShown = $state(false);
  let seenCursor = $state<string | undefined>();
  const paused = $derived(!!current && !current.is_new && hadNew && !restShown);
  $effect(() => {
    if (seenCursor && !loading && (paused || (!current && hadNew))) {
      const c = seenCursor;
      seenCursor = undefined;
      api.markFeedSeen(c).catch(() => {}); // the watermark only decides what is new next time
    }
  });
  // A judgment now would race the fly-off, the apply dwell or an Undo's write.
  const busy = () => gesture.exiting || undoing || applying;
  const applyClass =
    'flex w-full max-w-md items-center justify-center gap-2 rounded-full bg-brand px-6 py-3 text-sm font-semibold text-brand-foreground shadow-md transition hover:opacity-90';
  const gesture = new SwipeGesture();

  async function load() {
    const epoch = ++loadEpoch;
    loading = true;
    failed = false;
    try {
      const res = await api.meFeed(PAGE);
      if (stale() || epoch !== loadEpoch) return;
      const data = res.data;
      if (res.meta) {
        if (meta && (meta.source !== res.meta.source || meta.saved_search_id !== res.meta.saved_search_id)) {
          sourceRevision++;
          cards = [];
          last = null;
          toast = null;
          hadLoaded = false;
          hadNew = false;
          restShown = false;
          seenCursor = undefined;
        }
        meta = res.meta;
        feedSource.fromMeta(res.meta);
        if (!hadLoaded) firstEmpty = data.length === 0 && !res.meta.next_cursor;
      }
      hadLoaded = true;
      if (data.some((c) => c.is_new)) hadNew = true;
      seenCursor = res.meta?.seen_cursor ?? seenCursor;
      // Append, not replace: an Undo during the request may have put a card back.
      const have = new Set(cards.map((c) => c.public_slug));
      cards = [...cards, ...data.filter((c) => !have.has(c.public_slug))];
    } catch (e) {
      if (stale() || epoch !== loadEpoch) return;
      if (isPlanRefusal(e)) paywalled = true;
      else failed = true;
    } finally {
      if (!stale() && epoch === loadEpoch) {
        loading = false;
        if (feedSource.takeCardFocus(rev, 'swipe')) {
          await tick();
          const heading = current && overlay?.querySelector('h2');
          if (heading) {
            heading.tabIndex = -1;
            heading.focus();
          }
        }
      }
    }
  }

  const toastCopy = {
    save: "Didn't save — try again",
    dismiss: "Didn't skip — try again",
    undo: "Didn't undo — try again",
    apply: "Didn't mark as applied — try again",
  };
  let toast = $state<{ message: string; id: number; action?: { label: string; onClick: () => void } } | null>(null);
  function showToast(message: string, action?: { label: string; onClick: () => void }) {
    toast = { message, id: (toast?.id ?? 0) + 1, action };
  }
  // A toast's Undo belongs to the judgment it announced; any newer one makes it stale.
  const dropToastAction = () => {
    if (toast?.action) toast = { ...toast, action: undefined };
  };
  function fail(message: string) {
    showToast(message);
    announce(message, 'assertive');
  }
  const describe = (c: FeedCard) => `${c.title} at ${c.company || 'unknown company'}${c.is_new ? ', new' : ''}`;

  // Fling the card off, drop it after the fly-off and write optimistically. A failed write
  // puts the card back so nothing is lost silently; a 404 (job gone) leaves it dropped.
  // Apply (and Already applied) leave the same way minus the sweep.
  async function judge(action: Action, opts: { openedTab?: boolean } = {}) {
    const card = current;
    if (!card || busy()) return;
    const revision = sourceRevision;
    dropToastAction();
    const ref: Last['ref'] = {};
    let write: Promise<unknown>;
    if (action === 'apply') {
      applying = true;
      write = api.markJobApplied(card.public_slug).then((r) => {
        ref.id = r.application_id;
      });
    } else {
      gesture.fling(action);
      write = action === 'save' ? api.saveJob(card.public_slug) : api.dismissJob(card.public_slug);
    }
    // Settled either way so a rejection is handled below, not as an unhandled one here.
    const written: Promise<Outcome> = write.then(
      () => 'ok',
      (e) => (e instanceof ApiError && e.status === 404 ? 'gone' : 'failed'),
    );
    last = { card, action, written, ref };
    await new Promise((r) => setTimeout(r, FLY_MS));
    cards = cards.filter((c) => c !== card);
    if (action === 'apply') applying = false;
    else gesture.settle();
    const outcome = await written;
    if (sourceChanged(revision)) return;
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
    // The application existed before this apply (no id): undoing it would delete history we did not write.
    if (action === 'apply' && !ref.id && last?.card === card) last = null;
    const next = cards[0] ? ` Next: ${describe(cards[0])}` : '';
    if (action === 'apply' && opts.openedTab) {
      const done = `Applied to ${card.title}, opened in a new tab.`;
      showToast(
        done,
        ref.id && last?.card === card ? { label: 'Undo', onClick: () => last?.card === card && undo() } : undefined,
      );
      announce(done + next);
    } else if (action === 'apply') {
      announce(`Marked as applied.${next}`);
    } else {
      announce(`${action === 'save' ? 'Saved' : 'Skipped'}.${next}`);
    }
    if (cards.length === 0) await load();
  }

  const askHideCompany = () => {
    menuOpen = false;
    if (current?.company_slug && !busy()) hideTarget = { slug: current.company_slug, name: current.company };
  };
  // Cancelled: back to the menu button that opened it. Hidden: the cards, the Undo and any
  // reply still in flight belong to the old exclusion list, so the feed starts over.
  async function onHideClosed(result: 'hidden' | 'cancelled') {
    hideTarget = null;
    await tick();
    if (result === 'cancelled') {
      menuTrigger?.focus();
      return;
    }
    onCompanyHidden?.();
    sourceRevision++;
    cards = [];
    last = null;
    toast = null;
    hadLoaded = false;
    hadNew = false;
    restShown = false;
    seenCursor = undefined;
    await load();
    await tick();
    const heading = current && overlay?.querySelector('h2');
    const target = heading || overlay;
    if (target) {
      target.tabIndex = -1;
      target.focus();
    }
  }

  const markApplied = () => {
    menuOpen = false;
    return judge('apply');
  };
  // The link opens the employer's tab by itself; this records the intent and the application.
  function onApplyClick(e: MouseEvent) {
    // judge() would bail while busy: keep the tab from opening with nothing recorded.
    if (!current || busy()) {
      e.preventDefault();
      return;
    }
    track('job_apply', { slug: current.public_slug, source: current.source });
    judge('apply', { openedTab: true });
  }

  // The card returns at once; if the write fails it leaves again and Undo stays available.
  async function undo() {
    // An apply card is still in `cards` for its FLY_MS; restoring it now would duplicate it.
    if (!last || gesture.exiting || applying) return;
    const revision = sourceRevision;
    const { card, action, written, ref } = last;
    dropToastAction();
    last = null;
    cards = [card, ...cards];
    undoing = true;
    try {
      // judge() handles a failed or gone write first (it awaited earlier): the card stays back
      // with its toast, or leaves for good.
      const outcome = await written;
      if (sourceChanged(revision)) return;
      if (outcome === 'gone') {
        cards = cards.filter((c) => c !== card);
        if (cards.length === 0) await load();
      }
      if (outcome !== 'ok') return;
      if (action === 'apply') {
        if (!ref.id) {
          // Nothing of ours to delete: the card leaves again, like a gone job.
          cards = cards.filter((c) => c !== card);
          if (cards.length === 0) await load();
          return;
        }
        await api.untrackApplication(ref.id);
      } else await (action === 'save' ? api.unsaveJob(card.public_slug) : api.undismissJob(card.public_slug));
    } catch {
      if (sourceChanged(revision)) return;
      cards = cards.filter((c) => c !== card);
      last = { card, action, written, ref };
      fail(toastCopy.undo);
      return;
    } finally {
      undoing = false;
    }
    if (!sourceChanged(revision)) announce(`Undone. ${card.title} is back.`);
  }

  // ← skip, → save, U undo, Esc back to the feed. Bound to the overlay root, so it only
  // fires with focus inside it (or on body, when nothing is focused); fields, menus and
  // nested dialogs keep their own keys.
  function onKey(e: KeyboardEvent) {
    if (e.ctrlKey || e.metaKey || e.altKey || e.repeat || e.defaultPrevented || menuOpen) return;
    const t = e.target as HTMLElement | null;
    if (t?.closest('input,textarea,select,[contenteditable],[role=menu],[role^=menuitem]')) return;
    if (e.key !== 'Escape' && t?.closest('[aria-haspopup=menu]')) return;
    const dialog = t?.closest('[role=dialog]');
    if (dialog && dialog !== e.currentTarget) return;
    if (paused && e.key !== 'Escape') return; // the first rest card is hidden behind the pause screen
    if (e.key === 'ArrowLeft') judge('dismiss');
    else if (e.key === 'ArrowRight') judge('save');
    else if (e.key.toLowerCase() === 'u') undo();
    else if (e.key === 'Escape') onClose();
    else return;
    if (e.key.startsWith('Arrow')) e.preventDefault();
  }

  onMount(load);
</script>

<svelte:window
  onkeydown={(e) => e.target === document.body && onKey(e)}
  onclick={(e) => menuOpen && !menuRoot?.contains(e.target as Node) && (menuOpen = false)}
/>

<div
  class="fixed inset-0 z-50 flex flex-col bg-background text-foreground"
  role="dialog"
  aria-modal="true"
  aria-labelledby="match-swipe-title"
  tabindex="-1"
  bind:this={overlay}
  onkeydown={onKey}
  {@attach focusTrap()}
>
  <header class="flex shrink-0 items-center justify-between gap-3 px-4 pb-2 pt-safe">
    <div class="flex min-w-0 items-center gap-3">
      <h1 id="match-swipe-title" class="text-sm font-medium text-muted-foreground">Matches</h1>
      <FeedSourceMenu />
    </div>
    <div class="ml-auto flex items-center gap-2">
      {#if current && !paused}
        <div class="relative" bind:this={menuRoot}>
          <button
            type="button"
            bind:this={menuTrigger}
            aria-label={`More actions for ${current.company || 'unknown company'}`}
            aria-haspopup="menu"
            aria-expanded={menuOpen}
            class="flex h-11 w-11 items-center justify-center rounded-full border border-border bg-card transition hover:bg-accent"
            onclick={() => (menuOpen = !menuOpen)}
          >
            <MoreHorizontal class="size-5" />
          </button>
          {#if menuOpen}
            <div
              role="menu"
              aria-label="More actions"
              tabindex="-1"
              class="absolute right-0 top-full z-10 mt-2 w-48 rounded-md border border-border bg-background py-1 shadow-lg"
              onkeydown={(e) => e.key === 'Escape' && ((menuOpen = false), e.stopPropagation())}
              {@attach menuNav({ onclose: () => (menuOpen = false) })}
            >
              <button
                type="button"
                role="menuitem"
                class="flex w-full px-3 py-2 text-left text-sm hover:bg-accent focus:bg-accent focus:outline-none"
                onclick={markApplied}
              >
                Already applied
              </button>
              {#if current.company_slug}
                <button
                  type="button"
                  role="menuitem"
                  class="flex min-h-11 w-full items-center px-3 py-2 text-left text-sm hover:bg-accent focus:bg-accent focus:outline-none"
                  onclick={askHideCompany}
                >
                  Hide company
                </button>
              {/if}
            </div>
          {/if}
        </div>
      {/if}
      <button
        type="button"
        aria-label="Back to the feed"
        class="flex h-11 w-11 items-center justify-center rounded-full border border-border bg-card transition hover:bg-accent"
        onclick={onClose}
      >
        <X class="size-5" />
      </button>
    </div>
  </header>

  {#if current && paused}
    <FeedNewEnd onContinue={() => (restShown = true)} />
  {:else if current}
    {#key current.public_slug}
      <SwipeCard
        {gesture}
        label={`Job: ${current.title} at ${current.company || 'unknown company'}`}
        hasNext={cards.length > 1}
        onCommit={judge}
      >
        <SwipeCardBody job={current}>
          {#snippet badge()}
            {#if current.is_new}<Badge variant="brand" class="ml-auto shrink-0">New</Badge>{/if}
          {/snippet}
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

    {#if !current.closed_at}
      <div class="flex shrink-0 justify-center px-4 pb-3">
        {#if current.direct_apply === true}
          <a
            href={resolve('/jobs/[slug]', { slug: current.public_slug })}
            target="_blank"
            rel="noopener noreferrer"
            onclick={() => track('job_apply', { slug: current.public_slug, source: current.source })}
            aria-label={`Apply to ${current.title} on freehire (opens in a new tab)`}
            class={applyClass}
          >
            Apply <ExternalLink class="size-4" />
          </a>
        {:else if current.url}
          <!-- eslint-disable svelte/no-navigation-without-resolve -- the employer's own site, not an internal route -->
          <a
            href={current.url}
            target="_blank"
            rel="nofollow noopener noreferrer"
            aria-label={`Apply to ${current.title} on the employer's site (opens in a new tab)`}
            class={applyClass}
            onclick={onApplyClick}
          >
            Apply <ExternalLink class="size-4" />
          </a><!-- eslint-enable svelte/no-navigation-without-resolve -->
        {/if}
      </div>
    {/if}

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
  {:else if meta?.reason === 'search_deleted'}
    <FeedSourceEmpty kind="deleted" mode="swipe" />
  {:else if meta?.source === 'saved_search' && meta.reason}
    <FeedSourceEmpty kind="unusable" mode="swipe" />
  {:else if meta?.source === 'saved_search' && firstEmpty}
    <FeedSourceEmpty kind="no_matches" mode="swipe" />
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
  {#if hideTarget}
    <HideCompanyConfirm slug={hideTarget.slug} name={hideTarget.name} onclose={onHideClosed} />
  {/if}
  {#if toast}
    {#key toast.id}
      <FeedToast class="mb-28" message={toast.message} action={toast.action} onClose={() => (toast = null)} />
    {/key}
  {/if}
</div>
