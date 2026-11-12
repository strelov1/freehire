<script lang="ts">
  import type { InsightSalaryMonth } from '#lib/api.js';
  import { pickTickIndices } from '#lib/activityChart.js';
  import { nearestIndexByX } from '#lib/chartKeys.js';
  import { formatSalary } from '#lib/insights.js';
  import { buildCompareChart } from '#lib/salaries.js';

  // Median advertised salary by month for several lines (a role and a country each) on
  // one axis, in one currency and period: lines in different currencies cannot share a
  // scale, so a line without months in this unit is listed as having none.
  let {
    lines,
    currency,
    period,
    currentMonth,
  }: {
    lines: { label: string; months: InsightSalaryMonth[] }[];
    currency: string;
    period: string;
    currentMonth: string;
  } = $props();

  // Categorical slots in fixed order (design-system chart-1…5), written out whole so
  // Tailwind sees every class. Identity also rides on the legend, tooltip and table.
  const SERIES = [
    { line: 'stroke-chart-1', dot: 'stroke-chart-1 fill-chart-1', swatch: 'bg-chart-1' },
    { line: 'stroke-chart-2', dot: 'stroke-chart-2 fill-chart-2', swatch: 'bg-chart-2' },
    { line: 'stroke-chart-3', dot: 'stroke-chart-3 fill-chart-3', swatch: 'bg-chart-3' },
    { line: 'stroke-chart-4', dot: 'stroke-chart-4 fill-chart-4', swatch: 'bg-chart-4' },
    { line: 'stroke-chart-5', dot: 'stroke-chart-5 fill-chart-5', swatch: 'bg-chart-5' },
  ] as const;
  const series = (i: number) => SERIES[i % SERIES.length] ?? SERIES[0];

  const model = $derived(
    buildCompareChart(
      lines.map((l) => l.months),
      currentMonth,
    ),
  );
  const ticks = $derived(model ? pickTickIndices(model.slots.length) : []);
  let hovered = $state<number | null>(null); // a slot index
  let tipX = $state(0);
  let svgEl = $state<SVGSVGElement | null>(null);

  const money = (v: number) => formatSalary(v, currency);
  const monthDate = (key: string) => new Date(`${key}-01T00:00:00Z`);
  const shortMonth = (key: string) =>
    monthDate(key).toLocaleDateString('en-US', { month: 'short', timeZone: 'UTC' }) +
    (key.endsWith('-01') || key === model?.slots[0] ? ` ’${key.slice(2, 4)}` : '');
  const longMonth = (key: string) =>
    monthDate(key).toLocaleDateString('en-US', { month: 'long', year: 'numeric', timeZone: 'UTC' });
  const valueAt = (line: number, month: string) => lines[line]?.months.find((m) => m.month === month);

  function onMove(e: PointerEvent) {
    if (!model) return;
    // The month comes from the plot's own box (the wrapper is padded around it); the
    // tooltip is placed in the wrapper, which it is positioned against.
    const plot = svgEl?.getBoundingClientRect();
    const box = (e.currentTarget as HTMLElement).getBoundingClientRect();
    if (!plot || plot.width === 0) return;
    const vbX = ((e.clientX - plot.left) / plot.width) * model.width;
    hovered = nearestIndexByX(
      model.slotX.map((x) => ({ x })),
      vbX,
    );
    tipX = e.clientX - box.left;
  }
  const hoveredMonth = $derived(hovered === null || !model ? null : (model.slots[hovered] ?? null));
</script>

