<script lang="ts">
  // The on/off switch every settings surface uses. Extracted when it reached a third
  // copy — the notification card, the public unsubscribe page, and the email-group
  // card — because three hand-maintained copies of a control is how one of them ends
  // up a few pixels or one aria attribute away from the others.
  import { cn } from '$lib/ui';

  let {
    on,
    label,
    disabled = false,
    onToggle,
  }: {
    on: boolean;
    /** The accessible name. A switch with no label reads as an unnamed button. */
    label: string;
    disabled?: boolean;
    onToggle: () => void;
  } = $props();
</script>

<button
  type="button"
  role="switch"
  aria-checked={on}
  aria-label={label}
  {disabled}
  onclick={onToggle}
  class={cn(
    'relative inline-flex h-6 w-11 shrink-0 items-center rounded-full transition-colors disabled:opacity-50',
    'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-ring focus-visible:ring-offset-2 focus-visible:ring-offset-card',
    on ? 'bg-brand' : 'bg-muted',
  )}
>
  <span
    class={cn(
      'inline-block size-5 rounded-full bg-white shadow-sm transition-transform',
      on ? 'translate-x-[22px]' : 'translate-x-0.5',
    )}
  ></span>
</button>
