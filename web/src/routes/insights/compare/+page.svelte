<script lang="ts">
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import Seo from '#lib/components/Seo.svelte';
  import { categoryLabel } from '#lib/labels.js';
  import { groupDemand, summarizeDemand } from '#lib/roleDemand.js';
  import type { PageData } from './$types';

  let { data }: { data: PageData } = $props();

  const description = 'Compare how hiring demand changed for two specialties over the last 12 full weeks.';
  const count = new Intl.NumberFormat('en-US');
  const compact = new Intl.NumberFormat('en-US', { notation: 'compact', maximumFractionDigits: 1 });
  const change = new Intl.NumberFormat('en-US', {
    style: 'percent',
    signDisplay: 'exceptZero',
    maximumFractionDigits: 1,
  });
  const shortDate = new Intl.DateTimeFormat('en-US', { month: 'short', day: 'numeric', timeZone: 'UTC' });
  const longDate = new Intl.DateTimeFormat('en-US', {
    month: 'short',
    day: 'numeric',
    year: 'numeric',
    timeZone: 'UTC',
  });
  const date = (iso: string) => new Date(`${iso}T00:00:00Z`);

  let first = $state('backend');
  let second = $state('devops');

  const byCategory = $derived(groupDemand(data.points));
  const categories = $derived(
    [...byCategory.keys()].sort((a, b) => categoryLabel(a).localeCompare(categoryLabel(b), 'en-US')),
  );
  const left = $derived(byCategory.get(first) ?? []);
  const right = $derived(byCategory.get(second) ?? []);
  const weeks = $derived(
    left.flatMap((point, i) => {
      const other = right[i];
      return other?.week === point.week ? [{ week: point.week, first: point.new_jobs, second: other.new_jobs }] : [];
    }),
  );
  const firstWeek = $derived(weeks.at(0)?.week);
  const lastWeek = $derived(weeks.at(-1)?.week);
  const leftSummary = $derived(summarizeDemand(left.map((point) => point.new_jobs)));
  const rightSummary = $derived(summarizeDemand(right.map((point) => point.new_jobs)));

  const chart = $derived.by(() => {
    const peak = Math.max(4, ...[...left, ...right].map((point) => point.new_jobs));
    const magnitude = 10 ** Math.floor(Math.log10(peak / 4));
    const step = [1, 2, 5, 10].find((n) => n * magnitude * 4 >= peak) ?? 10;
    const max = 4 * step * magnitude;
    return { max, ticks: [max, 0.75 * max, 0.5 * max, 0.25 * max, 0] };
  });
  const x = (index: number) => (100 * index) / 11;
  const y = (value: number) => 100 * (1 - value / chart.max);
  const line = (points: typeof left) => points.map((point, i) => `${x(i)},${y(point.new_jobs)}`).join(' ');
</script>

<Seo title="Compare Hiring Demand · freehire" {description} canonical={`${page.url.origin}/insights/compare`} />

