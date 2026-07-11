<script lang="ts">
  import { dynamicLabel, FACETS } from '$lib/facets';
  import { type FilterStore, savedSearchQuery } from '$lib/filters';
  import { freshnessLabel } from '$lib/filterControls';
  import FilterSummaryShell, { type SummaryChip, type SummaryGroup } from './FilterSummaryShell.svelte';
  import SaveSearchAlert from './SaveSearchAlert.svelte';

  // The job filters sidebar: a summary of the *applied* filters as chips grouped by
  // facet, over the reusable FilterSummaryShell. Removing a chip edits the live store
  // directly — the sidebar applies immediately; only the modal defers. The shared
  // save-then-alert control (Save filter → get it in Telegram) sits under the
  // All-filters button on the standalone list (`canSave`); the modal's "My filters"
  // tab drives the same control.
  let { store, exclude = [], onOpen, canSave = false }: { store: FilterStore; exclude?: string[]; onOpen: () => void; canSave?: boolean } = $props();

  // The current filters as the saved-search / alert target (view-only sort dropped).
  const current = $derived(savedSearchQuery(store.value));

  function valueLabel(param: string, value: string): string {
    const def = FACETS.find((d) => d.param === param);
    const o = def?.options?.find((opt) => opt.value === value);
    return o ? o.label : dynamicLabel(param, value);
  }

  const groups = $derived.by((): SummaryGroup[] => {
    const f = store.value;
    const out: SummaryGroup[] = [];
    const push = (label: string, chips: SummaryChip[]) => {
      if (chips.length) out.push({ label, chips });
    };
    // Chips for one facet: included values first, then excluded (destructive style).
    const facetChips = (param: string): SummaryChip[] => {
      const st = f.facets[param];
      if (!st) return [];
      return [
        ...st.include.map((v) => ({ text: valueLabel(param, v), exclude: false, remove: () => store.remove(param, v) })),
        ...st.exclude.map((v) => ({ text: valueLabel(param, v), exclude: true, remove: () => store.remove(param, v) })),
      ];
    };
    const facetGroup = (param: string, label: string) => {
      if (exclude.includes(param)) return;
      push(label, facetChips(param));
    };

    // The text query (from the header search on the standalone list) as a removable
    // chip, so the sidebar shows what you searched, not just the facet filters.
    push('Search', f.q.trim() ? [{ text: f.q, exclude: false, remove: () => store.setQuery('') }] : []);

    facetGroup('category', 'Specialization');
    // Location: regions + countries + cities under one heading.
    push('Location', [...facetChips('regions'), ...facetChips('countries'), ...facetChips('cities')]);

    facetGroup('seniority', 'Seniority');
    facetGroup('work_mode', 'Work format');
    facetGroup('skills', 'Skills');
    facetGroup('domains', 'Industry');
    facetGroup('company_type', 'Company type');
    facetGroup('employment_type', 'Employment');
    facetGroup('collections', 'Collection');
    facetGroup('company_slug', 'Company');
    facetGroup('english_level', 'English');
    facetGroup('posting_language', 'Job language');
    facetGroup('relocation', 'Relocation');

    // Salary: currency + minimum.
    const salary: SummaryChip[] = facetChips('salary_currency');
    if (f.salaryMin != null) salary.push({ text: `${f.salaryMin.toLocaleString('en-US')}+`, exclude: false, remove: () => store.setSalaryMin(null) });
    push('Salary', salary);

    if (f.visa) push('Visa', [{ text: 'Sponsorship', exclude: false, remove: () => store.setVisa(false) }]);
    if (f.postedWithinDays != null) push('Posted', [{ text: freshnessLabel(f.postedWithinDays), exclude: false, remove: () => store.setPostedWithinDays(null) }]);

    return out;
  });
</script>

<FilterSummaryShell {groups} active={store.active} onReset={() => store.clear()} {onOpen}>
  {#snippet afterButton()}
    {#if canSave}
      <SaveSearchAlert query={current} variant="full" />
    {/if}
  {/snippet}
</FilterSummaryShell>
