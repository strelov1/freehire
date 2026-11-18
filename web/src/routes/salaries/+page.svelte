<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import Seo from '#lib/components/Seo.svelte';
  import SalaryHistoryChart from '#lib/components/SalaryHistoryChart.svelte';
  import SalaryCompareChart from '#lib/components/SalaryCompareChart.svelte';
  import SalaryHistogram from '#lib/components/SalaryHistogram.svelte';
  import { CATEGORY_OPTIONS, SENIORITY_OPTIONS, countryLabel } from '#lib/facets.js';
  import { formatSalary, seniorityLabel } from '#lib/insights.js';
  import { categoryLabel } from '#lib/labels.js';
  import {
    MAX_COMPARE,
    lineMonths,
    pageUnit,
    pageUnits,
    salariesSearch,
    unitOf,
    type SalariesQuery,
  } from '#lib/salaries.js';
  import type { PageData } from './$types';

  let { data }: { data: PageData } = $props();

  const q = $derived(data.query);
  const scope = $derived(q.scope);
  const bands = $derived(data.bands ?? []);
  // One unit for the whole page, then the band in it: the cards and the chart agree.
  const unit = $derived(pageUnit(bands, q.unit, data.histories));
  const units = $derived(pageUnits(bands, data.histories));
  const band = $derived(bands.find((b) => unitOf(b) === unit) ?? null);
  // The URL's country stays a choice even when the country list failed to load.
  const countryOptions = $derived(
    scope.country && !data.countries.includes(scope.country) ? [scope.country, ...data.countries] : data.countries,
  );
  const [currency = '', period = ''] = $derived(unit.split('-'));

  const lineName = (category: string, country: string) =>
    [category ? categoryLabel(category) : 'All roles', country ? countryLabel(country) : '']
      .filter(Boolean)
      .join(' · ');
  const scopeName = $derived(
    [lineName(scope.category, scope.country), scope.seniority ? seniorityLabel(scope.seniority) : '']
      .filter(Boolean)
      .join(' · '),
  );
  const lines = $derived(
    [{ category: scope.category, country: scope.country }, ...q.compare].map((l, i) => ({
      label: lineName(l.category, l.country),
      months: lineMonths(data.histories[i] ?? null, unit),
    })),
  );

  const categories = [...CATEGORY_OPTIONS].sort((a, b) => a.label.localeCompare(b.label));
  // Keep the scroll and focus: a picker changes the figures under it, not the page.
  const go = (next: SalariesQuery) =>
    // eslint-disable-next-line svelte/no-navigation-without-resolve -- resolve() supplies the path; the rule can't see through the appended query
    goto(`${resolve('/salaries')}${salariesSearch(next)}`, { reset: false });
  const setScope = (part: keyof SalariesQuery['scope'], value: string) =>
    go({ ...q, scope: { ...scope, [part]: value } });

  let addCategory = $state('');
  let addCountry = $state('');
  function addLine() {
    go({ ...q, compare: [...q.compare, { category: addCategory, country: addCountry }] });
    addCategory = '';
    addCountry = '';
  }
  const removeLine = (i: number) => go({ ...q, compare: q.compare.filter((_, j) => j !== i) });

  // The same scope as /jobs filters, so "Browse them" opens exactly the postings counted.
  const jobsQuery = $derived.by(() => {
    const p = Object.entries({ category: scope.category, seniority: scope.seniority, countries: scope.country })
      .filter(([, v]) => v)
      .map(([k, v]) => `${k}=${encodeURIComponent(v)}`)
      .join('&');
    return p ? `?${p}` : '';
  });
  const canonical = $derived(
    `${page.url.origin}${resolve('/salaries')}${salariesSearch({ scope, compare: [], unit: '' })}`,
  );
  const description = $derived(
    band
      ? `${scopeName}: median advertised salary ${formatSalary(band.p50, band.currency)} per ${band.period} across ${band.sample_size.toLocaleString('en-US')} open postings, with its distribution and monthly trend.`
      : `Advertised salaries for ${scopeName} on freehire: median, distribution and monthly trend, comparable across roles and countries.`,
  );
  // A distribution through a handful of postings is their list, not a shape: the
  // same floor a month needs on the chart.
  const HISTOGRAM_MIN = 30;
  const selectClass = 'rounded-md border border-input bg-background px-2 py-1.5 text-sm';
