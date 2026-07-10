<script lang="ts">
  import { buildActivityChart } from '$lib/activityChart';
  import type { ActivityPoint } from '$lib/types';

  // A grouped bar chart of catalogue flow: per period, a green "added" bar and a
  // red "removed" bar. Hand-built SVG scaled to its container width — no charting
  // dependency, matching PipelineFunnel/RateDonut. All geometry comes from the
  // pure buildActivityChart model; this component only draws it.
  let { points }: { points: ActivityPoint[] } = $props();

  const model = $derived(buildActivityChart(points));
</script>

{#if model.bars.length === 0}
  <p class="py-16 text-center text-sm text-muted-foreground">No activity in this range yet.</p>
{:else}
  <figure class="flex flex-col gap-3">
    <svg
      viewBox="0 0 {model.width} {model.height}"
      class="w-full"
      role="img"
      aria-label="Vacancies added versus removed per period"
    >
      {#each model.bars as bar (bar.period)}
        <rect
          x={bar.addedX}
          y={bar.addedY}
          width={model.barW}
          height={bar.addedH}
          class="fill-emerald-500"
        >
          <title>{bar.period}: +{bar.added} added</title>
        </rect>
        <rect
          x={bar.removedX}
          y={bar.removedY}
          width={model.barW}
          height={bar.removedH}
          class="fill-rose-500"
        >
          <title>{bar.period}: −{bar.removed} removed</title>
        </rect>
      {/each}
      <!-- Baseline the bars sit on. -->
      <line
        x1="0"
        y1={model.baselineY}
        x2={model.width}
        y2={model.baselineY}
        class="stroke-border"
        stroke-width="1"
      />
    </svg>

    <figcaption class="flex items-center justify-center gap-6 text-xs text-muted-foreground">
      <span class="flex items-center gap-1.5">
        <span class="inline-block h-2.5 w-2.5 rounded-sm bg-emerald-500"></span>
        Added
      </span>
      <span class="flex items-center gap-1.5">
        <span class="inline-block h-2.5 w-2.5 rounded-sm bg-rose-500"></span>
        Removed
      </span>
    </figcaption>
  </figure>
{/if}
