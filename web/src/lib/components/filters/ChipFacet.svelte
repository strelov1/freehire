<script lang="ts">
  import { FACETS, type FacetStore } from '$lib/facets';
  import FacetHeader from './FacetHeader.svelte';
  import PillGroup from '../facets/PillGroup.svelte';

  // One chip facet inside a modal pane: a FacetHeader (label + Clear) over a
  // PillGroup — the same per-facet controls the sidebar offers. Options and the
  // excludable flag come from the registry (by `param`), so a caller only names the
  // facet. Excludable facets cycle each pill off → include → exclude → off.
  let { store, param, label }: { store: FacetStore; param: string; label: string } = $props();

  const def = FACETS.find((d) => d.param === param);
  const options = def?.options ?? [];
  const excludable = def?.excludable ?? false;
  const st = $derived(store.facet(param));
  const onToggle = (v: string) => (excludable ? store.cycle(param, v) : store.pick(param, v));
</script>

<div>
  <FacetHeader {store} {param} {label} />
  <PillGroup {options} include={st.include} exclude={st.exclude} {excludable} {onToggle} />
</div>
