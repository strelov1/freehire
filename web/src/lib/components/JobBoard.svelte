<script lang="ts">
  import { tick, untrack } from 'svelte';
  import { SvelteURLSearchParams } from 'svelte/reactivity';
  import { pushState, replaceState } from '$app/navigation';
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import { CircleAlert, Search, X as XIcon } from '@lucide/svelte';
  import { refreshAppliedCompanies } from '#lib/appliedCompanies.svelte.js';
  import { api } from '#lib/api.js';
  import { Button, EmptyState, alertVariants, cn } from '#lib/ui/index.js';
  import { announce } from '#lib/announce.svelte.js';
  import { ScrollEdges } from '#lib/scrollEdges.svelte.js';
  import { isAuthenticated } from '#lib/auth.svelte.js';
  import { UrlSyncedState, syncOnNavigation } from '#lib/urlSynced.svelte.js';
  import type { MyJob } from '#lib/types.js';
  import {
    BOARD_COLUMNS,
    columnOf,
    matchesQuery,
    matchesStage,
    needsAttention,
    sortBoardItems,
    parseBoardSort,
    type BoardSort,
    type BoardColumnId,
    type BoardItem,
    type ClosedOutcome,
  } from '#lib/board.js';
  import { assertNever } from '#lib/utils.js';
  import { humanizeStage, stageGroupLabel } from '#lib/stages.js';
  import BoardColumn from './BoardColumn.svelte';
  import BoardList from './BoardList.svelte';
  import JobDrawer from './JobDrawer.svelte';
  import FollowUpDialog from './FollowUpDialog.svelte';
  import States from './States.svelte';
  import { messages } from './JobBoard.messages';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { format, t } from '#lib/i18n/t.js';

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

  const s = $derived(t(messages, locale()));
  // The stage groups' labels are the generated vocabulary, translated in
  // `stages.messages.ts`; the inbox is ours, and reads as the "No stage" option every
  // stage picker already offers.
  const columnLabel = (col: { id: BoardColumnId; label: string }) =>
    col.id === 'unstaged' ? s.noStage : stageGroupLabel(col.id, locale());

  function emptyColumns(): Record<BoardColumnId, BoardItem[]> {
    return { unstaged: [], preparing: [], applied: [], interview: [], offer: [], closed: [] };
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
  // Whether the open drawer has its own history entry (pushed by openDrawer). Only then
  // does Back close it, and only then does closing it go back rather than forward. A
  // drawer opened by a Closed drop has no entry, and a deep-linked one IS the entry the
  // visitor arrived on — Back from either leaves the page, as it should.
  let drawerInHistory = false;
  // Set while the drawer is the one a deep link (/my/tracking/[id]) opened, so closing
  // it can swap that URL for the board's own instead of leaving it in the address bar.
  let onDeepLink = false;
  // The follow-up dialog is its own overlay, not a drawer tab: it is reached from
  // the card's silence badge and closes back to the board.
  let followUpItem = $state.raw<BoardItem | null>(null);
  // A write that failed and was rolled back. Shown as a pinned alert rather than beside the
  // board, because the drawer covers the viewport and a stage picked there fails there too.
  let actionError = $state<string | null>(null);
  // While a card is being dragged the columns must not snap: the library auto-scrolls the
  // row toward the pointer, and a mandatory snap would keep yanking it back.
  let dragging = $state(false);
  let searchInput = $state<HTMLInputElement>();
  let emptyState = $state<HTMLDivElement>();

  // Lay the fetched rows out into the columns and open the deep-linked drawer once.
  // The 'board' filter returns saved ∪ applied ∪ stage, and a saved-only row lands in
  // the no-stage inbox (see columnOf) rather than being dropped here — saving a job is
  // taking it on, so the board is where it belongs.
  //
  // Those rows always counted toward the 500 cap; showing them makes an existing
  // consequence visible rather than creating it. A user with hundreds of bookmarks can
  // push older active applications outside the fetched window, and now sees a wide
  // inbox column while it happens. Still acceptable at this scale; the fix if it
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
        onDeepLink = true;
      }
    }
  }

  async function load() {
    status = 'loading';
    try {
      const slice = await api.listMyJobs('board', 500, 0, { compact: true });
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

  // Search, the "needs attention" toggle and the stage filter, mirrored into
  // `?q=`/`?attention=1`/`?stage=` so a shared or reloaded link shows the same rows.
  // One UrlSyncedState over all three — not separate ones — because each instance's
  // write replaces the WHOLE query string with its own serialization; independent
  // states on the same page would each erase the others' params on every change. It writes the
  // URL synchronously on every keystroke/click (the structural fix for the
  // dropped-character race) and seeds from location.search rather than page.url,
  // which lags after a shallow-routing back/forward. Filtering is local and
  // instant, so there is nothing to debounce — setNow writes and applies
  // together, and the view reads `value`.
  const filters = new UrlSyncedState<{ q: string; attention: boolean; stage: string; sort: BoardSort }>(
    new URLSearchParams(page.url.search),
    {
      parse: (p) => ({
        q: p.get('q') ?? '',
        attention: p.get('attention') === '1',
        stage: p.get('stage') ?? '',
        sort: parseBoardSort(p.get('sort')),
      }),
      serialize: (v) =>
        new URLSearchParams({
          ...(v.q ? { q: v.q } : {}),
          ...(v.attention ? { attention: '1' } : {}),
          ...(v.stage ? { stage: v.stage } : {}),
          ...(v.sort !== 'default' ? { sort: v.sort } : {}),
        }),
    },
  );
  syncOnNavigation(filters);
  const query = $derived(filters.value.q);
  const attentionOnly = $derived(filters.value.attention);
  // The stage filter is the list's: on the board the columns already ARE that filter,
  // so it is neither offered nor applied there — a `?stage=` carried over from the
  // list must not quietly empty five of the six columns.
  const stageFilter = $derived(view === 'list' ? filters.value.stage : '');

  const sort = $derived(filters.value.sort);
  const searching = $derived(query.trim().length > 0);
  const filtering = $derived(searching || attentionOnly || stageFilter !== '');
  const matchesFilters = (i: BoardItem) =>
    matchesQuery(i, query) && (!attentionOnly || needsAttention(i)) && matchesStage(i, stageFilter);
  const shown = $derived<Record<BoardColumnId, BoardItem[]>>(
    Object.fromEntries(BOARD_COLUMNS.map((c) => [
      c.id,
      sortBoardItems(filtering ? columns[c.id].filter(matchesFilters) : columns[c.id], sort),
    ])) as Record<BoardColumnId, BoardItem[]>,
  );
  const listItems = $derived(sort === 'default'
    ? Object.values(shown).flat().sort((a, b) => (b.last_activity_at ?? '').localeCompare(a.last_activity_at ?? ''))
    : sortBoardItems(Object.values(shown).flat(), sort));
  const total = $derived(Object.values(columns).flat().length);
  const matched = $derived(Object.values(shown).flat().length);

  function clearSearch() {
    filters.setNow({ ...filters.value, q: '' });
    searchInput?.focus();
  }

  function clearFilters() {
    filters.setNow({ ...filters.value, q: '', attention: false, stage: '' });
    searchInput?.focus();
  }

  function onconsider(id: BoardColumnId, items: BoardItem[]) {
    dragging = true;
    columns[id] = items; // live preview during the drag
  }

  function onfinalize(id: BoardColumnId, items: BoardItem[]) {
    dragging = false;
    columns[id] = items;
    for (const item of items) {
      if (cardCol[item.id] !== id) {
        cardCol[item.id] = id;
        void persistMove(item, id);
      }
    }
  }

  // The application's own name, the way the card shows it — the posting can be gone.
  const jobName = (item: MyJob) => item.job?.title || item.role_title || s.untitledJob;

  const columnName = (id: BoardColumnId) => {
    const col = BOARD_COLUMNS.find((c) => c.id === id);
    return col ? columnLabel(col) : id;
  };

  const stageName = (stage: string | null) => (stage ? humanizeStage(stage, locale()) : s.noStage);

  // Polite confirmation of a stage change, for whoever cannot see the card move.
  function announceMove(item: MyJob, where: string) {
    actionError = null;
    announce(format(s.movedTo, { job: jobName(item), stage: where }));
  }

  // The visible alert is mounted together with its text, which a screen reader usually
  // does not read; the shared announcer's standing region carries the voice.
  function showError(message: string) {
    actionError = message;
    announce(message, 'assertive');
  }

  // A rejected move: reload to resync (the simplest correct revert for a small board), then
  // say so — naming where the card actually landed, which only the reload knows.
  async function rollBack(item: MyJob) {
    await load();
    const col = cardCol[item.id];
    showError(
      col
        ? format(s.moveFailed, { job: jobName(item), stage: columnName(col) })
        : format(s.moveFailedNoStage, { job: jobName(item) }),
    );
    // The drawer still holds the object the reload replaced; point it at the fresh one so it
    // shows the stage the server kept rather than the one that failed.
    if (openItem?.id === item.id) {
      const fresh = Object.values(columns)
        .flat()
        .find((i) => i.id === item.id);
      if (fresh) openItem = fresh;
    }
  }

  // Map a drop target to the backend write. Optimistic: on failure roll back and say so.
  async function persistMove(item: BoardItem, to: BoardColumnId) {
    try {
      switch (to) {
        case 'unstaged':
          await clearStage(item);
          // A row with no saved mark has nothing to keep it on the board once its stage
          // is gone; reload rather than leave a card the next load would not show.
          if (!item.saved_at) await load();
          break;
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
          return;
        default:
          assertNever(to);
      }
      announceMove(item, columnName(to));
    } catch {
      await rollBack(item);
    }
  }

  async function chooseOutcome(outcome: ClosedOutcome) {
    if (!openItem) return;
    const item = openItem;
    item.stage = outcome;
    pendingOutcome = false;
    try {
      await api.trackApplication(item.id, { stage: outcome });
      announceMove(item, stageName(outcome));
    } catch {
      await rollBack(item);
    }
  }

  // The drawer acts on whatever is open; a list row acts on itself. Same write either
  // way, so the panel just names the open application.
  async function setStage(stage: string) {
    if (openItem) await applyStage(openItem, stage);
  }

  // The drawer makes the actual review call itself (it alone holds the queue id, loaded
  // via getTrackedApplication) and calls this only to report the outcome — the mutation on
  // openItem belongs here, the same division applyStage/setStage already draw for stage.
  function applyAutoApplyReview(decision: 'approved' | 'declined') {
    if (openItem) openItem.auto_apply_status = decision;
  }

  async function applyStage(item: BoardItem, stage: string) {
    const prevCol = cardCol[item.id];
    item.stage = stage || null;
    // "No stage" clears the pipeline, not the job. A SAVED job then falls back to the
    // no-stage inbox (columnOf), where the board keeps a job you have taken on but not
    // started — so this reads as moving the card back to the start rather than as
    // deleting it. Only a job that was never saved leaves the board.
    if (!stage) item.applied_at = null;
    const nextCol = columnOf(item);
    if (nextCol === null) {
      // Never saved, and now untracked: drop the card and close the drawer. Nothing is
      // lost — the posting is still in the catalogue, it is just not on this board.
      if (prevCol) {
        columns[prevCol] = columns[prevCol].filter((i) => i.id !== item.id);
        Reflect.deleteProperty(cardCol, item.id);
      }
      closeDrawer();
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
        await clearStage(item);
      }
      announceMove(item, stageName(stage || null));
    } catch {
      await rollBack(item);
    }
  }

  // The write behind "No stage", whether picked from a selector or by dropping a card
  // into the inbox column.
  async function clearStage(item: BoardItem) {
    item.stage = null;
    item.applied_at = null;
    // Keeping the bookmark only means something while there is a posting to bookmark.
    // An application whose posting was removed has no saved mark to preserve, and
    // saveJob is addressed by a slug it does not have.
    if (item.job) {
      await api.saveJob(item.job.public_slug);
      item.saved_at ??= new Date().toISOString();
    }
    await api.clearApplicationStage(item.id);
    void refreshAppliedCompanies();
  }

  // Rejects on failure: the editor reports it beside itself and keeps the text for a retry.
  // The row is written only once the server has it — writing it first would push the old
  // value back into the editor on a rollback and lose what the user typed.
  //
  // Bound to the item rather than read from openItem: the editor also saves on teardown,
  // which on a close runs after openItem is already null.
  async function saveNotes(item: BoardItem, notes: string) {
    try {
      await api.trackApplication(item.id, { notes });
    } catch (e) {
      // Saved on the way out (Escape, Back): the editor is gone, so the board says it.
      if (openItem?.id !== item.id) showError(format(s.notesFailed, { job: jobName(item) }));
      throw e;
    }
    item.notes = notes;
  }

  // Shared by the card and drawer. Untracking clears progress and notes, so the
  // card leaves only after the server agrees. Each caller displays a failure in place.
  async function removeItem(item: MyJob) {
    try {
      await api.untrackApplication(item.id);
      void refreshAppliedCompanies();
    } catch {
      throw new Error(s.removeFailed);
    }
    const col = cardCol[item.id];
    if (col) {
      columns[col] = columns[col].filter((i) => i.id !== item.id);
      Reflect.deleteProperty(cardCol, item.id);
    }
    if (openItem?.id === item.id) closeDrawer();
    announce(format(s.removed, { job: jobName(item) }));
    // The card that opened the drawer is gone, so the drawer's own focus return would land
    // on <body>. The search field is the board's first control and is there in both views.
    // The last card going takes the field with it; the empty state stands in.
    await tick();
    (searchInput ?? emptyState)?.focus();
  }

  async function remove() {
    if (openItem) await removeItem(openItem);
  }

  let removing = $state<string[]>([]);
  async function quickRemove(item: MyJob) {
    if (removing.includes(item.id)) return;
    removing = [...removing, item.id];
    actionError = null;
    try {
      await removeItem(item);
    } catch {
      showError(s.removeFailed);
    } finally {
      removing = removing.filter((id) => id !== item.id);
    }
  }

  // The row of columns is wider than a phone, and a hidden scrollbar leaves nothing saying
  // so: fade whichever edge still has columns beyond it.
  const rowEdges = new ScrollEdges();
  let boardRow = $state<HTMLDivElement>();
  let visibleColumn = $state<BoardColumnId>('unstaged');

  $effect(() => {
    if (boardRow) untrack(() => {
      const first = (initialId ? cardCol[initialId] : undefined)
        ?? BOARD_COLUMNS.find((column) => shown[column.id].length > 0)?.id;
      if (first) goToColumn(first);
      else syncVisibleColumn();
    });
  });

  function syncVisibleColumn() {
    if (!boardRow) return;
    const left = boardRow.getBoundingClientRect().left;
    let nearest = Infinity;
    for (const [index, column] of BOARD_COLUMNS.entries()) {
      const section = boardRow.children[index];
      if (!section) continue;
      const rect = section.getBoundingClientRect();
      const distance = Math.abs(rect.left - left);
      if (distance < nearest) {
        nearest = distance;
        visibleColumn = column.id;
      }
    }
  }

  function goToColumn(id: string) {
    const index = BOARD_COLUMNS.findIndex((col) => col.id === id);
    const section = boardRow?.children[index];
    if (!boardRow || !section) return;
    boardRow.scrollLeft += section.getBoundingClientRect().left - boardRow.getBoundingClientRect().left;
    syncVisibleColumn();
  }

  // Close the drawer without touching the URL — Back has already moved it.
  function dismissDrawer() {
    if (pendingOutcome) {
      // Closing without choosing an outcome reverts the Closed drop.
      pendingOutcome = false;
      void load();
    }
    openItem = null;
  }

  function closeDrawer() {
    const hadEntry = drawerInHistory;
    drawerInHistory = false;
    dismissDrawer();
    if (hadEntry) {
      // Undo openDrawer's push rather than pushing the list URL on top of it: a second
      // entry is what left Back walking through closed drawers, and going back also
      // returns to the exact view and filters the drawer was opened from.
      history.back();
    } else if (onDeepLink) {
      // Arrived on a deep link: swap the per-application URL for the board's own.
      onDeepLink = false;
      const params = new SvelteURLSearchParams(location.search);
      params.delete('application');
      const base = view === 'list' ? resolve('/my/tracking/list') : resolve('/my/tracking');
      // eslint-disable-next-line svelte/no-navigation-without-resolve -- both possible base routes above are resolved
      replaceState(base + (params.size ? `?${params}` : ''), {});
    }
  }

  // Back and Forward over the drawer's history entry. SvelteKit restores page.state
  // with the entry, so the open application is read from it rather than kept only in
  // this component — a URL that changed while the drawer stayed put was the bug.
  $effect(() => {
    const id = page.state.trackingItem;
    untrack(() => {
      if (id) {
        if (openItem?.id !== id) {
          const found = Object.values(columns)
            .flat()
            .find((i) => i.id === id);
          if (!found) return;
          openItem = found;
          pendingOutcome = false;
        }
        drawerInHistory = true;
      } else if (drawerInHistory) {
        drawerInHistory = false;
        dismissDrawer();
      }
    });
  });

  function openFollowUp(item: MyJob) {
    followUpItem = item as BoardItem;
  }

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
    // The query rides along so the filtered rows behind the drawer stay filtered.
    const id = (item as BoardItem).id;
    drawerInHistory = true;
    if (view === 'list') {
      const params = new SvelteURLSearchParams(location.search);
      params.set('application', id);
      // eslint-disable-next-line svelte/no-navigation-without-resolve -- the route is resolved before its query is added
      pushState(resolve('/my/tracking/list') + `?${params}`, { trackingItem: id });
    } else {
      // eslint-disable-next-line svelte/no-navigation-without-resolve -- the route is resolved before its query is added
      pushState(resolve('/my/tracking/[id]', { id }) + location.search, { trackingItem: id });
    }
  }
