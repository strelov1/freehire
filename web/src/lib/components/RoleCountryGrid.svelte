<script lang="ts">
  import { resolve } from '$app/paths';
  import type { InsightRoleCountries } from '#lib/api.js';
  import { countryLabel } from '#lib/facets.js';
  import { categoryLabel } from '#lib/labels.js';
  import {
    ALL_COUNTRIES,
    COVERAGE_JUMP_RATIO,
    GROWTH_MIN_BASE,
    VISIBLE_ROWS,
    buildGrid,
    cellTarget,
    columnShare,
    growthLevel,
    growthPercent,
    roundedGrowth,
    shareLevel,
    type CellTarget,
    type GridCell,
  } from '#lib/roleCountryGrid.js';

  let { data }: { data: InsightRoleCountries } = $props();

  const grid = $derived(buildGrid(data.cells, data.countries));

  // Both numbers are always printed, so the table reads the same without JavaScript;
  // the switch only chooses what the colour encodes.
  let shading = $state<'open' | 'growth'>('open');
  const SHADINGS = [
    ['open', 'Share of country'],
    ['growth', '30-day change'],
  ] as const;
  // Every row is in the HTML — crawlers and no-JS readers get all of them; "show all"
  // only unhides the tail on screen.
  let showAll = $state(false);

  // Static class strings, so Tailwind's scan sees every one; token-based, so `.dark`
  // moves them (the same reasoning as ActivityYear's SHADES).
  const OPEN_SHADES = ['', 'bg-primary/5', 'bg-primary/10', 'bg-primary/15', 'bg-primary/25'];
  const GROWTH_SHADES: Record<number, string> = {
    [-3]: 'bg-destructive/30',
    [-2]: 'bg-destructive/15',
    [-1]: 'bg-destructive/5',
    0: '',
    1: 'bg-success/10',
    2: 'bg-success/25',
    3: 'bg-success/40',
  };

  const compact = new Intl.NumberFormat('en-US', { notation: 'compact', maximumFractionDigits: 1 });
  const full = new Intl.NumberFormat('en-US');

  // A country whose coverage jumped prints no change anywhere: its cells would report
  // freehire's onboarding, not the market (COVERAGE_JUMP_RATIO says why).
  const jumped = (country: string) => grid.coverageJumps.has(country);

  function shadeOf(country: string, cell: GridCell): string {
    if (shading === 'growth') return jumped(country) ? '' : (GROWTH_SHADES[growthLevel(growthPercent(cell))] ?? '');
    return OPEN_SHADES[shareLevel(columnShare(cell.open, grid.columnTotals.get(country) ?? 0), grid.maxShare)] ?? '';
  }

  // Red and green only where they are the encoding: in the share view the cell colour
  // already says something else, so the change stays grey (its ▲/▼ still reads).
  function growthText(country: string, cell: GridCell): { text: string; tone: string } {
    const rounded = jumped(country) ? null : roundedGrowth(cell);
    if (rounded === null) return { text: '—', tone: 'text-muted-foreground' };
    if (rounded === 0) return { text: '0%', tone: 'text-muted-foreground' };
    const text = rounded > 0 ? `▲ ${rounded}%` : `▼ ${-rounded}%`;
    if (shading !== 'growth') return { text, tone: 'text-muted-foreground' };
    return { text, tone: rounded > 0 ? 'text-success-strong' : 'text-destructive-strong' };
  }

  /** What a cell says to a screen reader: the visible "12K ▲ 5%" reads as "up-pointing
   *  triangle", and a title attribute is not reliably announced. */
  function cellLabel(category: string, country: string, cell: GridCell): string {
    const rounded = roundedGrowth(cell);
    let change: string;
    if (jumped(country)) change = `change not shown: postings for ${columnName(country)} grew at least ${COVERAGE_JUMP_RATIO}× in 30 days, likely new coverage`;
    else if (rounded === null) change = `fewer than ${GROWTH_MIN_BASE} 30 days ago`;
    else if (rounded === 0) change = 'unchanged over 30 days';
    else change = `${rounded > 0 ? 'up' : 'down'} ${Math.abs(rounded)}% over 30 days`;
    return `${categoryLabel(category)}, ${columnName(country)}: ${full.format(cell.open)} open, ${change}`;
  }

  function hrefOf(target: CellTarget): string {
    switch (target.kind) {
      case 'pair':
        return resolve('/roles/[category]/[country]', { category: target.category, country: target.country });
      case 'category':
        return resolve('/roles/[category]', { category: target.category });
      case 'search': {
        const category = `category=${encodeURIComponent(target.category)}`;
        const country = target.country ? `&countries=${encodeURIComponent(target.country)}` : '';
        return `${resolve('/jobs')}?${category}${country}`;
      }
    }
  }

  const columnName = (country: string) => (country === ALL_COUNTRIES ? 'All countries' : countryLabel(country));
</script>

