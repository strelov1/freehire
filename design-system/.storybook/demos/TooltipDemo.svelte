<script lang="ts">
  // The tooltip wires aria-describedby onto the first focusable element inside
  // its trigger snippet, so the trigger has to be a real control for the story
  // to show the keyboard path (focus reveals it, Escape dismisses it).
  import Button from '../../src/button.svelte';
  import Tooltip from '../../src/tooltip.svelte';

  let {
    label = 'Posted 4 months ago and still open',
    side = 'top',
  }: { label?: string; side?: 'top' | 'right' | 'bottom' | 'left' } = $props();
</script>

<!-- Room on every side so the story reads the same whichever `side` is picked. -->
<div class="flex justify-center p-16">
  <Tooltip {side}>
    {#snippet content()}
      {label}
    {/snippet}
    <Button variant="outline">Hover or focus me</Button>
  </Tooltip>
</div>
