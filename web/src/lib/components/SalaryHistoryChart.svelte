<script lang="ts">
  import type { InsightSalaryMonth } from '#lib/api.js';
  import { pickTickIndices } from '#lib/activityChart.js';
  import { chartTipPosition, nextChartIndex } from '#lib/chartKeys.js';
  import { formatSalary } from '#lib/insights.js';
  import { MIX_SHIFT_RATIO, buildSalaryChart, historySeries } from '#lib/salaryHistoryChart.js';

  let { rows, currentMonth }: { rows: InsightSalaryMonth[]; currentMonth: string } = $props();

  // The four largest series: a category can carry a dozen (currency, period) pairs, most
  // of them a few hundred postings, and a dozen buttons bury the one most people want.
  const series = $derived(historySeries(rows).slice(0, 4));
  let picked = $state(0);
  // Back to the first series when the rows change under a reused component (client
  // navigation between categories) and the old index no longer exists.
  const selected = $derived(picked < series.length ? picked : 0);
  const active = $derived(series[selected]);
  const model = $derived(active ? buildSalaryChart(active, currentMonth) : null);
  const ticks = $derived(model ? pickTickIndices(model.slots.length) : []);

  let hovered = $state<number | null>(null);
  let tipX = $state(0);
  let tipY = $state(0);
  const hoveredPoint = $derived(hovered === null || !model ? null : (model.points[hovered] ?? null));
  let chartEl = $state<HTMLDivElement | null>(null);
  let valuesEl = $state<HTMLUListElement | null>(null);

  const PERIOD_LABEL: Record<string, string> = { year: 'per year', month: 'per month', day: 'per day', hour: 'per hour' };
  const seriesLabel = (s: { currency: string; period: string }) => `${s.currency} ${PERIOD_LABEL[s.period] ?? s.period}`;

  // Pinned to en-US and UTC: the page is server-rendered, and a locale or time zone
  // that differs between server and browser would rename a month on hydration.
  const monthDate = (key: string) => new Date(`${key}-01T00:00:00Z`);
  const shortMonth = (key: string) =>
    monthDate(key).toLocaleDateString('en-US', { month: 'short', timeZone: 'UTC' }) +
    (key.endsWith('-01') || key === model?.slots[0] ? ` ’${key.slice(2, 4)}` : '');
  const longMonth = (key: string) =>
    monthDate(key).toLocaleDateString('en-US', { month: 'long', year: 'numeric', timeZone: 'UTC' });
  const money = (v: number) => (active ? formatSalary(v, active.currency) : String(v));

  function pointText(i: number): string {
    const p = model?.points[i];
    if (!p) return '';
    const r = p.row;
    const shift = p.mixShift ? `; postings rose at least ${MIX_SHIFT_RATIO}-fold from the month before, so part of the change is which postings are counted` : '';
    return `${longMonth(p.month)}${p.partial ? ' (so far)' : ''}: median ${money(r.p50)}, middle half ${money(r.p25)} to ${money(r.p75)}, ${r.sample_size.toLocaleString('en-US')} postings${shift}`;
  }

  function showPoint(i: number) {
    if (!model) return;
    const point = model.points[i];
    if (!point) return;
    hovered = i;
    if (!chartEl) return;
    const tip = chartTipPosition(chartEl, point.x / model.width);
    tipX = tip.x;
    tipY = tip.y;
  }

  function onValueKey(e: KeyboardEvent) {
    if (!model) return;
    const i = nextChartIndex(e.key, hovered, model.points.length);
    if (i === null) return;
    e.preventDefault();
    showPoint(i);
    valuesEl?.querySelectorAll('button')[i]?.focus();
  }

  function onMove(e: PointerEvent) {
    if (!model || model.points.length === 0) return;
    const rect = (e.currentTarget as HTMLElement).getBoundingClientRect();
    if (rect.width === 0) return;
    const vbX = ((e.clientX - rect.left) / rect.width) * model.width;
    let nearest = 0;
    let nearestDist = Infinity;
    model.points.forEach((p, i) => {
      const d = Math.abs(p.x - vbX);
      if (d < nearestDist) {
        nearestDist = d;
        nearest = i;
      }
    });
    hovered = nearest;
    tipX = e.clientX - rect.left;
    tipY = e.clientY - rect.top;
  }

  const bandPath = (seg: { x: number; yLow: number; yHigh: number }[]) =>
    [...seg.map((p) => `${p.x},${p.yHigh}`), ...[...seg].reverse().map((p) => `${p.x},${p.yLow}`)].join(' ');
</script>

