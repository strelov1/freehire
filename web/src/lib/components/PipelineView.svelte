<script lang="ts">
  import { getMyPipeline } from '$lib/api';
  import { isAuthenticated } from '$lib/auth.svelte';
  import { interviewRate, offerRate } from '$lib/pipeline';
  import type { PipelineStats } from '$lib/types';
  import PipelineFunnel from './PipelineFunnel.svelte';
  import RateDonut from './RateDonut.svelte';
  import States from './States.svelte';

  // Single aggregate fetch (not paginated): the snapshot of where the caller's
  // applications stand. Reassigned wholesale, never mutated → $state.raw.
  let status = $state<'loading' | 'error' | 'ready'>('loading');
  let stats = $state.raw<PipelineStats | null>(null);

  async function load() {
    status = 'loading';
    try {
      stats = await getMyPipeline();
      status = 'ready';
    } catch {
      status = 'error';
    }
  }

  $effect(() => {
    if (isAuthenticated()) void load();
  });

  const iv = $derived(stats ? interviewRate(stats) : 0);
  const offer = $derived(stats ? offerRate(stats) : 0);
</script>

{#if status === 'loading'}
  <States state="loading" rows={3} />
{:else if status === 'error'}
  <States state="error" message="Couldn't load your pipeline." />
{:else if !stats || stats.applications === 0}
  <States state="empty" message="You haven't applied to any jobs yet. Applications you track will show up here." />
{:else}
  <div class="flex flex-col gap-3">
    <div class="rounded-lg border bg-card p-5">
      <!-- Rate donuts ride on top as a two-up row; the funnel below spans the full
           card width. -->
      <div class="mb-5 flex flex-wrap items-start justify-between gap-4">
        <p class="text-sm text-muted-foreground">
          {stats.applications} application{stats.applications === 1 ? '' : 's'}
        </p>
        <div class="flex shrink-0 gap-6">
          <RateDonut percent={iv} label="Interview Rate" sublabel="reached interview" />
          <RateDonut percent={offer} label="Offer Rate" sublabel="reached offer" />
        </div>
      </div>
      <PipelineFunnel applications={stats.applications} buckets={stats.buckets} />
    </div>
    <p class="text-xs text-muted-foreground">
      A snapshot of where your applications stand now. Rates are a lower bound — a job rejected after
      an interview counts only as rejected.
    </p>
  </div>
{/if}
