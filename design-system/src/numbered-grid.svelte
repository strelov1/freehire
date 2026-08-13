<script lang="ts">
  import { cn } from './cn.js';

  // A numbered hairline grid for "01 / 02 / 03" blocks: cells sit on a
  // `bg-border` grid with `gap-px`, so the one-pixel gaps read as hairline
  // rules instead of borders that double up between cells.
  interface NumberedItem {
    n: string;
    title: string;
    body: string;
  }

  let { items, class: className }: { items: NumberedItem[]; class?: string } = $props();
</script>

<div class={cn('grid gap-px overflow-hidden rounded-xl border border-border bg-border', className)}>
  {#each items as item (item.n)}
    <div class="group bg-background p-6 transition-colors hover:bg-secondary/40 sm:p-7">
      <span class="font-mono text-sm text-muted-foreground transition-colors group-hover:text-foreground">
        {item.n}
      </span>
      <h3 class="mt-4 text-lg font-semibold tracking-tight">{item.title}</h3>
      <p class="mt-2 text-sm leading-relaxed text-muted-foreground">{item.body}</p>
    </div>
  {/each}
</div>
