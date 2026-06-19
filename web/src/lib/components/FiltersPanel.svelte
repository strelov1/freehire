<script lang="ts">
  import type { FilterStore } from '$lib/filters.svelte';
  import type { FacetCounts } from '$lib/types';
  import { FACETS, type FacetDef } from '$lib/facets';
  import FacetSection from './facets/FacetSection.svelte';
  import SavedSearches from './SavedSearches.svelte';

  // The panel is pure presentation over the store: it iterates the facet
  // registry and renders each section, plus the two special controls (visa,
  // min salary) that aren't multi-value facets. `exclude` hides facets by param
  // (e.g. the company page pins one company, so its Source facet is irrelevant).
  // `counts` is the live facet distribution feeding the dynamic selects.
  let { store, exclude = [], counts = null }: { store: FilterStore; exclude?: string[]; counts?: FacetCounts | null } = $props();

  const facets = $derived(FACETS.filter((f) => !exclude.includes(f.param)));

  // A facet is "active" when it carries a current selection.
  const isActive = (param: string) => store.facet(param).values.length > 0;

  // Facets with an applied filter float to the top; the rest keep registry order
  // below. Recomputed live off store.value, so a section rises the moment it
  // gets its first value (the {#each} is keyed by param, so Svelte moves the
  // existing node rather than re-creating it — open selects/inputs survive).
  const orderedFacets = $derived.by((): FacetDef[] => [
    ...facets.filter((d) => isActive(d.param)),
    ...facets.filter((d) => !isActive(d.param)),
  ]);

  // Slider bounds for the min-salary filter. 0 means "no minimum".
  const SALARY_MAX = 300000;
  const SALARY_STEP = 5000;

  function onSalaryInput(e: Event) {
    const n = Number((e.currentTarget as HTMLInputElement).value);
    store.setSalaryMin(n === 0 ? null : n);
  }
</script>

<div class="flex flex-col gap-4">
  <SavedSearches {store} />

  <div class="flex items-center justify-between">
    <h2 class="text-base font-semibold tracking-tight">Filters</h2>
    {#if store.active > 0}
      <button type="button" class="text-xs text-muted-foreground transition-colors hover:text-foreground" onclick={() => store.clear()}>
        Reset all
      </button>
    {/if}
  </div>

  {#each orderedFacets as def (def.param)}
    <FacetSection {def} {store} {counts} />
  {/each}

  <div class="border-b border-border pb-4">
    <div class="mb-2 flex items-center justify-between">
      <h3 class="text-sm font-semibold tracking-tight">Min salary</h3>
      <span class="text-xs font-medium text-muted-foreground">
        {store.value.salaryMin ? `${store.value.salaryMin.toLocaleString('en-US')}+` : 'Any'}
      </span>
    </div>
    <input
      type="range"
      min="0"
      max={SALARY_MAX}
      step={SALARY_STEP}
      value={store.value.salaryMin ?? 0}
      oninput={onSalaryInput}
      aria-label="Minimum salary"
      class="w-full accent-primary"
    />
    <div class="mt-1 flex justify-between text-[10px] text-muted-foreground">
      <span>Any</span>
      <span>{SALARY_MAX.toLocaleString('en-US')}+</span>
    </div>
  </div>

  <div>
    <h3 class="mb-2 text-sm font-semibold tracking-tight">Visa</h3>
    <label class="flex cursor-pointer items-center gap-2 text-sm">
      <input
        type="checkbox"
        class="size-4 rounded border-border"
        checked={store.value.visa}
        onchange={(e) => store.setVisa(e.currentTarget.checked)}
      />
      <span>Visa sponsorship</span>
    </label>
  </div>
</div>
