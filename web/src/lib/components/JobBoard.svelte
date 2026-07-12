<script lang="ts">
  import { api } from '$lib/api';
  import { isAuthenticated } from '$lib/auth.svelte';
  import type { MyJob } from '$lib/types';
  import { BOARD_COLUMNS, columnOf, type BoardColumnId, type BoardItem, type ClosedOutcome } from '$lib/board';
  import BoardColumn from './BoardColumn.svelte';
  import JobDrawer from './JobDrawer.svelte';
  import States from './States.svelte';

  function emptyColumns(): Record<BoardColumnId, BoardItem[]> {
    return { applied: [], interview: [], offer: [], closed: [] };
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

  async function load() {
    status = 'loading';
    try {
      // The 'board' filter returns saved ∪ applied ∪ stage; we drop the saved-only
      // rows below (they belong to Activity → Saved). Those rows still count toward
      // this 500 cap, so a user with 500+ tracked jobs, many recently saved, could
      // have older active applications fall outside the fetched window. Acceptable
      // at this scale; revisit with a server-side board-minus-saved filter if it bites.
      const slice = await api.listMyJobs('board', 500, 0);
      const next = emptyColumns();
      const cols: Record<string, BoardColumnId> = {};
      for (const row of slice.items) {
        const item: BoardItem = { ...row, id: row.job.public_slug };
        const col = columnOf(item);
        if (!col) continue; // saved-only rows live in Activity → Saved, not the board
        next[col].push(item);
        cols[item.id] = col;
      }
      columns = next;
      cardCol = cols;
      status = 'ready';
    } catch {
      status = 'error';
    }
  }

  $effect(() => {
    if (isAuthenticated()) void load();
  });

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
        case 'applied':
          item.stage = 'applied';
          await api.trackJob(item.id, { stage: 'applied' });
          break;
        case 'interview':
          item.stage = 'interview';
          await api.trackJob(item.id, { stage: 'interview' });
          break;
        case 'offer':
          item.stage = 'offer';
          await api.trackJob(item.id, { stage: 'offer' });
          break;
        case 'closed':
          // Outcome unknown until the user picks: open the drawer, require a choice.
          openItem = item;
          pendingOutcome = true;
          break;
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
      await api.trackJob(item.id, { stage: outcome });
    } catch {
      await load();
    }
  }

  async function setStage(stage: string) {
    if (!openItem) return;
    const item = openItem;
    const prevCol = cardCol[item.id];
    item.stage = stage || null;
    if (!stage) item.applied_at = null; // "No stage" takes the job off the board (saved-only)
    const nextCol = columnOf(item);
    if (nextCol === null) {
      // Off the board now: drop the card and close the drawer. The job keeps its
      // saved mark and reappears under Activity → Saved.
      if (prevCol) {
        columns[prevCol] = columns[prevCol].filter((i) => i.id !== item.id);
        delete cardCol[item.id];
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
        await api.trackJob(item.id, { stage });
      } else {
        await api.saveJob(item.id);
        await api.clearJobStage(item.id);
      }
    } catch {
      await load();
    }
  }

  async function saveNotes(notes: string) {
    if (!openItem) return;
    openItem.notes = notes;
    try {
      await api.trackJob(openItem.id, { notes });
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
      delete cardCol[item.id];
    }
    openItem = null;
    try {
      await api.untrackJob(item.id);
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
  }

  function openDrawer(item: MyJob) {
    openItem = item as BoardItem;
    pendingOutcome = false;
  }
</script>

{#if status === 'loading'}
  <States state="loading" />
{:else if status === 'error'}
  <States state="error" message="Couldn't load your board." />
{:else}
  <div class="flex gap-3 overflow-x-auto pb-2">
    {#each BOARD_COLUMNS as col (col.id)}
      <BoardColumn
        id={col.id}
        label={col.label}
        items={columns[col.id]}
        {onconsider}
        {onfinalize}
        onopen={openDrawer}
      />
    {/each}
  </div>
{/if}

{#if openItem}
  <JobDrawer
    item={openItem}
    {pendingOutcome}
    onsetstage={setStage}
    onsavenotes={saveNotes}
    onchooseoutcome={chooseOutcome}
    onremove={remove}
    onclose={closeDrawer}
  />
{/if}
