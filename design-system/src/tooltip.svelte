<script lang="ts">
  import type { Snippet } from 'svelte';
  import { cn } from './cn.js';

  let {
    content,
    side = 'top',
    class: className,
    children,
  }: {
    content: Snippet;
    side?: 'top' | 'right' | 'bottom' | 'left';
    class?: string;
    children: Snippet;
  } = $props();

  let visible = $state(false);
  let triggerEl: HTMLElement | undefined = $state();

  const positions = {
    top: 'bottom-full left-1/2 -translate-x-1/2 mb-2',
    right: 'left-full top-1/2 -translate-y-1/2 ml-2',
    bottom: 'top-full left-1/2 --translate-x-1/2 mt-2',
    left: 'right-full top-1/2 -translate-y-1/2 mr-2',
  };
</script>

<span
  class="relative inline-flex"
  bind:this={triggerEl}
  onmouseenter={() => (visible = true)}
  onmouseleave={() => (visible = false)}
  onfocusin={() => (visible = true)}
  onfocusout={() => (visible = false)}
>
  {@render children()}
  {#if visible}
    <div
      role="tooltip"
      class={cn(
        'absolute z-popover max-w-xs rounded-md border border-border bg-popover px-3 py-1.5 text-xs text-popover-foreground shadow-md',
        positions[side],
        className,
      )}
    >
      {@render content()}
    </div>
  {/if}
</span>
