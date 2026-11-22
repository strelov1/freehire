<script lang="ts">
  // A transient message over the match feed. No role of its own: the voice is `announce()`,
  // a second live region here would read it twice. The timer pauses while the pointer or
  // focus is on the toast and while the tab is hidden, and resumes with the time left.
  // The parent keys it so a newer toast replaces the older one and restarts the clock.
  let {
    message,
    onClose,
    duration = 5000,
    action,
    class: cls = '',
  }: {
    message: string;
    onClose: () => void;
    duration?: number;
    action?: { label: string; onClick: () => void };
    class?: string;
  } = $props();

  let hovered = $state(false);
  let focused = $state(false);
  let hidden = $state(typeof document !== 'undefined' && document.hidden);
  // The clock starts from the initial duration.
  // svelte-ignore state_referenced_locally
  let left = duration;

  $effect(() => {
    if (hovered || focused || hidden) return;
    const started = Date.now();
    const timer = setTimeout(onClose, left);
    return () => {
      clearTimeout(timer);
      left = Math.max(0, left - (Date.now() - started));
    };
  });
</script>

<svelte:document onvisibilitychange={() => (hidden = document.hidden)} />

<div class="pointer-events-none fixed inset-x-0 bottom-safe z-60 flex justify-center px-4 {cls}">
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div
    class="pointer-events-auto rounded-xl border border-border bg-card px-4 py-2.5 text-sm text-foreground shadow-lg"
    onmouseenter={() => (hovered = true)}
    onmouseleave={() => (hovered = false)}
    onfocusin={() => (focused = true)}
    onfocusout={() => (focused = false)}
  >
    {message}
    {#if action}
      <button
        type="button"
        class="ml-3 font-semibold text-brand underline-offset-2 hover:underline"
        onclick={() => {
          action.onClick();
          onClose();
        }}
      >
        {action.label}
      </button>
    {/if}
  </div>
</div>
