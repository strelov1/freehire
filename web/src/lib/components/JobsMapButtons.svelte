<script lang="ts">
  import { Layers, Map as MapIcon } from '@lucide/svelte';
  import { Button } from '#lib/ui/index.js';

  // The feed toolbar's leading buttons: the map, and swipe mode on desktop. The map is
  // off while a personal filter it cannot draw (Hiring again, the watchlist) is on.
  let {
    blockedBy,
    onMap,
    onSwipe,
  }: { blockedBy: 'formerEmployers' | 'watchlist' | null; onMap: () => void; onSwipe: (() => void) | undefined } =
    $props();
</script>

<Button
  variant="outline"
  size="sm"
  disabled={blockedBy !== null}
  title={blockedBy === 'formerEmployers'
    ? 'Turn off Hiring again to explore the map'
    : blockedBy === 'watchlist'
      ? 'Turn off Your watchlist to explore the map'
      : undefined}
  aria-label="Map"
  onclick={onMap}
>
  <MapIcon class="size-4" />
  <span class="hidden sm:inline">Map</span>
</Button>
<!-- Desktop swipe-mode entry, beside Map in the toolbar: as a tab pinned to the viewport
     edge it sat half outside the content column, aligned with nothing. Phones keep
     their own entry in the inline toolbar. -->
{#if onSwipe}
  <Button
    variant="outline"
    size="sm"
    onclick={onSwipe}
    aria-label="Swipe mode"
    title="Swipe mode"
    class="hidden md:inline-flex"
  >
    <Layers class="size-4" />
    <span class="hidden lg:inline">Swipe</span>
  </Button>
{/if}