</script>

<Seo title="{scopeName} salaries · freehire" {description} {canonical} />

<div class="mx-auto w-full max-w-5xl px-4 py-6">
  <h1 class="text-2xl font-semibold tracking-tight">Salaries</h1>
  <p class="mt-1 text-sm text-muted-foreground">
    What open postings on freehire advertise, by role, grade and country. Compare roles and countries over time.
  </p>

  <div class="mt-4 flex flex-wrap items-end gap-3" role="group" aria-label="Scope">
    <label class="flex flex-col gap-1 text-xs text-muted-foreground"
      >Role
      <select class={selectClass} value={scope.category} onchange={(e) => setScope('category', e.currentTarget.value)}>
        <option value="">All roles</option>
        {#each categories as o (o.value)}<option value={o.value}>{o.label}</option>{/each}
      </select>
    </label>
    <label class="flex flex-col gap-1 text-xs text-muted-foreground"
      >Grade
      <select
        class={selectClass}
        value={scope.seniority}
        onchange={(e) => setScope('seniority', e.currentTarget.value)}
      >
        <option value="">All grades</option>
        {#each SENIORITY_OPTIONS as o (o.value)}<option value={o.value}>{o.label}</option>{/each}
      </select>
    </label>
    <label class="flex flex-col gap-1 text-xs text-muted-foreground"
      >Country
      <select class={selectClass} value={scope.country} onchange={(e) => setScope('country', e.currentTarget.value)}>
        <option value="">All countries</option>
        {#each countryOptions as c (c)}<option value={c}>{countryLabel(c)}</option>{/each}
      </select>
    </label>
    {#if units.length > 1}
      <label class="flex flex-col gap-1 text-xs text-muted-foreground"
        >Currency
        <select class={selectClass} value={unit} onchange={(e) => go({ ...q, unit: e.currentTarget.value })}>
          {#each units as u (u)}<option value={u}>{u.replace('-', ' / ')}</option>{/each}
        </select>
      </label>
    {/if}
  </div>

  <div class="mt-5 grid grid-cols-1 gap-3 sm:grid-cols-3">
    <div class="rounded-xl border border-border bg-card p-4">
      <p class="text-sm text-muted-foreground">Open jobs</p>
      <p class="mt-1 text-2xl font-semibold tabular-nums">
        {data.openJobs === null ? '—' : data.openJobs.toLocaleString('en-US')}
      </p>
      <!-- eslint-disable svelte/no-navigation-without-resolve -- resolve()d /jobs plus a query string -->
      <a
        href={`${resolve('/jobs')}${jobsQuery}`}
        class="mt-1 inline-block text-xs text-muted-foreground underline hover:text-foreground">Browse them</a
      >
      <!-- eslint-enable svelte/no-navigation-without-resolve -->
    </div>
    <div class="rounded-xl border border-border bg-card p-4">
      <p class="text-sm text-muted-foreground">Median salary</p>
      {#if band}
        <p class="mt-1 text-2xl font-semibold tabular-nums">
          {formatSalary(band.p50, band.currency)}<span class="text-sm font-normal text-muted-foreground">
            / {band.period}</span
          >
        </p>
        <p class="mt-1 text-xs text-muted-foreground tabular-nums">
          Middle half {formatSalary(band.p25, band.currency)} – {formatSalary(band.p75, band.currency)}
        </p>
      {:else}
        <p class="mt-1 text-sm text-muted-foreground">
          {#if data.bands === null}
            Salary figures are unavailable right now.
          {:else if bands.length}
            Too few open postings here state a salary in {currency} per {period}.
          {:else}
            Too few open postings state a salary here.
          {/if}
        </p>
      {/if}
    </div>
    <div class="rounded-xl border border-border bg-card p-4">
      <p class="text-sm text-muted-foreground">Postings with a salary</p>
      <p class="mt-1 text-2xl font-semibold tabular-nums">{band ? band.sample_size.toLocaleString('en-US') : '—'}</p>
      <p class="mt-1 text-xs text-muted-foreground">
        Open now, in {band ? `${band.currency} per ${band.period}` : 'any currency'}
      </p>
    </div>
  </div>

  {#if band}
    <section class="mt-5 rounded-xl border border-border bg-card p-4" aria-labelledby="dist-heading">
      <h2 id="dist-heading" class="text-base font-semibold">Salary distribution</h2>
      <p class="mb-3 text-xs text-muted-foreground">{scopeName}, {band.currency} per {band.period}</p>
      {#if band.sample_size >= HISTOGRAM_MIN}
        <SalaryHistogram quantiles={band.quantiles} currency={band.currency} period={band.period} />
      {:else}
        <p class="text-sm text-muted-foreground">
          {band.sample_size} postings are too few to draw a distribution; it needs {HISTOGRAM_MIN}.
        </p>
      {/if}
    </section>
  {/if}

  <section class="mt-5 rounded-xl border border-border bg-card p-4" aria-labelledby="compare-heading">
    <h2 id="compare-heading" class="text-base font-semibold">Over time</h2>
    <p class="mb-3 text-xs text-muted-foreground">
      Compare up to {MAX_COMPARE + 1} roles and countries{scope.seniority
        ? `, all at ${seniorityLabel(scope.seniority)}`
        : ''}. Every line is in {currency || 'one currency'} per {period || 'period'}: lines in other currencies cannot
      share a scale.
    </p>

    <div class="mb-3 flex flex-wrap items-end gap-2">
      {#each q.compare as l, i (`${l.category}.${l.country}`)}
        <span class="inline-flex items-center gap-1 rounded-full border border-border px-2.5 py-1 text-sm">
          {lineName(l.category, l.country)}
          <button
            type="button"
            class="text-muted-foreground hover:text-foreground"
            aria-label="Remove {lineName(l.category, l.country)}"
            onclick={() => removeLine(i)}>×</button
          >
        </span>
      {/each}
      {#if q.compare.length < MAX_COMPARE}
        <select class={selectClass} bind:value={addCategory} aria-label="Role to compare">
          <option value="">All roles</option>
          {#each categories as o (o.value)}<option value={o.value}>{o.label}</option>{/each}
        </select>
        <select class={selectClass} bind:value={addCountry} aria-label="Country to compare">
          <option value="">All countries</option>
          {#each data.countries as c (c)}<option value={c}>{countryLabel(c)}</option>{/each}
        </select>
        <button
          type="button"
          class="rounded-md border border-border px-3 py-1.5 text-sm hover:bg-accent"
          onclick={addLine}>Add to compare</button
        >
      {/if}
    </div>

    {#if q.compare.length === 0}
      <SalaryHistoryChart
        rows={(data.histories[0] ?? []).filter((r) => unitOf(r) === unit)}
        currentMonth={data.currentMonth}
      />
      {#if lines[0]?.months.length === 0}
        <p class="text-sm text-muted-foreground">Fewer than three months have enough postings with a salary here.</p>
      {/if}
    {:else}
      <SalaryCompareChart {lines} {currency} {period} currentMonth={data.currentMonth} />
    {/if}
  </section>

  <p class="mt-4 text-xs text-muted-foreground">
    Figures are advertised salaries of postings on freehire, not the whole job market. Currencies are never converted or
    combined. The median of open postings needs at least 5 with a salary; the distribution and each month on the chart
    need {HISTOGRAM_MIN}. Anything thinner is left out rather than guessed.
  </p>
</div>
