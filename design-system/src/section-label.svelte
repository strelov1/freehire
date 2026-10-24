<script lang="ts">
  import { cn } from './cn.js';

  // The mono `// section` label that heads a page section. The `//` is part of
  // the label, so callers pass the bare section name.
  //
  // `as` picks the element. A <p> is right where the label decorates a section that
  // already has its own heading; where the label IS the section's only title, it must be
  // a heading, or a screen-reader user walking the page by headings skips the section.
  // The `//` is visual either way — read aloud it is "slash slash".
  let {
    text,
    as = 'p',
    class: className,
    style,
  }: { text: string; as?: 'p' | 'h2' | 'h3'; class?: string; style?: string } = $props();
</script>

<svelte:element this={as} class={cn('font-mono text-xs uppercase tracking-[0.2em] text-muted-foreground', className)} {style}>
  <span aria-hidden="true">//</span>
  {text}
</svelte:element>
