<script lang="ts">
  import { Undo2 } from '@lucide/svelte';

  // A transient "Job hidden — Undo" affordance for the feed's hide gesture. It owns
  // its own auto-dismiss timer: after `duration` it calls `onClose` unless the user
  // undoes first. Self-contained on purpose — the feed just mounts one when a card is
  // hidden (keyed by slug so each hide restarts the timer) and unmounts on close.
  let {
    onUndo,
    onClose,
    duration = 5000,
  }: { onUndo: () => void; onClose: () => void; duration?: number } = $props();

  // Start the countdown on mount; clear it if the toast unmounts (undo, a newer
  // hide replacing this one, or navigation) so a stale timer can't fire onClose.
  $effect(() => {
    const timer = setTimeout(onClose, duration);
    return () => clearTimeout(timer);
  });
</script>

<div
  role="status"
  class="fixed inset-x-0 bottom-6 z-40 flex justify-center px-4"
>
  <div
    class="flex items-center gap-3 rounded-xl border border-border bg-card px-4 py-2.5 text-sm shadow-lg"
  >
    <span class="text-foreground">Job hidden</span>
    <button
      type="button"
      onclick={() => {
        onUndo();
        onClose();
      }}
      class="inline-flex items-center gap-1.5 font-medium text-brand transition-opacity hover:opacity-80"
    >
      <Undo2 class="size-4" aria-hidden="true" />
      Undo
    </button>
  </div>
</div>
