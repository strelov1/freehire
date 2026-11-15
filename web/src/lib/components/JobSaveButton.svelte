<script lang="ts">
  import { Bookmark } from '@lucide/svelte';
  import { Button } from '#lib/ui/index.js';

  let {
    saved,
    onclick,
    className = '',
    iconOnly = false,
  }: { saved: boolean; onclick: () => void; className?: string; iconOnly?: boolean } = $props();

  // The button's accessible name, where its visible text does not already state the effect.
  const ariaLabel = $derived(saved ? 'Remove from your board' : iconOnly ? 'Save to board' : undefined);
</script>

<!-- Save, a quiet action rather than the full-width button it was in the sidebar: keeping a
     posting is something a reader does in passing, not the thing the page is for. It sits
     with the other quiet ones on the tab row, and beside the apply CTA in the pinned header,
     which has room for one bookmark and not for four. The filled bookmark is the state.
     `iconOnly` keeps the pinned header and mobile action strip compact.
     The words are JobRow's ("Save to board" / "On your board"), so one action reads the same
     on the card and on the page, and so is the accessible-name rule: the visible label
     already flips with the state, so there is no `aria-pressed` announcing the same fact a
     second time in another vocabulary. Only the saved state overrides the name, because "On
     your board" is a state and leaves a screen reader with a button whose effect is
     unstated. The icon-only copy has no visible label, so it names both states itself. -->
<Button
  variant="ghost"
  size="sm"
  {onclick}
  aria-label={ariaLabel}
  title={saved ? 'On your board' : 'Save to board'}
  class={`shrink-0 gap-1.5 px-2 ${saved ? 'text-brand hover:text-brand' : 'text-muted-foreground'} ${className}`}
>
  <Bookmark class="size-4 shrink-0 {saved ? 'fill-current' : ''}" aria-hidden="true" />
  {#if !iconOnly}{saved ? 'On your board' : 'Save to board'}{/if}
</Button>
