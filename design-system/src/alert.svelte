<script lang="ts" module>
  import { tv, type VariantProps } from 'tailwind-variants';

  export const alertVariants = tv({
    base: 'flex items-start gap-3 rounded-lg border p-4 text-sm',
    variants: {
      variant: {
        default: 'border-border bg-card text-foreground',
        destructive: 'border-destructive/30 bg-destructive/5 text-destructive-strong',
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
    live = true,
    class: className,
    children,
  }: { variant?: AlertVariant; live?: boolean; class?: string; children: Snippet } = $props();

  // role="alert" is an assertive live region: it interrupts whatever the screen
  // reader is saying. Right for a failure the user needs now, wrong for the
  // informational variants, which are usually static page furniture and would
  // barge in on every render. `live={false}` is for a caller that already voices
  // the failure through a shared announcer: the box stays readable in browse mode,
  // it just does not speak a second time.
  let role = $derived(variant === 'destructive' && live ? 'alert' : undefined);
</script>

<div {role} class={cn(alertVariants({ variant }), className)}>
  {@render children()}
</div>
