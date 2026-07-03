<script lang="ts">
  import { SlidersHorizontal, X } from '@lucide/svelte';
  import { dynamicLabel, FACETS } from '$lib/facets';
  import type { FilterStore } from '$lib/filters';
  import { freshnessLabel } from '$lib/filterControls';
  import SavedSearches from '../SavedSearches.svelte';

  // The sidebar: a summary of the *applied* filters as chips grouped by facet, plus
  // the All-filters button (opening the modal) and Reset all. Removing a chip edits
  // the live store directly — the sidebar applies immediately; only the modal defers.
  let { store, exclude = [], onOpen }: { store: FilterStore; exclude?: string[]; onOpen: () => void } = $props();

  interface Chip {
    text: string;
    exclude: boolean;
    remove: () => void;
  }
  interface Group {
    label: string;
    chips: Chip[];
  }

  function valueLabel(param: string, value: string): string {
    const def = FACETS.find((d) => d.param === param);
    const o = def?.options?.find((opt) => opt.value === value);
    return o ? o.label : dynamicLabel(param, value);
  }

  const groups = $derived.by((): Group[] => {
    const f = store.value;
    const out: Group[] = [];
    const push = (label: string, chips: Chip[]) => {
      if (chips.length) out.push({ label, chips });
    };
    // Chips for one facet: included values first, then excluded (destructive style).
    const facetChips = (param: string): Chip[] => {
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
    const salary: Chip[] = facetChips('salary_currency');
    if (f.salaryMin != null) salary.push({ text: `${f.salaryMin.toLocaleString('en-US')}+`, exclude: false, remove: () => store.setSalaryMin(null) });
    push('Salary', salary);

    if (f.visa) push('Visa', [{ text: 'Sponsorship', exclude: false, remove: () => store.setVisa(false) }]);
    if (f.postedWithinDays != null) push('Posted', [{ text: freshnessLabel(f.postedWithinDays), exclude: false, remove: () => store.setPostedWithinDays(null) }]);

    return out;
  });
</script>

<div class="flex flex-col gap-4">
  <SavedSearches {store} />

  <div class="flex items-center justify-between">
    <h2 class="text-base font-semibold tracking-tight">Filters</h2>
    {#if store.active > 0}
      <button type="button" class="text-xs text-muted-foreground transition-colors hover:text-foreground" onclick={() => store.clear()}>Reset all</button>
    {/if}
  </div>

  <button
    type="button"
    onclick={onOpen}
    class="flex h-11 w-full items-center justify-center gap-2 rounded-xl border border-border bg-background text-sm font-medium transition-colors hover:bg-accent"
  >
    <SlidersHorizontal class="size-4" />
    <span>All filters</span>
    {#if store.active > 0}
      <span class="inline-flex h-5 min-w-5 items-center justify-center rounded-full bg-primary px-1.5 text-[11px] font-semibold text-primary-foreground">{store.active}</span>
    {/if}
  </button>

  {#if groups.length === 0}
    <div class="rounded-xl border border-dashed border-border p-4 text-center text-sm text-muted-foreground">
      No filters yet.<br />Open <b>All filters</b> to refine.
    </div>
  {:else}
    {#each groups as g (g.label)}
      <div class="flex flex-col gap-2">
        <span class="text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">{g.label}</span>
        <div class="flex flex-wrap gap-1.5">
          {#each g.chips as c (c.text)}
            <span
              class={[
                'inline-flex items-center gap-1 rounded-full border px-2.5 py-1 text-xs font-medium',
                c.exclude ? 'border-destructive/30 bg-destructive/15 text-destructive line-through' : 'border-border bg-secondary text-secondary-foreground',
              ]}
            >
              {c.text}
              <button type="button" aria-label="Remove {c.text}" onclick={c.remove} class="text-muted-foreground transition-colors hover:text-foreground">
                <X class="size-3" />
              </button>
            </span>
          {/each}
        </div>
      </div>
    {/each}
  {/if}
</div>
