<script lang="ts">
  import { uniqueByValue, type FacetOption } from '$lib/facets';
  import { pillClass } from './pill';

  // A wrap of toggle pills for one facet. Selected pills fill with the primary
  // color; in exclude mode they take the muted destructive treatment to signal
  // "filter these out". Stateless — selection and toggle come from the store.
  let {
    options,
    selected,
    exclude = false,
    onToggle,
  }: {
    options: FacetOption[];
    selected: string[];
    exclude?: boolean;
    onToggle: (value: string) => void;
  } = $props();

  // A selected value with no matching option — e.g. a vocabulary value removed
  // since an old bookmark or saved search was created — still renders as an
  // active pill so it stays removable instead of becoming an invisible, stuck
  // filter that silently constrains results.
  const shown = $derived(
    uniqueByValue([
      ...options,
      ...selected.filter((v) => !options.some((o) => o.value === v)).map((v) => ({ value: v, label: v })),
    ]),
  );
</script>

<div class="flex flex-wrap gap-2">
  {#each shown as opt (opt.value)}
    {@const active = selected.includes(opt.value)}
    <button
      type="button"
      onclick={() => onToggle(opt.value)}
      class={pillClass(active, exclude, 'px-3 py-1.5 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50')}
    >
      {opt.label}
    </button>
  {/each}
</div>
