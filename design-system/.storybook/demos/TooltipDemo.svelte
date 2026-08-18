<script lang="ts">
  // The tooltip wires aria-describedby onto the first focusable element inside
  // its trigger snippet, so the trigger has to be a real control for the story
  // to show the keyboard path (focus reveals it, Escape dismisses it).
  import { Info } from '@lucide/svelte';
  import Button from '../../src/button.svelte';
  import Tooltip from '../../src/tooltip.svelte';

  let {
    label = 'Posted 4 months ago and still open',
    side = 'top',
    // A small icon-only trigger, not the full Button — the shape that exposed
    // the shrink-to-fit width bug (see tooltip.svelte's `w-max` comment): a
    // narrow trigger is a narrow containing block, and content collapsed to
    // one word per line no matter how generous max-w-xs was.
    narrowTrigger = false,
  }: {
    label?: string;
    side?: 'top' | 'right' | 'bottom' | 'left';
    narrowTrigger?: boolean;
  } = $props();
</script>

<!-- Room on every side so the story reads the same whichever `side` is picked. -->
<div class="flex justify-center p-16">
  <Tooltip {side}>
    {#snippet content()}
      {label}
    {/snippet}
    {#if narrowTrigger}
      <button
        type="button"
        aria-label="More info"
        class="flex size-4 items-center justify-center rounded-full text-muted-foreground"
      >
        <Info class="size-3.5" aria-hidden="true" />
      </button>
    {:else}
      <Button variant="outline">Hover or focus me</Button>
    {/if}
  </Tooltip>
</div>
