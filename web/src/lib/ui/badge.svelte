<script lang="ts" module>
  import { tv, type VariantProps } from 'tailwind-variants';

  export const badgeVariants = tv({
    base: 'inline-flex items-center rounded-md border px-2 py-0.5 text-xs font-medium',
    variants: {
      variant: {
        secondary: 'border-transparent bg-secondary text-secondary-foreground',
        outline: 'border-border text-muted-foreground',
        brand: 'border-transparent bg-brand-muted text-brand-strong',
      },
    },
    defaultVariants: { variant: 'secondary' },
  });

  export type BadgeVariant = VariantProps<typeof badgeVariants>['variant'];
</script>

<script lang="ts">
  import type { Snippet } from 'svelte';
  import { cn } from '$lib/utils';

  let {
    variant = 'secondary',
    class: className,
    children,
  }: { variant?: BadgeVariant; class?: string; children: Snippet } = $props();
</script>

<span class={cn(badgeVariants({ variant }), className)}>
  {@render children()}
</span>
