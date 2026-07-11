<script lang="ts">
  import { buildActivityChart, formatCount, pickTickIndices } from '$lib/activityChart';
  import type { ActivityPoint } from '$lib/types';

  // A grouped bar chart of catalogue flow: per period, a green "added" bar and a
  // red "removed" bar. Hand-built SVG scaled to its container width — no charting
  // dependency, matching PipelineFunnel/RateDonut. Geometry comes from the pure
  // buildActivityChart model; this component draws it and layers on the x-axis
  // date labels, a y-axis max, and a hover readout of exact per-period counts.
  let { points }: { points: ActivityPoint[] } = $props();

  const model = $derived(buildActivityChart(points));
  const ticks = $derived(pickTickIndices(model.bars.length));
  // Top of the plot area (where a full-height bar reaches) = the model's top pad.
  const topY = $derived(model.height - model.baselineY);
  // Left/right padding the model reserves around the plot (mirrors its PAD), needed
  // to map a pointer x back to a bar index.
  const pad = $derived((model.width - model.slot * model.bars.length) / 2);

  // Hover state: the focused bar index plus the pointer pixel position for the
  // floating tooltip. Null until the pointer enters (so SSR renders no tooltip).
  let hovered = $state<number | null>(null);
  let tipX = $state(0);
  let tipY = $state(0);

  const hoveredBar = $derived(hovered === null ? null : (model.bars[hovered] ?? null));

  function onMove(e: PointerEvent) {
    const rect = (e.currentTarget as HTMLElement).getBoundingClientRect();
    if (rect.width === 0 || model.bars.length === 0) return;
    const vbX = ((e.clientX - rect.left) / rect.width) * model.width;
    const i = Math.floor((vbX - pad) / model.slot);
    hovered = Math.min(Math.max(i, 0), model.bars.length - 1);
    tipX = e.clientX - rect.left;
    tipY = e.clientY - rect.top;
  }

  /** Short axis label, e.g. "Jun 1". */
  function shortDate(period: string): string {
    return new Date(period + 'T00:00:00Z').toLocaleDateString(undefined, {
      month: 'short',
      day: 'numeric',
      timeZone: 'UTC',
    });
  }

  /** Full tooltip date, e.g. "1 Jun 2026". */
  function fullDate(period: string): string {
    return new Date(period + 'T00:00:00Z').toLocaleDateString(undefined, {
      day: 'numeric',
      month: 'short',
      year: 'numeric',
      timeZone: 'UTC',
    });
  }
</script>

{#if model.bars.length === 0}
  <p class="py-16 text-center text-sm text-muted-foreground">No activity in this range yet.</p>
{:else}
  <figure class="flex flex-col gap-3">
    <!-- svelte-ignore a11y_no_static_element_interactions -->
    <div
      class="relative"
      role="img"
      aria-label="Vacancies added versus removed per period"
      onpointermove={onMove}
      onpointerleave={() => (hovered = null)}
    >
      <svg viewBox="0 0 {model.width} {model.height + 22}" class="w-full">
        <!-- y-axis max reference line + label -->
        <line
          x1={pad}
          y1={topY}
          x2={model.width - pad}
          y2={topY}
          class="stroke-border"
          stroke-dasharray="2 3"
        />
        <text x={pad} y={topY - 4} class="fill-muted-foreground" font-size="11">
          {formatCount(model.max)}
        </text>

        {#if hovered !== null && hoveredBar}
          <!-- highlight the focused slot -->
          <rect
            x={pad + hovered * model.slot}
            y={topY}
            width={model.slot}
            height={model.baselineY - topY}
            class="fill-muted/50"
          />
        {/if}

        {#each model.bars as bar (bar.period)}
          <rect
            x={bar.addedX}
            y={bar.addedY}
            width={model.barW}
            height={bar.addedH}
            class="fill-emerald-500"
          />
          <rect
            x={bar.removedX}
            y={bar.removedY}
            width={model.barW}
            height={bar.removedH}
            class="fill-rose-500"
          />
        {/each}

        <!-- baseline -->
        <line
          x1="0"
          y1={model.baselineY}
          x2={model.width}
          y2={model.baselineY}
          class="stroke-border"
          stroke-width="1"
        />

        <!-- x-axis date labels (thinned for long series) -->
        {#each ticks as i (i)}
          {@const bar = model.bars[i]}
          {#if bar}
            <text
              x={bar.centerX}
              y={model.baselineY + 16}
              text-anchor="middle"
              class="fill-muted-foreground"
              font-size="11"
            >
              {shortDate(bar.period)}
            </text>
          {/if}
        {/each}
      </svg>

      {#if hoveredBar}
        <div
          class="pointer-events-none absolute z-10 -translate-x-1/2 -translate-y-full rounded-md border border-border bg-popover px-2.5 py-1.5 text-xs shadow-md"
          style="left: {tipX}px; top: {tipY - 8}px;"
        >
          <div class="mb-1 font-medium text-foreground">{fullDate(hoveredBar.period)}</div>
          <div class="flex items-center gap-1.5 text-muted-foreground">
            <span class="inline-block h-2 w-2 rounded-sm bg-emerald-500"></span>
            Added <span class="ml-auto font-medium text-foreground">{hoveredBar.added.toLocaleString()}</span>
          </div>
          <div class="flex items-center gap-1.5 text-muted-foreground">
            <span class="inline-block h-2 w-2 rounded-sm bg-rose-500"></span>
            Removed <span class="ml-auto font-medium text-foreground">{hoveredBar.removed.toLocaleString()}</span>
          </div>
        </div>
      {/if}
    </div>

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