{#if model && active}
  <section class="mb-8" aria-labelledby="salary-history-heading">
    <div class="flex flex-wrap items-end justify-between gap-3">
      <div>
        <h2 id="salary-history-heading" class="text-lg font-semibold text-foreground">Salary over time</h2>
        <p class="mt-0.5 text-sm text-muted-foreground">
          Median advertised salary by month, {seriesLabel(active)}. The shaded band is the middle half of postings.
        </p>
      </div>
      {#if series.length > 1}
        <div class="inline-flex flex-wrap gap-1 text-sm" role="group" aria-label="Currency and period">
          {#each series as s, i (s.currency + s.period)}
            <button
              type="button"
              class="rounded-md border px-2.5 py-1 {i === selected
                ? 'border-foreground font-medium text-foreground'
                : 'border-border text-muted-foreground hover:text-foreground'}"
              aria-pressed={i === selected}
              onclick={() => {
                picked = i;
                hovered = null;
              }}>{seriesLabel(s)}</button
            >
          {/each}
        </div>
      {/if}
    </div>

    <div
      class="relative mt-3 rounded-lg border border-border p-4"
      role="img"
      aria-label="Median salary by month, {seriesLabel(active)}"
      bind:this={chartEl}
      onpointermove={onMove}
      onpointerleave={() => (hovered = null)}
    >
      <svg viewBox="0 0 {model.width} {model.height}" class="w-full">
        <line x1="0" y1={model.topY} x2={model.width} y2={model.topY} class="stroke-border" stroke-dasharray="2 3" />
        <!-- Axis text is drawn in the viewBox, so a phone shrinks it to ~4px; there it is
             hidden and the same range is written as text under the chart. -->
        <text x="0" y={model.topY - 6} class="fill-muted-foreground max-sm:hidden" font-size="12">{money(model.max)}</text>
        <text x="0" y={model.baselineY - 6} class="fill-muted-foreground max-sm:hidden" font-size="12">{money(model.min)}</text>

        {#each model.segments as seg, s (s)}
          {#if seg.length > 1}
            <polygon points={bandPath(seg)} class="fill-brand/15" />
            <!-- One step per month pair, so the step INTO a month whose sample jumped can
                 be dashed while the rest of the line stays solid. -->
            {#each seg.slice(1) as p, k (p.month)}
              {@const from = seg[k]}
              {#if from}
                <line
                  x1={from.x}
                  y1={from.yMid}
                  x2={p.x}
                  y2={p.yMid}
                  class="stroke-brand"
                  stroke-width="2"
                  stroke-linecap="round"
                  stroke-dasharray={p.mixShift ? '6 6' : undefined}
                />
              {/if}
            {/each}
          {:else if seg[0]}
            <!-- A month with no neighbour: its band is a bar, there is no line to draw. -->
            <line x1={seg[0].x} y1={seg[0].yHigh} x2={seg[0].x} y2={seg[0].yLow} class="stroke-brand/25" stroke-width="8" stroke-linecap="round" />
          {/if}
        {/each}

        {#each model.points as p, i (p.month)}
          <circle
            cx={p.x}
            cy={p.yMid}
            r={hovered === i ? 6 : 4}
            stroke-width="2"
            class="stroke-brand {p.partial ? 'fill-background' : 'fill-brand'}"
          />
        {/each}

        <line x1="0" y1={model.baselineY} x2={model.width} y2={model.baselineY} class="stroke-border" />

        {#each ticks as i (i)}
          {@const key = model.slots[i]}
          {#if key}
            <text
              x={model.slotX[i]}
              y={model.baselineY + 18}
              text-anchor="middle"
              class="fill-muted-foreground max-sm:hidden"
              font-size="12">{shortMonth(key)}</text
            >
          {/if}
        {/each}
      </svg>

      {#if hoveredPoint}
        <div
          class="pointer-events-none absolute z-10 -translate-x-1/2 -translate-y-full rounded-md border border-border bg-popover px-2.5 py-1.5 text-xs shadow-md"
          style="left: {tipX}px; top: {tipY - 8}px;"
        >
          <div class="font-medium text-foreground">
            {longMonth(hoveredPoint.month)}{hoveredPoint.partial ? ' (so far)' : ''}
          </div>
          <div class="text-muted-foreground">
            Median <span class="font-medium text-foreground">{money(hoveredPoint.row.p50)}</span>
          </div>
          <div class="text-muted-foreground">
            Middle half {money(hoveredPoint.row.p25)} – {money(hoveredPoint.row.p75)}
          </div>
          <div class="text-muted-foreground">{hoveredPoint.row.sample_size.toLocaleString('en-US')} postings</div>
          {#if hoveredPoint.mixShift}
            <div class="mt-1 max-w-56 text-muted-foreground">
              {MIX_SHIFT_RATIO}× or more postings than the month before — part of the change is which postings are counted.
            </div>
          {/if}
        </div>
      {/if}
    </div>

    <p class="mt-1 flex justify-between text-xs text-muted-foreground tabular-nums sm:hidden">
      <span>{longMonth(model.slots[0] ?? currentMonth)} – {longMonth(model.slots[model.slots.length - 1] ?? currentMonth)}</span>
      <span>{money(model.min)} – {money(model.max)}</span>
    </p>

    <!-- The same figures as text: what a screen reader and a no-JS reader get, and the
         keyboard's way along the line (arrow keys, Home, End). -->
    <ul class="sr-only" bind:this={valuesEl}>
      {#each model.points as p, i (p.month)}
        <li>
          <button type="button" tabindex={i === (hovered ?? 0) ? 0 : -1} onfocus={() => showPoint(i)} onkeydown={onValueKey}>
            {pointText(i)}
          </button>
        </li>
      {/each}
    </ul>

    <p class="mt-2 text-xs text-muted-foreground">
      Months follow the date a posting was published, and count postings closed since — the
      table below counts only those open now. A month needs 30 postings with a salary
      to be shown; a missing month is a gap, not a guess. A dashed step means the month had at
      least {MIX_SHIFT_RATIO}× the postings of the one before, so part of that change is which
      postings are counted. The hollow point is the current month, still filling up. These are
      postings on freehire, not the whole job market.
    </p>
  </section>
{/if}
