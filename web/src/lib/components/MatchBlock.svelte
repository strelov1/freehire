<script lang="ts">
  import type { FeedCard } from '#lib/types.js';

  // A feed card's explanation, as the server computed it (AD-4): rendered, never recomputed.
  // Unknown conditions read as unknown, never as met.
  let { match }: { match: FeedCard['match'] } = $props();

  const UNKNOWN_LABEL: Record<string, string> = {
    salary: 'salary',
    work_mode: 'remote or office',
    seniority: 'level',
  };
  const MAX = 5;
  const more = (n: number) => (n > MAX ? ` +${n - MAX}` : '');
</script>

<dl class="grid w-full gap-1 rounded-lg bg-muted/50 px-3 py-2 text-sm leading-snug">
  {#if match.why.length}
    <div class="flex gap-1.5">
      <dt class="shrink-0 font-medium text-brand">Fits</dt>
      <dd class="min-w-0">{match.why.slice(0, MAX).join(', ')}{more(match.why.length)}</dd>
    </div>
  {/if}
  {#if match.may_not_fit.length}
    <div class="flex gap-1.5">
      <dt class="shrink-0 font-medium text-warning-strong">Missing</dt>
      <dd class="min-w-0 text-muted-foreground">
        {match.may_not_fit.slice(0, MAX).join(', ')}{more(match.may_not_fit.length)}
      </dd>
    </div>
  {/if}
  {#if match.unknown.length}
    <div class="flex gap-1.5">
      <dt class="shrink-0 font-medium text-muted-foreground">Not stated</dt>
      <dd class="min-w-0 text-muted-foreground">{match.unknown.map((u) => UNKNOWN_LABEL[u] ?? u).join(', ')}</dd>
    </div>
  {/if}
</dl>
