<script lang="ts">
  import { cn } from './cn.js';

  // The small caps label that heads a page section — the same voice as every other
  // eyebrow on the site (it was a mono `// label` until the UI polish pass, which read as a
  // second, competing type style beside them).
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

<svelte:element
  this={as}
  class={cn('text-xs font-medium uppercase tracking-wider text-muted-foreground', className)}
  {style}
>
  {text}
</svelte:element>
