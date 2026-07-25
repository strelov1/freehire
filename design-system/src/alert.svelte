<script lang="ts" module>
  import { tv, type VariantProps } from 'tailwind-variants';

  export const alertVariants = tv({
    base: 'flex items-start gap-3 rounded-lg border p-4 text-sm',
    variants: {
      variant: {
        default: 'border-border bg-card text-foreground',
        destructive: 'border-destructive/30 bg-destructive/5 text-destructive',
        brand: 'border-brand/30 bg-brand-muted text-brand-strong',
      },
    },
    defaultVariants: { variant: 'default' },
  });

  export type AlertVariant = VariantProps<typeof alertVariants>['variant'];
</script>

<script lang="ts">
  import type { Snippet } from 'svelte';
  import { cn } from './cn.js';

  let {
    variant = 'default',
    class: className,
    children,
  }: { variant?: AlertVariant; class?: string; children: Snippet } = $props();

  // role="alert" is an assertive live region: it interrupts whatever the screen
  // reader is saying. Right for a failure the user needs now, wrong for the
  // informational variants, which are usually static page furniture and would
  // barge in on every render.
  let role = $derived(variant === 'destructive' ? 'alert' : undefined);
</script>

<div {role} class={cn(alertVariants({ variant }), className)}>
  {@render children()}
</div>