{#if grid.rows.length > 0}
  <section class="mt-8" aria-labelledby="role-country-heading">
    <div class="flex flex-wrap items-end justify-between gap-3">
      <div>
        <h2 id="role-country-heading" class="text-xl font-semibold text-foreground">Where each role is hiring</h2>
        <p class="mt-0.5 text-sm text-muted-foreground">Open roles by country, and how that changed over 30 days.</p>
      </div>
      <div class="inline-flex rounded-md border border-border p-0.5 text-sm" role="group" aria-label="Shade cells by">
        {#each SHADINGS as [value, label] (value)}
          <button
            type="button"
            class="rounded px-2.5 py-1 {shading === value ? 'bg-muted font-medium text-foreground' : 'text-muted-foreground'}"
            aria-pressed={shading === value}
            onclick={() => (shading = value)}>{label}</button
          >
        {/each}
      </div>
    </div>

    <div class="mt-4 overflow-x-auto rounded-lg border border-border">
      <table class="w-full border-collapse text-sm tabular-nums">
        <caption class="sr-only">
          Open roles per role and country, each with its change over 30 days
        </caption>
        <thead>
          <tr class="border-b border-border text-left text-xs text-muted-foreground">
            <th scope="col" class="sticky left-0 z-10 bg-background px-3 py-2 font-medium">Role</th>
            {#each grid.columns as country (country)}
              <th scope="col" class="px-2 py-2 text-right font-medium {country === ALL_COUNTRIES ? 'border-r border-border' : ''}">
                {#if country === ALL_COUNTRIES}
                  {columnName(country)}
                {:else}
                  <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- internal /jobs filter link; resolve()d base plus a query string, no dynamic route to resolve -->
                  <a href={`${resolve('/jobs')}?countries=${encodeURIComponent(country)}`} class="hover:text-foreground hover:underline">
                    {columnName(country)}
                  </a>
                {/if}
                {#if jumped(country)}
                  <span title="Postings grew {COVERAGE_JUMP_RATIO}× or more in 30 days, likely new coverage; change not shown"
                    ><span aria-hidden="true">*</span><span class="sr-only"> (coverage grew, change not shown)</span></span
                  >
                {/if}
              </th>
            {/each}
          </tr>
        </thead>
        <tbody id="role-country-rows">
          {#each grid.rows as row, i (row.category)}
            <tr class="border-b border-border last:border-b-0 {i >= VISIBLE_ROWS && !showAll ? 'hidden' : ''}">
              <th scope="row" class="sticky left-0 z-10 bg-background px-3 py-1.5 text-left font-medium whitespace-nowrap">
                <a href={resolve('/insights/roles/[category]', { category: row.category })} class="text-foreground hover:underline">
                  {categoryLabel(row.category)}
                </a>
              </th>
              {#each grid.columns as country (country)}
                {@const cell = row.cells.get(country)}
                <td class="p-0 text-right {country === ALL_COUNTRIES ? 'border-r border-border' : ''}">
                  {#if cell}
                    {@const growth = growthText(country, cell)}
                    <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- hrefOf builds every target with resolve(); the rule cannot see through the call -->
                    <a href={hrefOf(cellTarget(row.category, country, cell.open))}
                      aria-label={cellLabel(row.category, country, cell)}
                      title="{categoryLabel(row.category)} · {columnName(country)}: {full.format(cell.open)} open, {full.format(cell.prev)} 30 days ago"
                      class="block px-2 py-1.5 transition-colors hover:outline hover:outline-1 hover:-outline-offset-1 hover:outline-foreground {shadeOf(country, cell)}"
                    >
                      <span class="block font-medium text-foreground">{compact.format(cell.open)}</span>
                      <span class="block text-xs whitespace-nowrap {growth.tone}">{growth.text}</span>
                    </a>
                  {:else}
                    <span class="block px-2 py-1.5 text-muted-foreground"><span aria-hidden="true">·</span><span class="sr-only">none</span></span>
                  {/if}
                </td>
              {/each}
            </tr>
          {/each}
        </tbody>
      </table>
    </div>

    {#if grid.rows.length > VISIBLE_ROWS}
      <button
        type="button"
        class="mt-2 text-sm text-brand-strong hover:underline"
        aria-expanded={showAll}
        aria-controls="role-country-rows"
        onclick={() => (showAll = !showAll)}
      >
        {showAll ? `Show top ${VISIBLE_ROWS} roles` : `Show all ${grid.rows.length} roles`}
      </button>
    {/if}

    <p class="mt-2 text-xs text-muted-foreground">
      {shading === 'open'
        ? 'Darker means a larger share of that country’s open roles.'
        : 'Green grew, red shrank over 30 days.'}
      Change compares postings by when they were published, and shows "—" when there were
      fewer than {GROWTH_MIN_BASE} a month ago. These are postings on freehire, not the whole
      job market.
      {#if grid.coverageJumps.size > 0}
        * postings freehire carries for that country grew {COVERAGE_JUMP_RATIO}× or more in 30
        days — a jump that size is usually new sources rather than hiring, and the two cannot be
        told apart, so its change is not shown.
      {/if}
    </p>
  </section>
{/if}
