<script lang="ts">
  import { cn } from './cn.js';

  // The mono `// section` label that heads a page section. The `//` is part of
  // the label, so callers pass the bare section name.
  let {
    text,
    class: className,
    style,
  }: { text: string; class?: string; style?: string } = $props();
</script>

<p class={cn('font-mono text-xs uppercase tracking-[0.2em] text-muted-foreground', className)} {style}>
  // {text}
</p>
