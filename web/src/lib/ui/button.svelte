<script lang="ts" module>
  import { tv, type VariantProps } from 'tailwind-variants';

  export const buttonVariants = tv({
    base: 'inline-flex items-center justify-center gap-2 whitespace-nowrap rounded-md text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:pointer-events-none disabled:opacity-50',
    variants: {
      variant: {
        primary: 'bg-brand text-brand-foreground hover:opacity-90',
        secondary: 'bg-secondary text-secondary-foreground hover:bg-accent',
        outline: 'border border-border bg-background hover:bg-accent hover:text-accent-foreground',
        ghost: 'hover:bg-accent hover:text-accent-foreground',
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
  import { cn } from '$lib/utils';

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
    children,
    ...rest
  }: Props = $props();
</script>

{#if href}
  <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- generic link primitive; href may be external — e.g. a job's apply URL — so the caller owns resolving internal routes -->
  <a {href} class={cn(buttonVariants({ variant, size }), className)} {...rest}>
    {@render children()}
  </a>
{:else}
  <button type="button" class={cn(buttonVariants({ variant, size }), className)} {...rest}>
    {@render children()}
  </button>
{/if}