</script>

{#if status === 'loading'}
  <States state="loading" />
{:else if status === 'error'}
  <States state="error" message={s.loadError} />
{:else if total === 0}
  <!-- A first visit, or every card removed. Empty columns with a 0 on each say nothing
       about what the board is for or how anything gets onto it. -->
  <div bind:this={emptyState} tabindex="-1" class="rounded-xl border border-dashed border-border outline-none">
    <EmptyState title={s.emptyTitle} description={s.emptyDescription}>
      {#snippet action()}
        <div class="flex flex-wrap items-center justify-center gap-2">
          <Button variant="primary" size="sm" href={resolve('/jobs')}>{s.emptyFindJobs}</Button>
          <Button variant="outline" size="sm" href={resolve('/my/profile')}>{s.emptyUploadCv}</Button>
        </div>
      {/snippet}
    </EmptyState>
  </div>
{:else}
  <!-- One field over both views. It narrows what is already loaded: the listing is
       bounded, and a request per keystroke would buy nothing. -->
  <div class="mb-3 flex flex-wrap items-center gap-2">
    <div class="relative w-full sm:max-w-xs sm:flex-1">
      <Search class="pointer-events-none absolute left-2.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" aria-hidden="true" />
      <input
        bind:this={searchInput}
        type="search"
        value={query}
        oninput={(e) => filters.setNow({ ...filters.value, q: e.currentTarget.value })}
        placeholder={s.searchPlaceholder}
        aria-label={s.searchAria}
        class="h-10 w-full rounded-md border border-input bg-transparent pl-8 pr-10 text-base focus-visible:border-ring focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring sm:h-9 sm:text-sm [&::-webkit-search-cancel-button]:appearance-none"
      />
      {#if searching}
        <button type="button" onclick={clearSearch} aria-label={s.clearSearchAria}
          class="absolute right-0 top-0 flex size-10 items-center justify-center rounded text-muted-foreground hover:text-foreground sm:size-9">
          <XIcon class="size-3.5" />
        </button>
      {/if}
    </div>
    <div class="flex w-full items-center gap-2 sm:w-auto">
      <button type="button" aria-label={s.needsAttention} title={s.needsAttention} aria-pressed={attentionOnly}
        onclick={() => filters.setNow({ ...filters.value, attention: !attentionOnly })}
        class={cn('inline-flex h-8 shrink-0 items-center justify-center gap-1.5 rounded-md px-2 text-xs font-medium transition-colors',
          attentionOnly ? 'bg-warning-muted text-warning-strong' : 'text-muted-foreground hover:bg-accent hover:text-foreground')}>
        <CircleAlert class="size-4 shrink-0" aria-hidden="true" />
        <span class="sr-only sm:not-sr-only">{s.needsAttention}</span>
      </button>
      <select value={sort} onchange={(e) => filters.setNow({ ...filters.value, sort: parseBoardSort(e.currentTarget.value) })}
        aria-label={s.sortAria} class="h-8 min-w-0 flex-1 rounded-md border border-input bg-background px-2 text-xs sm:w-40 sm:flex-none sm:text-sm">
        <option value="default">{s.sortDefault}</option>
        <option value="company">{s.sortCompany}</option>
        <option value="added">{s.sortAdded}</option>
        <option value="deadline">{s.sortDeadline}</option>
      </select>
      {#if view === 'board'}
        <select value={visibleColumn} onchange={(e) => goToColumn(e.currentTarget.value)}
          aria-label={s.boardStage} class="h-8 min-w-0 flex-1 rounded-md border border-input bg-background px-2 text-xs sm:w-40 sm:flex-none sm:text-sm">
          {#each BOARD_COLUMNS as col (col.id)}
            <option value={col.id}>{columnLabel(col)} ({shown[col.id].length})</option>
          {/each}
        </select>
      {:else}
        <select value={stageFilter} onchange={(e) => filters.setNow({ ...filters.value, stage: e.currentTarget.value })}
          aria-label={s.stageFilterAria} class="h-8 min-w-0 flex-1 rounded-md border border-input bg-background px-2 text-xs sm:w-40 sm:flex-none sm:text-sm">
          <option value="">{s.allStages}</option>
          {#each BOARD_COLUMNS as col (col.id)}<option value={col.id}>{columnLabel(col)}</option>{/each}
        </select>
      {/if}
    </div>
    {#if filtering}
      <span role="status" class="text-xs tabular-nums text-muted-foreground">{format(s.matchedOfTotal, { matched: String(matched), total: String(total) })}</span>
    {/if}
  </div>

  {#if filtering && matched === 0}
    <div class="rounded-xl border border-dashed border-border">
      <EmptyState title={s.noMatches} description={s.noMatchesDescription}>
        {#snippet action()}
          <Button variant="outline" size="sm" class="min-h-11 sm:min-h-0" onclick={clearFilters}>{s.clearFilters}</Button>
        {/snippet}
      </EmptyState>
    </div>
  {:else if view === 'list'}
    <!-- Keyed on the filters so a new search starts from the first page: the list draws
         rows a page at a time, and a cleared filter would otherwise redraw every row a
         long scroll had reached, in one update. -->
    {#key `${query}|${attentionOnly}|${stageFilter}|${sort}`}
      <BoardList items={listItems} onopen={openDrawer} onsetstage={applyStage} onremove={quickRemove} {removing} />
    {/key}
  {:else}
    <div
      bind:this={boardRow}
      onscroll={syncVisibleColumn}
      {@attach rowEdges.attach}
      class={cn(
        'no-scrollbar flex gap-3 overflow-x-auto pb-2',
        !dragging && 'snap-x snap-mandatory',
        !rowEdges.atStart && 'mask-l-from-90%',
        !rowEdges.atEnd && 'mask-r-from-90%',
      )}
    >
      {#each BOARD_COLUMNS as col (col.id)}
        <BoardColumn
          id={col.id}
          label={columnLabel(col)}
          items={shown[col.id]}
          dragDisabled={filtering || sort !== 'default'}
          {onconsider}
          {onfinalize}
          onopen={openDrawer}
          onremove={quickRemove}
          {removing}
        />
      {/each}
    </div>
  {/if}
{/if}

{#if actionError}
  <!-- Pinned above everything, the drawer included: a stage picked there fails there. -->
  <div class="fixed inset-x-0 bottom-safe z-toast flex justify-center px-4">
    <div class={cn(alertVariants({ variant: 'destructive' }), 'w-full max-w-md items-center bg-background shadow-lg')}>
      <p class="flex-1">{actionError}</p>
      <button
        type="button"
        onclick={() => (actionError = null)}
        aria-label={s.dismissError}
        class="flex size-11 shrink-0 items-center justify-center rounded transition-colors hover:bg-destructive/10 sm:size-auto sm:p-1"
      >
        <XIcon class="size-4" aria-hidden="true" />
      </button>
    </div>
  </div>
{/if}

{#if openItem}
  {#key openItem.id}
    {@const item = openItem}
    <JobDrawer
      {item}
      {pendingOutcome}
      onsetstage={setStage}
      onsavenotes={(notes) => saveNotes(item, notes)}
      onchooseoutcome={chooseOutcome}
      onautoapplyreview={applyAutoApplyReview}
      onremove={remove}
      onclose={closeDrawer}
      onfollowup={openFollowUp}
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
