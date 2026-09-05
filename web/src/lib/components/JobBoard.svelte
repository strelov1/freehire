<script lang="ts">
  import { goto, pushState } from '$app/navigation';
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import { Search, X as XIcon } from '@lucide/svelte';
  import { api } from '$lib/api';
  import { isAuthenticated } from '$lib/auth.svelte';
  import { UrlSyncedState, syncOnNavigation } from '$lib/urlSynced.svelte';
  import type { MyJob } from '$lib/types';
  import {
    BOARD_COLUMNS,
    columnOf,
    matchesQuery,
    type BoardColumnId,
    type BoardItem,
    type ClosedOutcome,
  } from '$lib/board';
  import { createRehearsal, createDebrief } from '$lib/assistant/api';
  import { assertNever } from '$lib/utils';
  import BoardColumn from './BoardColumn.svelte';
  import BoardList from './BoardList.svelte';
  import JobDrawer from './JobDrawer.svelte';
  import FollowUpDialog from './FollowUpDialog.svelte';
  import States from './States.svelte';

  // initialId (from /my/tracking/[id]) opens that application's drawer once the
  // board has loaded, so a deep link / inbox link / refresh reopens the same card.
  // It is the row id the listing served, which for a posting-backed row is that
  // posting's slug — so the inbox's existing slug links keep resolving.
  // initial carries the board rows fetched in the route's server load, so the board
  // paints with the page (no client fetch on mount); a caller that omits it falls
  // back to a client fetch. Mutations still reload() client-side.
  // `view` picks the presentation. This component owns the rows, the mutations and the
  // application panel; the board and the list are two renderings of the same thing, and
  // keeping one owner is what stops them drifting apart.
  let {
    initialId,
    initial,
    view = 'board',
  }: { initialId?: string; initial?: MyJob[]; view?: 'board' | 'list' } = $props();
  let openedInitial = false;

  function emptyColumns(): Record<BoardColumnId, BoardItem[]> {
    return { preparing: [], applied: [], interview: [], offer: [], closed: [] };
  }

  // Per-column arrays are the source of truth once loaded; svelte-dnd-action
  // moves BoardItem objects between them. cardCol tracks each card's persisted
  // column so a finalize can tell which card actually moved into a zone.
  let columns = $state<Record<BoardColumnId, BoardItem[]>>(emptyColumns());
  let cardCol = $state<Record<string, BoardColumnId>>({});
  let status = $state<'loading' | 'ready' | 'error'>('loading');

  // Drawer state. A Closed drop opens the drawer requiring an outcome choice.
  let openItem = $state.raw<BoardItem | null>(null);
  let pendingOutcome = $state(false);
  // The follow-up dialog is its own overlay, not a drawer tab: it is reached from
  // the card's silence badge and closes back to the board.
  let followUpItem = $state.raw<BoardItem | null>(null);
  // Starting a rehearsal or a debrief is one round trip before the navigation, so a
  // second click in that window would mint a second conversation. One flag for both:
  // they end in the same navigation, and only one of them can be underway.
  let startingSession = $state(false);
  let sessionError = $state<string | null>(null);

  // Lay the fetched rows out into the columns and open the deep-linked drawer once.
  // The 'board' filter returns saved ∪ applied ∪ stage, and a saved-only row now lands
  // in Preparing (see columnOf) rather than being dropped here — saving a job is taking
  // it on, so the board is where it belongs.
  //
  // Those rows always counted toward the 500 cap; showing them makes an existing
  // consequence visible rather than creating it. A user with hundreds of bookmarks can
  // push older active applications outside the fetched window, and now sees a wide
  // Preparing column while it happens. Still acceptable at this scale; the fix if it
  // bites is the same one as before — a server-side filter that pages the board's
  // columns separately, so a long backlog cannot crowd out the live applications.
  function build(rows: MyJob[]) {
    const next = emptyColumns();
    const cols: Record<string, BoardColumnId> = {};
    for (const row of rows) {
      const item: BoardItem = { ...row, id: row.id };
      const col = columnOf(item);
      if (!col) continue; // neither saved nor tracked — nothing to place
      next[col].push(item);
      cols[item.id] = col;
    }
    columns = next;
    cardCol = cols;
    status = 'ready';
    // Deep link: open the requested application's drawer once, after the board is
    // built. A slug that isn't on the board — one the user has neither saved nor
    // tracked — just leaves the board showing.
    if (initialId && !openedInitial) {
      openedInitial = true;
      const found = Object.values(next)
        .flat()
        .find((i) => i.id === initialId);
      if (found) {
        openItem = found;
        pendingOutcome = false;
      }
    }
  }

  async function load() {
    status = 'loading';
    try {
      const slice = await api.listMyJobs('board', 500, 0);
      build(slice.items);
    } catch {
      status = 'error';
    }
  }

  // Server-preloaded rows paint immediately (SSR + client-nav both run the route's
  // server load). `initial` is fixed for this mount (drawer open/close is pushState,
  // not a reload), so capture its presence once and only bootstrap a client fetch
  // when the caller gave us none.
  const preloaded = !!initial;
  if (initial) build(initial);

  $effect(() => {
    if (!preloaded && isAuthenticated()) void load();
  });

  // Search, mirrored into `?q=` so a shared or reloaded link shows the same rows.
  // UrlSyncedState owns the transport: it writes the URL synchronously on every
  // keystroke (the structural fix for the dropped-character race) and seeds from
  // location.search rather than page.url, which lags after a shallow-routing
  // back/forward. Filtering is local and instant, so there is nothing to debounce —
  // setNow writes and applies together, and the view reads `value`.
  const search = new UrlSyncedState<string>(page.url.searchParams, {
    parse: (p) => p.get('q') ?? '',
    serialize: (v) => new URLSearchParams(v ? { q: v } : {}),
  });
  syncOnNavigation(search);
  const query = $derived(search.value);

  const searching = $derived(query.trim().length > 0);
  const shown = $derived<Record<BoardColumnId, BoardItem[]>>(
    searching
      ? {
          preparing: columns.preparing.filter((i) => matchesQuery(i, query)),
          applied: columns.applied.filter((i) => matchesQuery(i, query)),
          interview: columns.interview.filter((i) => matchesQuery(i, query)),
          offer: columns.offer.filter((i) => matchesQuery(i, query)),
          closed: columns.closed.filter((i) => matchesQuery(i, query)),
        }
      : columns,
  );

  // The list is the same rows, read in one sequence instead of four. Ordered by last
  // activity — an application that moved yesterday is the one worth seeing first, which
  // the columns cannot express because they order by stage.
  const listItems = $derived(
    Object.values(shown)
      .flat()
      .sort((a, b) => (b.last_activity_at ?? '').localeCompare(a.last_activity_at ?? '')),
  );
  const total = $derived(Object.values(columns).flat().length);
  const matched = $derived(Object.values(shown).flat().length);

  function onconsider(id: BoardColumnId, items: BoardItem[]) {
    columns[id] = items; // live preview during the drag
  }

  function onfinalize(id: BoardColumnId, items: BoardItem[]) {
    columns[id] = items;
    for (const item of items) {
      if (cardCol[item.id] !== id) {
        cardCol[item.id] = id;
        void persistMove(item, id);
      }
    }
  }

  // Map a drop target to the backend write. Optimistic: on failure reload to
  // resync (simplest correct revert for a small board).
  async function persistMove(item: BoardItem, to: BoardColumnId) {
    try {
      switch (to) {
        case 'preparing':
          item.stage = 'preparing';
          await api.trackApplication(item.id, { stage: 'preparing' });
          break;
        case 'applied':
          item.stage = 'applied';
          await api.trackApplication(item.id, { stage: 'applied' });
          break;
        case 'interview':
          item.stage = 'interview';
          await api.trackApplication(item.id, { stage: 'interview' });
          break;
        case 'offer':
          item.stage = 'offer';
          await api.trackApplication(item.id, { stage: 'offer' });
          break;
        case 'closed':
          // Outcome unknown until the user picks: open the drawer, require a choice.
          openItem = item;
          pendingOutcome = true;
          break;
        default:
          assertNever(to);
      }
    } catch {
      await load();
    }
  }

  async function chooseOutcome(outcome: ClosedOutcome) {
    if (!openItem) return;
    const item = openItem;
    item.stage = outcome;
    pendingOutcome = false;
    try {
      await api.trackApplication(item.id, { stage: outcome });
    } catch {
      await load();
    }
  }

  // The drawer acts on whatever is open; a list row acts on itself. Same write either
  // way, so the panel just names the open application.
  async function setStage(stage: string) {
    if (openItem) await applyStage(openItem, stage);
  }

  async function applyStage(item: BoardItem, stage: string) {
    const prevCol = cardCol[item.id];
    item.stage = stage || null;
    // "No stage" clears the pipeline, not the job. A SAVED job then falls back to
    // Preparing (columnOf), which is where the board says a job you have taken on but
    // not applied to lives — so this reads as moving the card back to the start rather
    // than as deleting it. Only a job that was never saved leaves the board.
    if (!stage) item.applied_at = null;
    const nextCol = columnOf(item);
    if (nextCol === null) {
      // Never saved, and now untracked: drop the card and close the drawer. Nothing is
      // lost — the posting is still in the catalogue, it is just not on this board.
      if (prevCol) {
        columns[prevCol] = columns[prevCol].filter((i) => i.id !== item.id);
        Reflect.deleteProperty(cardCol, item.id);
      }
      openItem = null;
    } else if (prevCol && nextCol !== prevCol) {
      columns[prevCol] = columns[prevCol].filter((i) => i.id !== item.id);
      columns[nextCol] = [item, ...columns[nextCol]];
      cardCol[item.id] = nextCol;
    }
    try {
      // Empty stage is not a vocabulary value — clearing progress keeps the saved
      // mark but removes the application, so the job leaves the board.
      if (stage) {
        await api.trackApplication(item.id, { stage });
      } else {
        // Keeping the bookmark only means something while there is a posting to
        // bookmark. An application whose posting was removed has no saved mark to
        // preserve, and saveJob is addressed by a slug it does not have.
        if (item.job) await api.saveJob(item.job.public_slug);
        await api.clearApplicationStage(item.id);
      }
    } catch {
      await load();
    }
  }

  async function saveNotes(notes: string) {
    if (!openItem) return;
    openItem.notes = notes;
    try {
      await api.trackApplication(openItem.id, { notes });
    } catch {
      /* keep optimistic value; a transient failure shouldn't drop the edit */
    }
  }

  async function remove() {
    if (!openItem) return;
    const item = openItem;
    const col = cardCol[item.id];
    if (col) {
      columns[col] = columns[col].filter((i) => i.id !== item.id);
      Reflect.deleteProperty(cardCol, item.id);
    }
    openItem = null;
    try {
      await api.untrackApplication(item.id);
    } catch {
      await load();
    }
  }

  function closeDrawer() {
    if (pendingOutcome) {
      // Closing without choosing an outcome reverts the Closed drop.
      pendingOutcome = false;
      void load();
    }
    openItem = null;
    pushState(resolve('/my/tracking'), {}); // drop the per-application slug from the URL
  }

  function openFollowUp(item: MyJob) {
    followUpItem = item as BoardItem;
  }

  // Start a conversation held against one application and hand it over to the assistant
  // page, which opens it — the agent speaks first, so there is nothing to type on the way
  // in. The board does not wait for the turn: creating the session is the whole of its
  // job here.
  async function startApplicationSession(
    item: MyJob,
    create: (slug: string) => Promise<{ id: string }>,
    failure: string,
  ) {
    if (startingSession || !item.job) return;
    startingSession = true;
    try {
      const session = await create(item.job.public_slug);
      await goto(resolve('/my/assistant/[[id]]', { id: session.id }));
    } catch {
      sessionError = failure;
    } finally {
      startingSession = false;
    }
  }

  const startRehearsal = (item: MyJob) =>
    startApplicationSession(item, createRehearsal, 'Could not start the rehearsal.');
  const startDebrief = (item: MyJob) =>
    startApplicationSession(item, createDebrief, 'Could not start the debrief.');

  // Stamp the recorded chase onto the card in place. The board holds the only copy
  // of the row, and reloading the whole listing to learn one timestamp is noise —
  // the same optimistic treatment stage moves already get.
  function markChased(at: string) {
    if (followUpItem) followUpItem.followed_up_at = at;
  }

  function openDrawer(item: MyJob) {
    openItem = item as BoardItem;
    pendingOutcome = false;
    // Give the open application its own shareable URL without a full navigation.
    // Keyed by the row id, so an application whose posting was removed gets one too.
    pushState(resolve('/my/tracking/[id]', { id: (item as BoardItem).id }), {});
  }
