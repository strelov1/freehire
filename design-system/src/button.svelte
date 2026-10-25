<script lang="ts" module>
  import { tv, type VariantProps } from 'tailwind-variants';

  export const buttonVariants = tv({
    base: 'inline-flex items-center justify-center gap-2 whitespace-nowrap rounded-md text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background disabled:pointer-events-none disabled:opacity-50',
    variants: {
      variant: {
        primary: 'bg-brand text-brand-foreground hover:opacity-90',
        // The page's one call to action (create the account, show the results, pay,
        // tailor the CV on a job page): the brand fill framed by a soft ring of itself
        // with a gap of page between. Focus keeps its own state by thickening the ring to
        // the solid focus colour. One per view; the ring is decoration, not the indicator.
        cta: 'bg-brand text-brand-foreground ring-2 ring-brand/30 ring-offset-2 ring-offset-background hover:opacity-90 focus-visible:ring-4 focus-visible:ring-ring',
        secondary: 'bg-secondary text-secondary-foreground hover:bg-accent',
        outline: 'border border-border bg-background hover:bg-accent hover:text-accent-foreground',
        ghost: 'hover:bg-accent hover:text-accent-foreground',
        // Filled, for the action that destroys something and cannot be undone —
        // account deletion, not every remove button. A soft `ghost` + destructive
        // text is the right weight for the reversible ones.
        destructive: 'bg-destructive text-destructive-foreground hover:opacity-90',
      },
      size: {
        sm: 'h-8 px-3',
        md: 'h-9 px-4',
        lg: 'h-11 px-6',
        icon: 'size-8',
      },
    },
    defaultVariants: { variant: 'secondary', size: 'md' },
  });

  export type ButtonVariant = VariantProps<typeof buttonVariants>['variant'];
  export type ButtonSize = VariantProps<typeof buttonVariants>['size'];
</script>

<script lang="ts">
  import type { Snippet } from 'svelte';
  import type { HTMLAnchorAttributes, HTMLButtonAttributes } from 'svelte/elements';
  import { cn } from './cn.js';

  type Props = {
    variant?: ButtonVariant;
    size?: ButtonSize;
    class?: string;
    href?: string;
    children: Snippet;
  } & HTMLButtonAttributes &
    HTMLAnchorAttributes;

  let {
    variant = 'secondary',
    size = 'md',
    class: className,
    href,
    target,
    rel,
    children,
    ...rest
  }: Props = $props();

  // A target="_blank" anchor keeps a window.opener handle back to this page unless
  // rel says otherwise — the reverse-tabnabbing gap. Fill in the safe default once,
  // here, rather than relying on every call site to remember it; an explicit `rel`
  // from the caller still wins.
  const effectiveRel = $derived(target === '_blank' ? (rel ?? 'noopener noreferrer') : rel);
</script>

{#if href}
  <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- generic link primitive; href may be external — e.g. a job's apply URL — so the caller owns resolving internal routes -->
  <a {href} {target} rel={effectiveRel} class={cn(buttonVariants({ variant, size }), className)} {...rest}>
    {@render children()}
  </a>
{:else}
  <button type="button" class={cn(buttonVariants({ variant, size }), className)} {...rest}>
    {@render children()}
  </button>
{/if}