<div class="mx-auto w-full max-w-4xl px-4 py-8">
  <nav aria-label="Breadcrumb" class="text-sm">
    <a href={resolve('/insights')} class="text-brand-strong hover:underline">← Insights</a>
  </nav>
  <h1 class="mt-3 text-3xl font-bold tracking-tight text-foreground">Compare hiring demand</h1>
  <p class="mt-2 text-muted-foreground">{description}</p>

  {#if weeks.length === 12 && firstWeek && lastWeek}
    <div class="mt-6 grid grid-cols-2 gap-3 sm:gap-6">
      {#each [0, 1] as index (index)}
        {@const isFirst = index === 0}
        {@const summary = isFirst ? leftSummary : rightSummary}
        <div class="min-w-0">
          <label class="block text-sm font-medium text-foreground" for={`specialty-${index}`}>
            {isFirst ? 'First specialty' : 'Second specialty'}
          </label>
          <select
            id={`specialty-${index}`}
            class="mt-1 block min-h-10 w-full rounded-md border border-border bg-background px-2.5 text-sm text-foreground focus-visible:outline-2 focus-visible:outline-brand-strong"
            value={isFirst ? first : second}
            onchange={(event) => {
              if (isFirst) first = event.currentTarget.value;
              else second = event.currentTarget.value;
            }}
          >
            {#each categories as category (category)}
              <option value={category} disabled={category === (isFirst ? second : first)}
                >{categoryLabel(category)}</option
              >
            {/each}
          </select>
          <div class="mt-4 border-t border-border pt-3" aria-live="polite" aria-atomic="true">
            <div class="flex items-center gap-2">
              <span
                aria-hidden="true"
                class="w-5 shrink-0 border-t-2 {isFirst ? 'border-brand-strong' : 'border-dashed border-foreground'}"
              ></span>
              <span class="text-2xl font-semibold tabular-nums text-foreground">{count.format(summary.latest)}</span>
            </div>
            <p class="text-sm text-muted-foreground">New jobs in the last 4 weeks</p>
            <p class="mt-1 text-sm text-muted-foreground">
              {#if summary.change === null}
                Previous 4 weeks had zero new jobs
              {:else}
                <strong class="font-semibold tabular-nums text-foreground">{change.format(summary.change)}</strong>
                vs. previous 4 weeks
              {/if}
            </p>
          </div>
        </div>
      {/each}
    </div>

    <section class="mt-8" aria-labelledby="weekly-demand-heading">
      <h2 id="weekly-demand-heading" class="text-lg font-semibold text-foreground">New jobs per week</h2>
      <p class="mt-1 text-sm text-muted-foreground">
        {longDate.format(date(firstWeek))} – {longDate.format(new Date(date(lastWeek).getTime() + 6 * 86_400_000))}
      </p>

      <div class="mt-4 rounded-lg border border-border px-3 py-4 sm:px-5">
        <div class="chart-grid grid gap-x-2 pr-3 sm:gap-x-3 sm:pr-5" aria-hidden="true">
          <div class="relative text-right text-xs tabular-nums text-muted-foreground">
            {#each chart.ticks as tick (tick)}
              <span class="absolute right-0 -translate-y-1/2" style:top={`${y(tick)}%`}>{compact.format(tick)}</span>
            {/each}
            <span class="invisible">{compact.format(chart.max)}</span>
          </div>
          <svg viewBox="0 0 100 100" preserveAspectRatio="none" class="h-44 w-full overflow-visible sm:h-64">
            {#each chart.ticks as tick (tick)}
              <line
                x1="0"
                x2="100"
                y1={y(tick)}
                y2={y(tick)}
                class="stroke-border"
                vector-effect="non-scaling-stroke"
              />
            {/each}
            <polyline
              points={line(left)}
              fill="none"
              class="stroke-brand-strong"
              stroke-width="3"
              stroke-linejoin="round"
              vector-effect="non-scaling-stroke"
            />
            <polyline
              points={line(right)}
              fill="none"
              class="stroke-foreground"
              stroke-width="2"
              stroke-dasharray="4 4"
              stroke-linejoin="round"
              vector-effect="non-scaling-stroke"
            />
          </svg>
          <div class="relative col-start-2 mt-2 h-4 text-xs text-muted-foreground">
            {#each weeks as { week }, index (week)}
              {#if [0, 3, 6, 9, 11].includes(index)}
                <span
                  class="absolute whitespace-nowrap {index === 0
                    ? ''
                    : index === 11
                      ? '-translate-x-full'
                      : '-translate-x-1/2'} {index === 3 || index === 9 ? 'hidden sm:block' : ''}"
                  style:left={`${x(index)}%`}>{shortDate.format(date(week))}</span
                >
              {/if}
            {/each}
          </div>
        </div>
        <details class="mt-3 border-t border-border pt-3 text-sm">
          <summary class="cursor-pointer font-medium text-brand-strong">View weekly counts</summary>
          <table class="mt-2 w-full text-left">
            <thead>
              <tr class="border-b border-border text-muted-foreground">
                <th scope="col" class="py-2 pr-2 font-medium">Week of</th>
                <th scope="col" class="py-2 pr-2 text-right font-medium">{categoryLabel(first)}</th>
                <th scope="col" class="py-2 text-right font-medium">{categoryLabel(second)}</th>
              </tr>
            </thead>
            <tbody>
              {#each weeks as point (point.week)}
                <tr class="border-b border-border last:border-b-0">
                  <th scope="row" class="py-1.5 pr-2 font-normal"
                    ><time datetime={point.week}>{shortDate.format(date(point.week))}</time></th
                  >
                  <td class="py-1.5 pr-2 text-right tabular-nums">{count.format(point.first)}</td>
                  <td class="py-1.5 text-right tabular-nums">{count.format(point.second)}</td>
                </tr>
              {/each}
            </tbody>
          </table>
        </details>
      </div>

      <p class="mt-3 max-w-prose text-sm text-muted-foreground">
        Counts include unique public jobs from sources tracked by freehire, including jobs that have since closed. Each
        job counts in its publication week, or in the week freehire first found it if the source gave no date. Weeks
        start on Monday in UTC. Changes in source coverage can affect the counts.
      </p>
    </section>
  {:else}
    <p class="mt-6 rounded-lg border border-border p-4 text-sm text-muted-foreground">
      Hiring demand data is temporarily unavailable. Check back shortly.
    </p>
  {/if}
</div>

<style>
  .chart-grid {
    grid-template-columns: auto minmax(0, 1fr);
  }
</style>
