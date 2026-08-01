<script lang="ts">
  import { dndzone } from 'svelte-dnd-action';
  import BoardCard from './BoardCard.svelte';
  import type { BoardItem, BoardColumnId } from '$lib/board';
  import type { MyJob } from '$lib/types';

  let {
    id,
    label,
    items,
    onconsider,
    onfinalize,
    onopen,
  }: {
    id: BoardColumnId;
    label: string;
    items: BoardItem[];
    onconsider: (id: BoardColumnId, items: BoardItem[]) => void;
    onfinalize: (id: BoardColumnId, items: BoardItem[]) => void;
    // BoardItem extends MyJob; the card calls back with the item it received,
    // which satisfies MyJob. The parent can widen back to BoardItem via cast.
    onopen: (item: MyJob) => void;
  } = $props();

  // Neutral drop-target frame — overrides svelte-dnd-action's default yellow
  // outline, which clashes with the monochrome palette.
  const dropTargetStyle = { outline: '2px solid var(--ring)', borderRadius: 'var(--radius-md)' };
</script>

<section class="flex w-72 shrink-0 flex-col gap-2 rounded-xl bg-secondary/40 p-2">
  <header class="flex items-center justify-between px-1 py-0.5 text-sm font-medium">
    <span>{label}</span>
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
    use:dndzone={{ items, flipDurationMs: 150, type: 'board', dropTargetStyle }}
    onconsider={(e) => onconsider(id, e.detail.items as BoardItem[])}
    onfinalize={(e) => onfinalize(id, e.detail.items as BoardItem[])}
  >
    {#each items as item (item.id)}
      <div>
        <BoardCard {item} {onopen} />
      </div>
    {/each}
  </div>
</section>
