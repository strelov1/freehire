<script lang="ts">
  import { untrack } from 'svelte';
  import { page } from '$app/state';
  import Seo from '$lib/components/Seo.svelte';
  import ActivityBars from '$lib/components/ActivityBars.svelte';
  import { api } from '$lib/api';
  import type { ActivityGranularity, ActivityPoint } from '$lib/types';
  import type { PageData } from './$types';

  let { data }: { data: PageData } = $props();

  const canonical = $derived(`${page.url.origin}/stats`);

  const options: { value: ActivityGranularity; label: string }[] = [
    { value: 'day', label: 'Day' },
    { value: 'week', label: 'Week' },
    { value: 'month', label: 'Month' },
  ];

  // The server renders the daily series; the client swaps in a coarser one on
  // toggle. `loading` dims the chart during the re-fetch; `failed` shows a retry.
  let granularity = $state<ActivityGranularity>('day');
  // Seeded from the SSR series (untracked — we only want its initial value), then
  // replaced wholesale on toggle, so $state.raw is the right, cheaper choice.
  let points = $state.raw<ActivityPoint[]>(untrack(() => data.initial));
  let loading = $state(false);
  let failed = $state(false);

  async function load(next: ActivityGranularity) {
    granularity = next;
    loading = true;
    failed = false;
    try {
      points = await api.jobsActivity(next);
    } catch {
      failed = true;
    } finally {
      loading = false;
    }
  }

  function select(next: ActivityGranularity) {
    if (next !== granularity) load(next);
  }
</script>

<Seo
  title="Job activity · freehire"
  description="How the freehire catalogue moves over time — new vacancies added versus postings removed, by day, week, or month."
  {canonical}
/>

<div class="mx-auto w-full max-w-6xl px-4 py-6">
  <div class="mb-4 flex flex-wrap items-end justify-between gap-3">
    <div>
      <h1 class="text-xl font-semibold tracking-tight">Job activity</h1>
      <p class="mt-1 text-sm text-muted-foreground">
        New vacancies added versus postings removed over time.
      </p>
    </div>

    <div class="inline-flex rounded-md border border-border p-0.5" role="group" aria-label="Granularity">
      {#each options as option (option.value)}
        <button
          type="button"
          onclick={() => select(option.value)}
          aria-pressed={granularity === option.value}
          class="rounded px-3 py-1 text-sm transition-colors {granularity === option.value
            ? 'bg-muted font-medium text-foreground'
            : 'text-muted-foreground hover:text-foreground'}"
        >
          {option.label}
        </button>
      {/each}
    </div>
  </div>

  {#if failed}
    <div class="py-16 text-center text-sm text-muted-foreground">
      Couldn't load activity.
      <button type="button" onclick={() => load(granularity)} class="ml-1 underline hover:text-foreground">
        Retry
      </button>
    </div>
  {:else}
    <div class="rounded-lg border border-border p-4 transition-opacity {loading ? 'opacity-50' : ''}">
      <ActivityBars {points} />
    </div>
  {/if}
</div>
