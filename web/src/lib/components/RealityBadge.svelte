<script lang="ts">
  import { realityBadge } from '$lib/reality';
  import type { Reality } from '$lib/generated/contracts';
  import { cn } from '$lib/ui';

  // Surfaces the job-reality signal as a facts-backed badge: nothing for a fresh or
  // unclassified job, a muted age chip for a stale one, a warning-toned "Likely evergreen"
  // for a converged one. It states facts (in the chip label and the hover tooltip, and
  // inline when `detailed`), never a bare accusation. `detailed` renders complementary
  // facts beside the chip on the job detail page — deliberately NOT the age, which the
  // chip already carries, so "Open N days" never reads twice in a row.
  //
  // Nor the posting date. It used to add a "posting dated N ago" note here, which on the
  // detail page landed on the same line as that page's own posting date and printed the
  // identical phrase twice; the contrast that note existed to draw — a long-open role whose
  // source rewrites its date every crawl — is already visible in the age chip standing
  // beside the date.
  let {
    reality,
    detailed = false,
  }: { reality?: Reality | null; detailed?: boolean } = $props();

  const badge = $derived(realityBadge(reality));
  const detail = $derived(badge?.evidence ?? '');

  const toneClass: Record<'warn' | 'muted', string> = {
    warn: 'border-warning/40 bg-warning/10 text-warning-strong',
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
