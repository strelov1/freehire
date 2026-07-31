<script lang="ts">
  import { ghostBadge } from '$lib/ghost';
  import type { Ghost } from '$lib/generated/contracts';
  import { cn } from '$lib/ui';

  // Surfaces the ghost signal as a hedged chip plus the fired/total scale. It states
  // that a posting MAY be inactive and shows how many criteria back that, never that
  // an employer is not hiring — an intent nothing here can observe. The full
  // justification lives in GhostChecklist on the job page; the chip carries the
  // scale so a card reader can tell "2 of 4" from "4 of 4" at a glance.
  let { ghost }: { ghost?: Ghost | null } = $props();

  const badge = $derived(ghostBadge(ghost));

  const toneClass: Record<'warn' | 'muted', string> = {
    warn: 'border-amber-500/40 bg-amber-500/10 text-amber-700 dark:text-amber-400',
    muted: 'border-border text-muted-foreground',
  };
</script>

{#if badge}
  <span
    title={badge.tooltip}
    class={cn(
      'inline-flex items-center gap-1.5 rounded-md border px-2 py-0.5 text-xs font-medium',
      toneClass[badge.tone],
    )}
  >
    {badge.label}
    <span class="tabular-nums opacity-70">{badge.scale}</span>
  </span>
{/if}
