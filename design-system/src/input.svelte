<script lang="ts">
  import type { HTMLInputAttributes } from 'svelte/elements';
  import { cn } from './cn.js';

  // The shared text/search input surface — one styled <input> primitive so the
  // search boxes and the facet filter cannot drift in border/focus/dark styling.
  // No width is baked in; call sites pass `w-full` or `flex-1` via class. value is
  // $bindable, so a call site may either bind:value or pass value + oninput (the
  // latter flow through ...rest).
  let {
    value = $bindable(),
    class: className,
    ...rest
  }: { value?: string; class?: string } & HTMLInputAttributes = $props();
</script>

<input
  bind:value
  class={cn(
    'h-9 min-w-0 rounded-lg border border-input bg-transparent px-3 transition-colors placeholder:text-muted-foreground focus-visible:border-ring focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50 dark:bg-input/30',
    className,
  )}
  {...rest}
/>
