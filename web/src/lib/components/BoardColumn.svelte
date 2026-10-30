<script lang="ts">
  import { tick } from 'svelte';
  import { dndzone } from 'svelte-dnd-action';
  import BoardCard from './BoardCard.svelte';
  import type { BoardItem, BoardColumnId } from '#lib/board.js';
  import type { MyJob } from '#lib/types.js';

  let {
    id,
    label,
    items,
    onconsider,
    onfinalize,
    onopen,
    dragDisabled = false,
  }: {
    id: BoardColumnId;
    label: string;
    items: BoardItem[];
    // Set while the board is filtered — by search text or the "needs attention" toggle.
    // The zone is handed the matching rows only, and svelte-dnd-action answers a drop
    // with the array it was given — so a drop while filtered would write the visible
    // subset back as the whole column and lose every row the filter hid.
    dragDisabled?: boolean;
    onconsider: (id: BoardColumnId, items: BoardItem[]) => void;
    onfinalize: (id: BoardColumnId, items: BoardItem[]) => void;
    // BoardItem extends MyJob; the card calls back with the item it received,
    // which satisfies MyJob. The parent can widen back to BoardItem via cast.
    onopen: (item: MyJob) => void;
  } = $props();

  // Neutral drop-target frame — overrides svelte-dnd-action's default yellow
  // outline, which clashes with the monochrome palette.
  const dropTargetStyle = { outline: '2px solid var(--ring)', borderRadius: 'var(--radius-md)' };

  // A labelled <section> is a region landmark, so each column is reachable by its name.
  const titleId = $props.id();

  // Cards are drawn a page at a time as the column is scrolled, not all at once: a board
  // of 500 applications took ~0.85 s of main thread to build every card (measured
  // 2026-10-30, one column alone held 163). A drawn card stays drawn — unloading cards
  // that scroll away would hand svelte-dnd-action a zone that changes under the pointer.
  const PAGE = 30;
  let limit = $state(PAGE);
  let dragging = false;
  let zone: HTMLElement | undefined = $state();
  const visible = $derived(items.length > limit ? items.slice(0, limit) : items);

  // The zone holds only the drawn cards, and svelte-dnd-action answers with that array.
  // The undrawn rest of the column rides along untouched — or a drop would write the
  // drawn part back as the whole column — and the boundary follows the zone, so a card
  // dragged out does not pull an undrawn one into view mid-drag.
  function withTail(zoneItems: BoardItem[]): BoardItem[] {
    if (items.length <= limit) return zoneItems;
    const tail = items.slice(limit);
    limit = zoneItems.length;
    return [...zoneItems, ...tail];
  }

  // Draw the next page once the column is scrolled near its end. Paused while a card is
  // dragged: auto-scroll would otherwise grow the zone under the pointer.
  function grow() {
    if (!zone || dragging || limit >= items.length || zone.clientHeight === 0) return;
    if (zone.scrollTop + zone.clientHeight >= zone.scrollHeight - 400) limit += PAGE;
  }

  // A page too short to scroll can never be scrolled to its end, so check again after
  // each page is drawn.
  $effect(() => {
    void visible.length;
    void tick().then(grow);
  });
</script>

<!-- snap-start: the board's row snaps (JobBoard), so a swipe lands on a column, not between two. -->
<section
  aria-labelledby={titleId}
  class="flex w-72 shrink-0 snap-start flex-col gap-2 rounded-xl bg-secondary/40 p-2"
>
  <header class="flex items-center justify-between px-1 py-0.5 text-sm font-medium">
    <h2 id={titleId}>{label}</h2>
    <span class="text-xs tabular-nums text-muted-foreground">{items.length}</span>
  </header>
  <!-- Each column's card list is its own bounded scroll container. svelte-dnd-action
       auto-scrolls a scrollable *container* correctly during a drag, but mis-places
       the drop once the *window* auto-scrolls (a card dropped low in a long column
       would snap back near the top). Capping the height and scrolling here keeps the
       drag inside a container the library handles, so a card lands where it's
       dropped — and the column header/count stay put. -->
  <div
    class="flex max-h-[calc(100dvh-14rem)] min-h-24 flex-col gap-2 overflow-y-auto"
    bind:this={zone}
    onscroll={grow}
    use:dndzone={{ items: visible, flipDurationMs: 150, type: 'board', dropTargetStyle, dragDisabled }}
    onconsider={(e) => {
      dragging = true;
      onconsider(id, withTail(e.detail.items as BoardItem[]));
    }}
    onfinalize={(e) => {
      dragging = false;
      onfinalize(id, withTail(e.detail.items as BoardItem[]));
    }}
  >
    {#each visible as item (item.id)}
      <div>
        <BoardCard {item} {onopen} />
      </div>
    {/each}
  </div>
</section>
