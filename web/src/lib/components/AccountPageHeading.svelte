<script lang="ts">
  import { getContext } from 'svelte';
  import type { Snippet } from 'svelte';
  import type { HTMLAttributes } from 'svelte/elements';
  import { accountHeaderActions } from '#lib/accountHeader.js';
  import { cn } from '#lib/ui/index.js';

  let { children, class: className, ...rest }: HTMLAttributes<HTMLHeadingElement> = $props();
  const actions = getContext<Snippet | undefined>(accountHeaderActions);
</script>

<div class={cn('flex items-center justify-between gap-2', className)}>
  <h1 {...rest} class="min-w-0 flex-1 break-words text-2xl font-semibold tracking-tight max-lg:text-xl">
    {@render children?.()}
  </h1>
  {#if actions}
    {@render actions()}
  {/if}
</div>