</script>

{#if status === 'loading'}
  <States state="loading" />
{:else if status === 'error'}
  <States state="error" message="Couldn't load your board." />
{:else}
  <!-- One field over both views. It narrows what is already loaded: the listing is
       bounded, and a request per keystroke would buy nothing. -->
  <div class="mb-3 flex items-center gap-2">
    <div class="relative flex-1 sm:max-w-xs">
      <Search
        class="pointer-events-none absolute left-2.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground"
        aria-hidden="true"
      />
      <input
        type="search"
        value={query}
        oninput={(e) => search.setNow(e.currentTarget.value)}
        placeholder="Search company or role"
        aria-label="Search applications by company or role"
        class="w-full rounded-md border border-input bg-transparent py-1.5 pl-8 pr-8 text-sm"
      />
      {#if searching}
        <button
          type="button"
          onclick={() => search.setNow('')}
          aria-label="Clear search"
          class="absolute right-1.5 top-1/2 -translate-y-1/2 rounded p-1 text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
        >
          <XIcon class="size-3.5" />
        </button>
      {/if}
    </div>
    {#if searching}
      <span class="shrink-0 text-xs tabular-nums text-muted-foreground">{matched} of {total}</span>
    {/if}
  </div>

  {#if view === 'list'}
    <BoardList items={listItems} onopen={openDrawer} onsetstage={applyStage} />
  {:else}
    <div class="no-scrollbar flex gap-3 overflow-x-auto pb-2">
      {#each BOARD_COLUMNS as col (col.id)}
        <BoardColumn
          id={col.id}
          label={col.label}
          items={shown[col.id]}
          dragDisabled={searching}
          {onconsider}
          {onfinalize}
          onopen={openDrawer}
        />
      {/each}
    </div>
  {/if}
{/if}

{#if openItem}
  {#key openItem.id}
    <JobDrawer
      item={openItem}
      {pendingOutcome}
      onsetstage={setStage}
      onsavenotes={saveNotes}
      onchooseoutcome={chooseOutcome}
      onremove={remove}
      onclose={closeDrawer}
      onrehearse={startRehearsal}
      ondebrief={startDebrief}
      onfollowup={openFollowUp}
      {startingSession}
      {sessionError}
      blocked={!!followUpItem}
    />
  {/key}
{/if}

{#if followUpItem}
  {#key followUpItem.id}
    <FollowUpDialog
      slug={followUpItem.job?.public_slug ?? ''}
      company={followUpItem.job?.company ?? followUpItem.company_slug}
      onclose={() => (followUpItem = null)}
      onrecorded={markChased}
    />
  {/key}
{/if}
