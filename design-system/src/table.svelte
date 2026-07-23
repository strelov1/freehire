<script lang="ts" module>
  import { tv, type VariantProps } from 'tailwind-variants';

  export const tableVariants = tv({
    base: 'w-full caption-bottom text-sm',
    slots: {
      thead: 'border-b border-border [&_tr]:border-b',
      tbody: '[&_tr:last-child]:border-0',
      tr: 'border-b border-border transition-colors hover:bg-muted/50',
      th: 'h-10 px-3 text-left font-medium text-muted-foreground',
      td: 'p-3 align-middle',
    },
  });

  export type TableSlots = VariantProps<typeof tableVariants>;
</script>

<script lang="ts">
  import type { Snippet } from 'svelte';
  import { cn } from './cn.js';

  let {
    class: className,
    header,
    children,
  }: { class?: string; header?: Snippet; children: Snippet } = $props();

  let slots = tableVariants();
</script>

<div class={cn('w-full overflow-x-auto', className)}>
  <table class={slots.base()}>
    {#if header}
      <thead class={slots.thead()}>
        {@render header()}
      </thead>
    {/if}
    <tbody class={slots.tbody()}>
      {@render children()}
    </tbody>
  </table>
</div>
