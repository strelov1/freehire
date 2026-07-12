<script lang="ts">
  import { SlidersHorizontal } from '@lucide/svelte';
  import { cn } from '$lib/utils';

  // Icon-only tab pinned to a viewport edge on mobile, opening the filters drawer
  // — the mobile counterpart to the desktop aside panel, and a mirror of the jobs
  // list's right-edge swipe tab. Hidden at/above `md` where the aside is always
  // visible. The active-filter count rides as a corner badge. `side` picks the
  // edge (default left); pages that carry a tab strip below the header (e.g. the
  // account profile) pass `side="right"` + a lower `top-*` via `class` so the tab
  // sits level with the strip instead of overlapping it. Used by the top-level
  // list pages (/jobs, /companies); embedded/scoped lists (a company's jobs) keep
  // an inline button instead, so the tab never overlaps a page hero.
  let {
    active = 0,
    onclick,
    side = 'left',
    class: extra = '',
  }: {
    active?: number;
    onclick: () => void;
    side?: 'left' | 'right';
    class?: string;
  } = $props();

  const sideClass = $derived(
    side === 'right'
      ? 'right-0 rounded-l-lg border-r-0 pl-2 pr-1.5'
      : 'left-0 rounded-r-lg border-l-0 pl-1.5 pr-2',
  );
</script>

<button
  type="button"
  {onclick}
  aria-label="Filters"
  title="Filters"
  class={cn(
    'fixed top-16 z-30 flex items-center border border-border bg-secondary py-2.5 text-secondary-foreground shadow-sm transition-colors hover:bg-accent md:hidden',
    sideClass,
    extra,
  )}
>
  <SlidersHorizontal class="size-4 shrink-0" />
  {#if active > 0}
    <span
      class={cn(
        'absolute -top-1.5 flex h-4 min-w-4 items-center justify-center rounded-full bg-brand px-1 text-[10px] font-semibold leading-none text-brand-foreground',
        side === 'right' ? '-left-1.5' : '-right-1.5',
      )}
    >
      {active}
    </span>
  {/if}
</button>
