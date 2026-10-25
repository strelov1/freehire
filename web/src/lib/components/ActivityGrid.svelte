<script lang="ts">
  import { onMount } from 'svelte';
  import { resolve } from '$app/paths';
  import { api } from '#lib/api.js';
  import { buildActivityGrid, rangeForWindow } from '#lib/activityGrid.js';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { t } from '#lib/i18n/t.js';
  import type { TimelineEvent } from '#lib/types.js';
  import { Button, Card } from '#lib/ui/index.js';
  import { messages } from './ActivityGrid.messages';
  import ActivityYear from './ActivityYear.svelte';
  import ApplicationEventList from './ApplicationEventList.svelte';
  import States from './States.svelte';

  // A year of the caller's own job-search actions, drawn the way a contribution graph is.
  //
  // The server load hands over the window fetched in ITS timezone; which square an event
  // lands on is decided here, because only the browser knows the reader's clock. See
  // activityGrid.ts — the arithmetic, the counting rule and the streaks all live there —
  // and ActivityYear, which draws the model. This component owns the data and the copy.
  let { prefetched }: { prefetched: TimelineEvent[] | undefined } = $props();

  const s = $derived(t(messages, locale()));

  // The prop is a fallback, not a seed: copying it into state freezes it at its first value.
  // Once the client has fetched a window of its own, that is what the grid reads.
  let fetched = $state<TimelineEvent[] | null>(null);
  const series = $derived(fetched ?? prefetched ?? []);
  let phase = $state<'initial' | 'loading' | 'ready' | 'error'>('initial');
  // 'initial' means nothing has been asked for yet: usable when the server load answered,
  // waiting on the mount fetch otherwise. Reading the prop here rather than in an initialiser
  // is what keeps it live instead of frozen at its first value.
  const status = $derived.by(() => {
    if (phase !== 'initial') return phase;
    return prefetched ? 'ready' : 'loading';
  });

  // One fetch on mount, and only when the server load did not answer. Not reactive: nothing
  // moves the window, so nothing re-fetches it.
  onMount(() => {
    if (!prefetched) void load();
  });

  async function load() {
    phase = 'loading';
    try {
      const { from, to } = rangeForWindow();
      fetched = await api.myTimeline(from, to);
      phase = 'ready';
    } catch {
      phase = 'error';
    }
  }

  const grid = $derived(buildActivityGrid(series));
</script>

<div class="flex flex-col gap-4">
  {#if status === 'error'}
    <States state="error" message={s.loadError} />
  {:else if status === 'loading'}
    <States state="loading" rows={3} />
  {:else}
    <ActivityYear {grid} heading={s.heading} caption={s.caption}>
      <!-- Assembled from the events already fetched for the window. Selecting a day issues no
           request — and it lists EVERY event of that day, including the ones that do not shade
           the square, because what a square measures is effort and what a day held is history. -->
      {#snippet panel(selected)}
        {#if selected.events.length === 0}
          <p class="text-sm text-muted-foreground">{s.panelNothing}</p>
        {:else}
          <ApplicationEventList events={selected.events} />
        {/if}
      {/snippet}
    </ActivityYear>

    {#if grid.total === 0}
      <Card class="p-4">
        <p class="text-sm text-muted-foreground">{s.empty}</p>
        <Button variant="outline" size="sm" class="mt-3" href={resolve('/my/tracking')}>{s.emptyCta}</Button>
      </Card>
    {/if}
  {/if}
</div>
