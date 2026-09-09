<script lang="ts">
  import { Bot } from '@lucide/svelte';
  import { cn } from '$lib/ui';

  // States that candidates have reported this employer screening with an AI
  // interviewer, and how many said so.
  //
  // The styling is deliberately neutral — the muted border GhostBadge uses for its
  // unremarkable tone, never the warning one. Plenty of people prefer an AI screen:
  // it is fast, it runs outside office hours, and it does not react to a gap in a CV.
  // The badge exists so somebody can recognise the practice before they meet it, not
  // so the platform can rule on it, and a neutral label that is trusted is worth more
  // than a warning that gets discounted.
  //
  // The count is not decoration. One report is enough to show the label, so a reader
  // is entitled to weigh `1` differently from `40` — a bare badge would hide exactly
  // that difference. Which is why an absent count renders nothing at all rather than
  // a badge with no number beside it.
  let { count, class: className = '' }: { count?: number | null; class?: string } = $props();

  const shown = $derived(typeof count === 'number' && count > 0 ? count : null);
</script>

{#if shown !== null}
  <span
    title={`${shown} ${shown === 1 ? 'person reports' : 'people report'} this employer interviews with AI`}
    class={cn(
      'inline-flex items-center gap-1.5 rounded-md border border-border px-2 py-0.5 text-xs font-medium text-muted-foreground',
      className,
    )}
  >
    <Bot class="size-3.5 shrink-0" aria-hidden="true" />
    AI interview
    <span class="tabular-nums opacity-70">{shown}</span>
  </span>
{/if}
