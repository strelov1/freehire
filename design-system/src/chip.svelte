<script lang="ts" module>
  import { tv, type VariantProps } from 'tailwind-variants';

  export const chipVariants = tv({
    base: 'inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-medium transition-colors',
    variants: {
      variant: {
        default: 'border-border bg-muted text-foreground',
        primary: 'border-transparent bg-primary text-primary-foreground',
        secondary: 'border-transparent bg-secondary text-secondary-foreground',
        brand: 'border-transparent bg-brand-muted text-brand-strong',
        destructive: 'border-destructive/20 bg-destructive/10 text-destructive',
      },
    },
    defaultVariants: { variant: 'default' },
  });

  export type ChipVariant = VariantProps<typeof chipVariants>['variant'];
</script>

<script lang="ts">
  import type { Snippet } from 'svelte';
  import { cn } from './cn.js';

  let {
    variant = 'default',
    class: className,
    children,
  }: { variant?: ChipVariant; class?: string; children: Snippet } = $props();
</script>

<span class={cn(chipVariants({ variant }), className)}>
  {@render children()}
</span>
