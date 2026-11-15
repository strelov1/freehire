<script lang="ts">
  import { SALARY_MAX, SALARY_STEP, salaryLabel } from '#lib/filterControls.js';
  import type { StagedFilters } from '#lib/stagedFilters.svelte.js';
  import type { FacetCounts } from '#lib/types.js';
  import ChipFacet from './ChipFacet.svelte';
  import RangeSlider from './RangeSlider.svelte';

  // The filter modal's Salary pane: currency, the salary range and equity, over the
  // staged filters (`store`).
  let { store, counts }: { store: StagedFilters; counts: FacetCounts | null } = $props();

  // "No ceiling" is a stop of its own, one step past the highest real bound — the
  // shape of the "Any" stop on the experience and freshness sliders. Letting
  // SALARY_MAX double as it would make SALARY_MAX unreachable as a ceiling, and,
  // since the thumbs stop a step apart, as a floor too.
  const SALARY_NO_CEILING = SALARY_MAX + SALARY_STEP;

  // What a salary thumb announces: each end of the track is "no bound", so the raw
  // 0 / SALARY_NO_CEILING would mislead a screen reader into a real figure.
  function salaryThumbText(n: number, thumb: 'lo' | 'hi'): string {
    if (thumb === 'lo' && n <= 0) return 'No minimum';
    if (thumb === 'hi' && n >= SALARY_NO_CEILING) return 'No ceiling';
    return n.toLocaleString('en-US');
  }
</script>

<ChipFacet {store} param="salary_currency" label="Currency" {counts} />
<div class="mb-2 mt-6 flex items-center justify-between">
  <h3 class="text-sm font-semibold tracking-tight">Salary range</h3>
  <span class="text-xs font-medium text-muted-foreground"
    >{salaryLabel(store.value.salaryMin, store.value.salaryMax)}</span
  >
</div>
<!-- Each end of the track is "no bound", so a thumb parked there writes nothing. -->
<RangeSlider
  min={0}
  max={SALARY_NO_CEILING}
  step={SALARY_STEP}
  lo={store.value.salaryMin ?? 0}
  hi={store.value.salaryMax ?? SALARY_NO_CEILING}
  loLabel="Minimum salary"
  hiLabel="Maximum salary"
  onLo={(n) => store.setSalaryMin(n > 0 ? n : null)}
  onHi={(n) => store.setSalaryMax(n < SALARY_NO_CEILING ? n : null)}
  valueText={salaryThumbText}
/>
<!-- The API bounds each end of a posting's own range (its minimum from below, its
     maximum from above), so a posting paying 90,000–160,000 is outside a
     100,000–150,000 search. Said up front, for the same reason as the experience
     note: a count that drops without explanation reads as a broken filter. -->
<p class="mt-2 text-xs text-muted-foreground">
  Matches postings whose stated salary range falls entirely within these bounds.
</p>
<!-- Beside the salary because it is the other half of the pay: a posting can offer
     equity with no salary figure at all, so this is not a refinement of the range. -->
<div class="mt-6">
  <ChipFacet {store} param="equity" label="Equity" {counts} />
</div>
<p class="mt-2 text-xs text-muted-foreground">
  Not stated means the posting says nothing about equity, not that it offers none.
</p>
