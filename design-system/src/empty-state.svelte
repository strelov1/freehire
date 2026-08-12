<script lang="ts" module>
  import { tv, type VariantProps } from 'tailwind-variants';

  export const emptyStateVariants = tv({
    slots: {
      title: 'text-sm text-foreground',
    },
    variants: {
      variant: {
        default: { title: 'font-medium' },
        muted: { title: 'text-muted-foreground' },
        destructive: { title: 'text-destructive' },
      },
    },
    defaultVariants: { variant: 'default' },
  });

  export type EmptyStateVariant = VariantProps<typeof emptyStateVariants>['variant'];
</script>

<script lang="ts">
  import type { Snippet } from 'svelte';
  import { cn } from './cn.js';

  let {
    title = 'Nothing here yet',
    description,
    icon,
    variant = 'default',
    class: className,
    action,
  }: {
    title?: string;
    description?: string;
    icon?: Snippet;
    variant?: EmptyStateVariant;
    class?: string;
    action?: Snippet;
  } = $props();

  const slots = $derived(emptyStateVariants({ variant }));
  // Mirrors alert.svelte: a destructive result is worth interrupting for, the
  // other variants are page furniture that should not barge in on every render.
  const role = $derived(variant === 'destructive' ? 'alert' : 'status');
</script>

<div
  class={cn('flex flex-col items-center justify-center gap-3 py-12 text-center', className)}
  {role}
>
  {#if icon}
    <div class="text-muted-foreground">
      {@render icon()}
    </div>
  {/if}
  <p class={slots.title()}>{title}</p>
  {#if description}
    <p class="max-w-sm text-sm text-muted-foreground">{description}</p>
  {/if}
  {#if action}
    <div class="mt-2">
      {@render action()}
    </div>
  {/if}
</div>
