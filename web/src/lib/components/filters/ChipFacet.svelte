<script lang="ts">
  import { FACETS, type FacetStore } from '$lib/facets';
  import FacetHeader from './FacetHeader.svelte';
  import PillGroup from '../facets/PillGroup.svelte';

  // One chip facet inside a modal pane: a FacetHeader (label + Exclude + Clear) over a
  // PillGroup — the same per-facet controls the sidebar offers. Options come from the
  // registry (by `param`), so a caller only names the facet.
  let { store, param, label }: { store: FacetStore; param: string; label: string } = $props();

  const options = FACETS.find((d) => d.param === param)?.options ?? [];
  const st = $derived(store.facet(param));
</script>

<div>
  <FacetHeader {store} {param} {label} />
  <PillGroup {options} selected={st.values} exclude={st.exclude} onToggle={(v) => store.toggle(param, v)} />
</div>
