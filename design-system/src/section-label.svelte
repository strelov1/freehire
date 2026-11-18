<script lang="ts">
  import { cn } from './cn.js';

  // A quiet section label, using the same sentence case and spacing as the interface.
  //
  // `as` picks the element. A <p> is right where the label decorates a section that
  // already has its own heading; where the label IS the section's only title, it must be
  // a heading, or a screen-reader user walking the page by headings skips the section.
  let {
    text,
    as = 'p',
    class: className,
    style,
  }: { text: string; as?: 'p' | 'h2' | 'h3'; class?: string; style?: string } = $props();
</script>

<svelte:element this={as} class={cn('text-xs font-medium text-muted-foreground', className)} {style}>
  {text}
</svelte:element>
