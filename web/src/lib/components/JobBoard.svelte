<script lang="ts">
  import { listMyJobs, trackJob, saveJob, clearJobStage, untrackJob } from '$lib/api';
  import { isAuthenticated } from '$lib/auth.svelte';
  import type { MyJob } from '$lib/types';
  import { BOARD_COLUMNS, columnOf, type BoardColumnId, type BoardItem, type ClosedOutcome } from '$lib/board';
  import BoardColumn from './BoardColumn.svelte';
  import JobDrawer from './JobDrawer.svelte';
  import States from './States.svelte';

  function emptyColumns(): Record<BoardColumnId, BoardItem[]> {
    return { saved: [], applied: [], interview: [], offer: [], closed: [] };
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
      const slice = await listMyJobs('board', 500, 0); // pipeline is small; whole board in one page
      const next = emptyColumns();
      const cols: Record<string, BoardColumnId> = {};
      for (const row of slice.items) {
        const item: BoardItem = { ...row, id: row.job.public_slug };
        const col = columnOf(item);
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
          await trackJob(item.id, { stage: 'applied' });
          break;
        case 'interview':
          item.stage = 'interview';
          await trackJob(item.id, { stage: 'interview' });
          break;
        case 'offer':
          item.stage = 'offer';
          await trackJob(item.id, { stage: 'offer' });
          break;
        case 'saved':
          item.stage = null;
          item.applied_at = null;
          await saveJob(item.id);
          await clearJobStage(item.id);
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
      await trackJob(item.id, { stage: outcome });
    } catch {
      await load();
    }
  }

  async function setStage(stage: string) {
    if (!openItem) return;
    const item = openItem;
    const prevCol = cardCol[item.id];
    item.stage = stage || null;
    if (!stage) item.applied_at = null; // "No stage" demotes back to the wishlist
    const nextCol = columnOf(item);
    if (prevCol && nextCol !== prevCol) {
      columns[prevCol] = columns[prevCol].filter((i) => i.id !== item.id);
      columns[nextCol] = [item, ...columns[nextCol]];
      cardCol[item.id] = nextCol;
    }
    try {
      // Empty stage is not a vocabulary value — clearing progress is its own
      // backend path (the same one the drag-to-Saved move uses).
      if (stage) {
        await trackJob(item.id, { stage });
      } else {
        await saveJob(item.id);
        await clearJobStage(item.id);
      }
    } catch {
      await load();
    }
  }

  async function saveNotes(notes: string) {
    if (!openItem) return;
    openItem.notes = notes;
    try {
      await trackJob(openItem.id, { notes });
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
      await untrackJob(item.id);
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
