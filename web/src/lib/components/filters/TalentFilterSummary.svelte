<script lang="ts">
  import { TALENT_FACETS, cityLabel, skillLabel, type FacetDef } from '$lib/facets';
  import type { TalentFilterStore } from '$lib/talentFilters';
  import FilterSummaryShell, { type SummaryGroup } from './FilterSummaryShell.svelte';

  // What is currently narrowing the catalogue, as removable chips — the catalogue
  // counterpart of FilterSummary and CompanyFilterSummary, over the same shell.
  //
  // Catalogue facets are include-only and flat (one group per facet); removing a chip
  // applies immediately. `onOpen` opens TalentFilterModal.
  let { store, onOpen }: { store: TalentFilterStore; onOpen: () => void } = $props();

  // A chip has to read as the thing the visitor picked. The closed vocabularies carry
  // their own option list; the two DYNAMIC ones do not — their values come from the live
  // distribution, so there is no `options` to look a label up in and the slug would show
  // raw. `berlin` beside a timezone's own `Berlin` looks like a bug in the half that is
  // correct, which is exactly what it looked like before.
  function valueLabel(def: FacetDef, value: string): string {
    const fromOptions = def.options?.find((o) => o.value === value)?.label;
    if (fromOptions) return fromOptions;
    if (def.param === 'skills') return skillLabel(value);
    if (def.param === 'cities') return cityLabel(value);
    return value;
  }

  const groups = $derived.by((): SummaryGroup[] => {
    const facetGroups: SummaryGroup[] = TALENT_FACETS.map((def) => ({
      label: def.label,
      chips: store.facet(def.param).include.map((v) => ({
        key: `${def.param}:${v}`,
        text: valueLabel(def, v),
        exclude: false,
        remove: () => store.remove(def.param, v),
      })),
    }));

    // Years is not a facet — one value or none — but it narrows the list, so it belongs
    // in the summary. A filter a visitor cannot see is one they cannot undo.
    const years = store.value.minYears;
    if (years) {
      facetGroups.push({
        label: 'Experience',
        chips: [
          {
            key: `min_years:${years}`,
            text: `${years}+ years`,
            exclude: false,
            remove: () => store.setMinYears(undefined),
          },
        ],
      });
    }

    return facetGroups.filter((g) => g.chips.length > 0);
  });
</script>

<FilterSummaryShell {groups} active={store.active} onReset={() => store.clear()} {onOpen} />
