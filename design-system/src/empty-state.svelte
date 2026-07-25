<script lang="ts">
  import type { Snippet } from 'svelte';
  import { cn } from './cn.js';

  let {
    title = 'Nothing here yet',
    description,
    icon,
    class: className,
    action,
  }: {
    title?: string;
    description?: string;
    icon?: Snippet;
    class?: string;
    action?: Snippet;
  } = $props();
</script>

<div
  class={cn('flex flex-col items-center justify-center gap-3 py-12 text-center', className)}
  role="status"
>
  {#if icon}
    <div class="text-muted-foreground">
      {@render icon()}
    </div>
  {/if}
  <p class="text-sm font-medium text-foreground">{title}</p>
  {#if description}
    <p class="max-w-sm text-sm text-muted-foreground">{description}</p>
  {/if}
  {#if action}
    <div class="mt-2">
      {@render action()}
    </div>
  {/if}
</div>
