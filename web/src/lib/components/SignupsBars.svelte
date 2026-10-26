<script lang="ts">
  import { buildSignupsChart } from '#lib/signupsChart.js';
  import { pickTickIndices } from '#lib/activityChart.js';
  import { chartTipPosition, nextChartIndex } from '#lib/chartKeys.js';
  import { formatCount } from '#lib/utils.js';
  import type { UserGrowthPoint } from '#lib/types.js';

  // A single-series bar chart of new members per day — the ActivityBars sibling
  // for signups, one bar per day instead of an added/removed pair. Hand-built SVG
  // scaled to its container width, no charting dependency, matching the site's
  // other bespoke charts. Geometry comes from the pure buildSignupsChart model.
  let { points }: { points: UserGrowthPoint[] } = $props();

  const model = $derived(buildSignupsChart(points));
  const ticks = $derived(pickTickIndices(model.bars.length));
  const topY = $derived(model.height - model.baselineY);

  let hovered = $state<number | null>(null);
  let tipX = $state(0);
  let tipY = $state(0);

  const hoveredBar = $derived(hovered === null ? null : (model.bars[hovered] ?? null));

  let chartEl = $state<HTMLDivElement | null>(null);
  let valuesEl = $state<HTMLUListElement | null>(null);

  function showBar(i: number) {
    const bar = model.bars[i];
    if (!bar) return;
    hovered = i;
    if (!chartEl) return;
    const tip = chartTipPosition(chartEl, bar.centerX / model.width);
    tipX = tip.x;
    tipY = tip.y;
  }

  function onValueKey(e: KeyboardEvent) {
    const i = nextChartIndex(e.key, hovered, model.bars.length);
    if (i === null) return;
    e.preventDefault();
    showBar(i);
    valuesEl?.querySelectorAll('button')[i]?.focus();
  }

  function onMove(e: PointerEvent) {
    const rect = (e.currentTarget as HTMLElement).getBoundingClientRect();
    if (rect.width === 0 || model.bars.length === 0) return;
    const vbX = ((e.clientX - rect.left) / rect.width) * model.width;
    const i = Math.floor((vbX - model.pad) / model.slot);
    hovered = Math.min(Math.max(i, 0), model.bars.length - 1);
    tipX = e.clientX - rect.left;
    tipY = e.clientY - rect.top;
  }

  /** Short axis label, e.g. "Jun 1". */
  function shortDate(date: string): string {
    return new Date(date + 'T00:00:00Z').toLocaleDateString(undefined, {
      month: 'short',
      day: 'numeric',
      timeZone: 'UTC',
    });
  }

  /** Full tooltip date, e.g. "1 Jun 2026". */
  function fullDate(date: string): string {
    return new Date(date + 'T00:00:00Z').toLocaleDateString(undefined, {
      day: 'numeric',
      month: 'short',
      year: 'numeric',
      timeZone: 'UTC',
    });
  }
</script>

{#if model.bars.length === 0}
  <p class="py-16 text-center text-sm text-muted-foreground">No members yet.</p>
{:else}
  <figure class="flex flex-col gap-3">
    <div
      class="relative"
      role="img"
      aria-label="New members per day"
      bind:this={chartEl}
      onpointermove={onMove}
      onpointerleave={() => (hovered = null)}
    >
      <svg viewBox="0 0 {model.width} {model.height + 22}" class="w-full">
        <!-- y-axis max reference line + label -->
        <line
          x1={model.pad}
          y1={topY}
          x2={model.width - model.pad}
          y2={topY}
          class="stroke-border"
          stroke-dasharray="2 3"
        />
        <text x={model.pad} y={topY - 4} class="fill-muted-foreground" font-size="11">
          {formatCount(model.max)}
        </text>

        {#if hovered !== null && hoveredBar}
          <!-- highlight the focused slot -->
          <rect
            x={model.pad + hovered * model.slot}
            y={topY}
            width={model.slot}
            height={model.baselineY - topY}
            class="fill-muted/50"
          />
        {/if}

        {#each model.bars as bar (bar.date)}
          <rect x={bar.x} y={bar.y} width={model.barW} height={bar.h} class="fill-primary" />
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
              {shortDate(bar.date)}
            </text>
          {/if}
        {/each}
      </svg>

      {#if hoveredBar}
        <div
          class="pointer-events-none absolute z-10 -translate-x-1/2 -translate-y-full rounded-md border border-border bg-popover px-2.5 py-1.5 text-xs shadow-md"
          style="left: {tipX}px; top: {tipY - 8}px;"
        >
          <div class="mb-1 font-medium text-foreground">{fullDate(hoveredBar.date)}</div>
          <div class="flex items-center gap-1.5 text-muted-foreground">
            <span class="inline-block h-2 w-2 rounded-sm bg-primary"></span>
            New members <span class="ml-auto font-medium text-foreground"
              >{hoveredBar.new.toLocaleString()}</span
            >
          </div>
        </div>
      {/if}
    </div>
    <ul class="sr-only" bind:this={valuesEl}>
      {#each model.bars as bar, i (bar.date)}
        <li>
          <button
            type="button"
            tabindex={i === (hovered ?? 0) ? 0 : -1}
            onfocus={() => showBar(i)}
            onkeydown={onValueKey}
          >
            {fullDate(bar.date)}: {bar.new.toLocaleString()} new members
          </button>
        </li>
      {/each}
    </ul>
  </figure>
{/if}