<div class="salary-compare">
  <ul class="mb-2 flex flex-wrap gap-x-4 gap-y-1 text-sm">
    {#each lines as l, i (l.label)}
      <li class="flex items-center gap-1.5">
        <span class="inline-block h-0.5 w-4 rounded-full {series(i).swatch}" aria-hidden="true"></span>
        <span class="text-foreground">{l.label}</span>
        {#if l.months.length === 0}<span class="text-muted-foreground">— no {currency} per {period} history</span>{/if}
      </li>
    {/each}
  </ul>

  {#if model}
    <div
      class="relative rounded-lg border border-border p-4"
      role="img"
      aria-label="Median salary by month, {currency} per {period}, {lines.length} lines; the table below lists every figure"
      onpointermove={onMove}
      onpointerleave={() => (hovered = null)}
    >
      <svg
        bind:this={svgEl}
        viewBox="0 0 {model.width} {model.height}"
        class="block h-auto w-full overflow-visible"
        aria-hidden="true"
      >
        <line x1="0" y1={model.topY} x2={model.width} y2={model.topY} class="stroke-border" stroke-dasharray="2 3" />
        <text x="0" y={model.topY - 6} class="fill-muted-foreground max-sm:hidden" font-size="12"
          >{money(model.max)}</text
        >
        <text x="0" y={model.baselineY - 6} class="fill-muted-foreground max-sm:hidden" font-size="12"
          >{money(model.min)}</text
        >
        {#if hovered !== null}
          <line
            x1={model.slotX[hovered]}
            x2={model.slotX[hovered]}
            y1={model.topY}
            y2={model.baselineY}
            class="stroke-muted-foreground/40"
          />
        {/if}
        {#each model.lines as line, i (i)}
          {#each line.segments as seg, s (s)}
            {#if seg.length > 1}
              <polyline
                points={seg.map((p) => `${p.x},${p.y}`).join(' ')}
                class={series(i).line}
                fill="none"
                stroke-width="2"
                stroke-linejoin="round"
                stroke-linecap="round"
              />
            {/if}
          {/each}
          {#each line.points as p (p.month)}
            <circle
              cx={p.x}
              cy={p.y}
              r={hoveredMonth === p.month ? 5 : 3.5}
              stroke-width="2"
              class={p.partial ? `${series(i).line} fill-background` : series(i).dot}
            />
          {/each}
        {/each}
        <line x1="0" y1={model.baselineY} x2={model.width} y2={model.baselineY} class="stroke-border" />
        {#each ticks as t (t)}
          {@const key = model.slots[t]}
          {#if key}
            <text
              x={model.slotX[t]}
              y={model.baselineY + 18}
              text-anchor="middle"
              class="fill-muted-foreground max-sm:hidden"
              font-size="12">{shortMonth(key)}</text
            >
          {/if}
        {/each}
      </svg>

      {#if hoveredMonth}
        <div
          class="pointer-events-none absolute top-6 z-10 -translate-x-1/2 rounded-md border border-border bg-popover px-2.5 py-1.5 text-xs shadow-md"
          style="left: {tipX}px;"
        >
          <div class="font-medium text-foreground">
            {longMonth(hoveredMonth)}{hoveredMonth === currentMonth ? ' (so far)' : ''}
          </div>
          {#each lines as l, i (l.label)}
            {@const v = valueAt(i, hoveredMonth)}
            <div class="flex items-center gap-1.5 text-muted-foreground">
              <span class="inline-block h-0.5 w-3 rounded-full {series(i).swatch}"></span>
              {l.label}
              <span class="ml-auto pl-3 font-medium text-foreground tabular-nums">{v ? money(v.p50) : '—'}</span>
            </div>
          {/each}
        </div>
      {/if}
    </div>

    <details class="mt-2 text-sm">
      <summary class="cursor-pointer text-muted-foreground">Show the figures as a table</summary>
      <div class="mt-2 overflow-x-auto">
        <table class="w-full text-xs tabular-nums">
          <thead>
            <tr class="text-left text-muted-foreground">
              <th class="py-1 pr-3 font-medium">Month</th>
              {#each lines as l (l.label)}<th class="py-1 pr-3 font-medium">{l.label}</th>{/each}
            </tr>
          </thead>
          <tbody>
            {#each model.slots as m (m)}
              <tr class="border-t border-border">
                <td class="py-1 pr-3">{longMonth(m)}</td>
                {#each lines as l, i (l.label)}
                  {@const v = valueAt(i, m)}
                  <td class="py-1 pr-3">{v ? `${money(v.p50)} (${v.sample_size})` : '—'}</td>
                {/each}
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    </details>
  {:else}
    <p class="text-sm text-muted-foreground">No line has three months of {currency} per {period} figures yet.</p>
  {/if}
</div>
