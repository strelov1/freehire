<script lang="ts">
  import type { Snippet } from 'svelte';
  import { cn } from '#lib/ui/index.js';

  // The header search's one-sentence panel, in place of the suggestion list: a recognised
  // link, or an operator value that names nothing.
  let { hero, children }: { hero: boolean; children: Snippet } = $props();

  /** The panel a recognised link drops.
   *
   *  It does NOT take the phone's screen the way the suggestions list does. That panel
   *  goes full-bleed because it is a dozen rows that truncate at the box's width; this
   *  one is a sentence, and a sentence stretched from the header to the bottom of the
   *  screen reads as a page that has gone wrong. */
  const panelClass = $derived(
    cn(
      'absolute inset-x-0 top-full z-50 mt-2 border border-border bg-background px-3 py-2.5 text-sm shadow-lg',
      hero ? 'rounded-2xl' : 'rounded-md',
    ),
  );
</script>

<div class={panelClass} role="status" aria-live="polite">
  {@render children()}
</div>
