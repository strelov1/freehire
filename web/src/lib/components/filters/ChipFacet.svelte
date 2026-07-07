<script lang="ts">
  import { FACETS, type FacetStore } from '$lib/facets';
  import type { FacetCounts } from '$lib/types';
  import FacetHeader from './FacetHeader.svelte';
  import PillGroup from '../facets/PillGroup.svelte';

  // One chip facet inside a modal pane: a FacetHeader (label + Clear) over a
  // PillGroup — the same per-facet controls the sidebar offers. Options and the
  // excludable flag come from the registry (by `param`), so a caller only names the
  // facet. Excludable facets cycle each pill off → include → exclude → off. When
  // `counts` is passed, each pill shows its live match count under the current scope.
  let { store, param, label, counts = null }: { store: FacetStore; param: string; label: string; counts?: FacetCounts | null } = $props();

  const def = FACETS.find((d) => d.param === param);
  const excludable = def?.excludable ?? false;
  const st = $derived(store.facet(param));
  const onToggle = (v: string) => (excludable ? store.cycle(param, v) : store.pick(param, v));

  // Merge the live distribution counts into the static registry options.
  const options = $derived.by(() => {
    const dist = counts?.facets?.[param];
    const base = def?.options ?? [];
    return dist ? base.map((o) => ({ ...o, count: dist[o.value] ?? 0 })) : base;
  });
</script>

<div>
  <FacetHeader {store} {param} {label} />
  <PillGroup {options} include={st.include} exclude={st.exclude} {excludable} {onToggle} />
</div>
