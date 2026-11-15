<script lang="ts">
  import type { HTMLAttributes } from 'svelte/elements';
  import { cn } from '#lib/ui/index.js';

  let { children, class: className, ...rest }: HTMLAttributes<HTMLHeadingElement> = $props();
</script>

<h1 {...rest} class={cn('break-words text-2xl font-semibold tracking-tight max-lg:text-xl', className)}>
  {@render children?.()}
</h1>
