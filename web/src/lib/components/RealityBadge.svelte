<script lang="ts">
  import { realityBadge } from '$lib/reality';
  import type { Reality } from '$lib/generated/contracts';
  import { cn } from '$lib/utils';

  // Surfaces the job-reality signal as a facts-backed badge: nothing for a fresh or
  // unclassified job, a muted age chip for a stale one, an amber "Likely evergreen"
  // for a converged one. It states facts (in the label and the hover title, and
  // inline when `detailed`), never a bare accusation. `detailed` renders the full
  // fact string beside the badge on the job detail page.
  let { reality, detailed = false }: { reality?: Reality | null; detailed?: boolean } = $props();

  const badge = $derived(realityBadge(reality));

  const toneClass: Record<'warn' | 'muted', string> = {
    warn: 'border-amber-500/40 bg-amber-500/10 text-amber-700 dark:text-amber-400',
    muted: 'border-border text-muted-foreground',
  };
</script>

{#if badge}
  <span class="inline-flex items-center gap-1.5">
    <span
      title={badge.facts}
      class={cn(
        'inline-flex items-center rounded-md border px-2 py-0.5 text-xs font-medium',
        toneClass[badge.tone],
      )}
    >
      {badge.label}
    </span>
    {#if detailed}
      <span class="text-xs text-muted-foreground">{badge.facts}</span>
    {/if}
  </span>
{/if}
