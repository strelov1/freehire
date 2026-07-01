<script lang="ts">
  import { SlidersHorizontal } from '@lucide/svelte';

  // Icon-only tab pinned to the left viewport edge on mobile, opening the filters
  // drawer — the mobile counterpart to the desktop aside panel, and a mirror of
  // the jobs list's right-edge swipe tab. Hidden at/above `md` where the aside is
  // always visible. The active-filter count rides as a corner badge. Used by the
  // top-level list pages (/jobs, /companies); embedded/scoped lists (a company's
  // jobs) keep an inline button instead, so the tab never overlaps a page hero.
  let { active = 0, onclick }: { active?: number; onclick: () => void } = $props();
</script>

<button
  type="button"
  {onclick}
  aria-label="Filters"
  title="Filters"
  class="fixed left-0 top-16 z-30 flex items-center rounded-r-lg border border-l-0 border-border bg-secondary py-2.5 pl-1.5 pr-2 text-secondary-foreground shadow-sm transition-colors hover:bg-accent md:hidden"
>
  <SlidersHorizontal class="size-4 shrink-0" />
  {#if active > 0}
    <span
      class="absolute -right-1.5 -top-1.5 flex h-4 min-w-4 items-center justify-center rounded-full bg-primary px-1 text-[10px] font-semibold leading-none text-primary-foreground"
    >
      {active}
    </span>
  {/if}
</button>
