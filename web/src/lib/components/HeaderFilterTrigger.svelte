<script lang="ts">
  import { SlidersHorizontal } from '@lucide/svelte';

  // HeaderSearch's All-filters trigger. `open` is absent where no filter modal exists.
  let { open, count }: { open: (() => void) | undefined; count: number } = $props();
</script>

<!-- All-filters trigger, mirroring the Location scope-prefix on the left: divided
     from the input and pinned to the right edge. Opens the active page's own filter
     modal — or, on a page with no list, the one its host handed us. The badge shows
     the active-filter count, and only a list can have one. Hidden where neither
     exists.

     On a phone this is the ONLY way into the filters (the jobs sidebar is md+), and
     it was a bare 16px glyph. The button is now a 44px square around the same glyph,
     pulled into the box's right padding by `-mr-3` so the icon sits where it did;
     the badge rides the glyph's own wrapper so it stays pinned to the icon rather
     than to the larger box. -->
{#if open}
  <div class="h-5 w-px shrink-0 bg-border"></div>
  <button
    type="button"
    onclick={open}
    aria-label={count > 0 ? `Filters (${count} active)` : 'Filters'}
    title="Filters"
    class="-mr-3 grid size-11 shrink-0 place-items-center rounded-md text-muted-foreground transition-colors hover:text-foreground"
  >
    <span class="relative flex">
      <SlidersHorizontal class="size-4 shrink-0" aria-hidden="true" />
      {#if count > 0}
        <span
          aria-hidden="true"
          class="absolute -right-2 -top-2 flex h-4 min-w-4 items-center justify-center rounded-full bg-brand px-1 text-[10px] font-semibold leading-none text-brand-foreground"
        >
          {count}
        </span>
      {/if}
    </span>
  </button>
{/if}
