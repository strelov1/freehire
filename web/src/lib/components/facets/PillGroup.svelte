<script lang="ts">
  import { uniqueByValue, type FacetOption } from '$lib/facets';
  import { pillClass, pillTitle } from './pill';

  // A wrap of three-state pills for one facet. Each pill is off (idle), included
  // (primary fill), or excluded (muted destructive, struck through). Clicking calls
  // `onToggle`, which the parent wires to cycle (excludable facets: off → include →
  // exclude → off) or to a plain include toggle (non-excludable). Stateless —
  // selection and the transition come from the store.
  let {
    options,
    include,
    exclude = [],
    excludable = true,
    onToggle,
  }: {
    options: FacetOption[];
    include: string[];
    exclude?: string[];
    excludable?: boolean;
    onToggle: (value: string) => void;
  } = $props();

  // A selected value with no matching option — e.g. a vocabulary value removed
  // since an old bookmark or saved search was created — still renders as a pill so
  // it stays removable instead of becoming an invisible, stuck filter that silently
  // constrains results.
  const shown = $derived(
    uniqueByValue([
      ...options,
      ...[...include, ...exclude]
        .filter((v) => !options.some((o) => o.value === v))
        .map((v) => ({ value: v, label: v })),
    ]),
  );
</script>

<div class="flex flex-wrap gap-2">
  {#each shown as opt (opt.value)}
    {@const excluded = exclude.includes(opt.value)}
    {@const included = include.includes(opt.value)}
    <button
      type="button"
      onclick={() => onToggle(opt.value)}
      title={pillTitle(included, excluded, excludable)}
      class={pillClass(included || excluded, excluded, 'px-3 py-1.5 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50')}
    >
      {opt.label}
    </button>
  {/each}
</div>
