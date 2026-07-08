<script lang="ts">
  import { realityBadge, postingContrast } from '$lib/reality';
  import type { Reality } from '$lib/generated/contracts';
  import { cn } from '$lib/utils';

  // Surfaces the job-reality signal as a facts-backed badge: nothing for a fresh or
  // unclassified job, a muted age chip for a stale one, an amber "Likely evergreen"
  // for a converged one. It states facts (in the chip label and the hover tooltip, and
  // inline when `detailed`), never a bare accusation. `detailed` renders complementary
  // facts beside the chip on the job detail page — deliberately NOT the age, which the
  // chip already carries, so "Open N days" never reads twice in a row.
  let {
    reality,
    postedAt = null,
    detailed = false,
  }: { reality?: Reality | null; postedAt?: string | null; detailed?: boolean } = $props();

  const badge = $derived(realityBadge(reality));
  // The posting-date contrast (when the source's date reads fresher than the true age)
  // followed by the remaining evidence; empty when neither applies (chip shows alone).
  const detail = $derived(
    badge && reality
      ? [postingContrast(reality, postedAt), badge.evidence].filter(Boolean).join(' · ')
      : '',
  );

  const toneClass: Record<'warn' | 'muted', string> = {
    warn: 'border-amber-500/40 bg-amber-500/10 text-amber-700 dark:text-amber-400',
    muted: 'border-border text-muted-foreground',
  };
</script>

{#if badge}
  <span class="inline-flex items-center gap-1.5">
    <span
      title={badge.tooltip}
      class={cn(
        'inline-flex items-center rounded-md border px-2 py-0.5 text-xs font-medium',
        toneClass[badge.tone],
      )}
    >
      {badge.label}
    </span>
    {#if detailed && detail}
      <span class="text-xs text-muted-foreground">{detail}</span>
    {/if}
  </span>
{/if}
