<script lang="ts">
  import CompanyLogo from './CompanyLogo.svelte';
  import { Badge } from '$lib/ui';
  import { humanizeStage } from '$lib/stages';
  import type { MyJob } from '$lib/types';

  let { item, onopen }: { item: MyJob; onopen: (item: MyJob) => void } = $props();

  const hasNotes = $derived(!!item.notes && item.notes.trim().length > 0);
</script>

<button
  type="button"
  onclick={() => onopen(item)}
  class="flex w-full flex-col gap-1.5 rounded-lg border border-border bg-card p-3 text-left shadow-sm transition-colors hover:bg-accent"
>
  <span class="flex items-center gap-1.5 text-sm font-semibold">
    <CompanyLogo name={item.job.company} />
    <span class="min-w-0 truncate">{item.job.company || 'Unknown company'}</span>
  </span>
  <span class="line-clamp-2 text-sm">{item.job.title}</span>
  <span class="flex items-center gap-1.5">
    {#if item.stage}
      <Badge variant="secondary">{humanizeStage(item.stage)}</Badge>
    {/if}
    {#if hasNotes}
      <span class="text-xs text-muted-foreground" title="Has notes" aria-label="Has notes">📝</span>
    {/if}
  </span>
</button>
