<script lang="ts">
  import { Mail } from '@lucide/svelte';
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
    {#if item.email_count > 0}
      <span
        class="flex items-center gap-0.5 text-xs tabular-nums text-muted-foreground"
        title="{item.email_count} linked email{item.email_count === 1 ? '' : 's'}"
        aria-label="{item.email_count} linked email{item.email_count === 1 ? '' : 's'}"
      >
        <Mail class="size-3 shrink-0" aria-hidden="true" />
        {item.email_count}
      </span>
    {/if}
    {#if hasNotes}
      <svg
        class="size-3 shrink-0 text-muted-foreground"
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        stroke-width="2"
        stroke-linecap="round"
        stroke-linejoin="round"
        role="img"
        aria-label="Has notes"
      >
        <title>Has notes</title>
        <path d="M8 7h8M8 12h8M8 17h5" />
      </svg>
    {/if}
  </span>
</button>
