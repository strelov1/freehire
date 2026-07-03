<script lang="ts">
  import { COMPANY_FACETS, type FacetDef } from '$lib/facets';
  import type { CompanyFilterStore } from '$lib/companyFilters';
  import FilterSummaryShell, { type SummaryGroup } from './FilterSummaryShell.svelte';

  // The companies filters sidebar: a summary of the applied company facets as chips,
  // over the reusable FilterSummaryShell (the company counterpart of FilterSummary).
  // Company facets are include-only and flat (one group per facet); removing a chip
  // applies immediately. `onOpen` opens the shared CompanyFilterModal.
  let { store, onOpen }: { store: CompanyFilterStore; onOpen: () => void } = $props();

  function valueLabel(def: FacetDef, value: string): string {
    return def.options?.find((o) => o.value === value)?.label ?? value;
  }

  const groups = $derived.by((): SummaryGroup[] =>
    COMPANY_FACETS.map((def) => ({
      label: def.label,
      chips: store.facet(def.param).include.map((v) => ({
        text: valueLabel(def, v),
        exclude: false,
        remove: () => store.remove(def.param, v),
      })),
    })).filter((g) => g.chips.length > 0),
  );
</script>

<FilterSummaryShell {groups} active={store.active} onReset={() => store.clear()} {onOpen} />
