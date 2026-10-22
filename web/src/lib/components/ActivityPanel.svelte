<script lang="ts">
  import type { Snippet } from 'svelte';
  import { untrack } from 'svelte';
  import ActivityBars from './ActivityBars.svelte';
  import type { ActivityGranularity, ActivityPoint } from '#lib/types.js';

  // The added-vs-removed chart with its Day/Week/Month toggle — the catalogue's on
  // /trends, one company's on its Hiring tab. The server renders the daily series; the
  // toggle swaps in a coarser one through `load`. `title` fills the left of the toggle
  // row, so a page can keep its heading beside the control.
  let {
    initial,
    load,
    title,
  }: {
    initial: ActivityPoint[];
    load: (granularity: ActivityGranularity) => Promise<ActivityPoint[]>;
    title?: Snippet;
  } = $props();

  const options: { value: ActivityGranularity; label: string }[] = [
    { value: 'day', label: 'Day' },
    { value: 'week', label: 'Week' },
    { value: 'month', label: 'Month' },
  ];

  // `loading` dims the chart during the re-fetch; `failed` shows a retry.
  let granularity = $state<ActivityGranularity>('day');
  // Seeded from the SSR series (untracked — we only want its initial value), then
  // replaced wholesale on toggle, so $state.raw is the right, cheaper choice.
  let points = $state.raw<ActivityPoint[]>(untrack(() => initial));
  let loading = $state(false);
  let failed = $state(false);

  // Sequence toggles: a fast Day→Week→Month burst fires overlapping fetches, and
  // an earlier one can resolve last. Only the latest request may touch state, so a
  // stale response can't overwrite the chart the active toggle promises.
  let latestRequest = 0;

  async function fetchSeries(next: ActivityGranularity) {
    granularity = next;
    const request = ++latestRequest;
    loading = true;
    failed = false;
    try {
      const result = await load(next);
      if (request !== latestRequest) return;
      points = result;
    } catch {
      if (request !== latestRequest) return;
      failed = true;
    } finally {
      if (request === latestRequest) loading = false;
    }
  }

  // Reload on a genuine change, or when re-clicking the active tab after a failure
  // (so the toggle itself retries, not only the separate Retry link).
  function select(next: ActivityGranularity) {
    if (next !== granularity || failed) fetchSeries(next);
  }
</script>

<div class="mb-4 flex flex-wrap items-end justify-between gap-3">
  <div>{@render title?.()}</div>

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
    <button type="button" onclick={() => fetchSeries(granularity)} class="ml-1 underline hover:text-foreground">
      Retry
    </button>
  </div>
{:else}
  <div class="rounded-lg border border-border p-4 transition-opacity {loading ? 'opacity-50' : ''}">
    <ActivityBars {points} />
  </div>
{/if}
