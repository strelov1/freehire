<script lang="ts">
  import { Button } from 'freehire-design-system';
  import { RectangleEllipsis, Square } from '@lucide/svelte';

  // The Match tab's pinned Autofill button, which becomes Stop while a walk runs.
  let {
    autofilling,
    walkStop = $bindable(),
    onAutofill,
  }: { autofilling: boolean; walkStop: boolean; onAutofill: () => void } = $props();
</script>

<div class="match-footer">
  {#if autofilling}
    <!-- The same button, not a second one beside it: while a walk runs
         there is exactly one thing to do with it, and a disabled
         "Filling…" left the user watching something they could not
         call off. -->
    <Button class="w-full" variant="outline" size="lg" onclick={() => (walkStop = true)} disabled={walkStop}>
      {walkStop ? 'Stopping…' : 'Stop'}
      <Square class="size-4" />
    </Button>
  {:else}
    <Button class="w-full" variant="primary" size="lg" onclick={onAutofill}>
      Autofill
      <RectangleEllipsis class="size-4" />
    </Button>
  {/if}
</div>

<style>
  .match-footer {
    flex-shrink: 0;
    padding: 10px 12px;
    border-top: 1px solid var(--border);
  }
</style>
