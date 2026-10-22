<script lang="ts">
  import { FACETS, type FacetOption, type FacetStore } from '#lib/facets.js';
  import type { FacetCounts } from '#lib/types.js';
  import FacetHeader from './FacetHeader.svelte';
  import PillGroup from '../facets/PillGroup.svelte';

  // One chip facet inside a modal pane: a FacetHeader (label + Clear) over a
  // PillGroup — the same per-facet controls the sidebar offers. Options and the
  // excludable flag come from the registry (by `param`), so a caller only names the
  // facet. Excludable facets cycle each pill off → include → exclude → off. When
  // `counts` is passed, each pill shows its live match count under the current scope.
  //
  // `options`, when passed, overrides the registry's list — for a param rendered as
  // more than one block (e.g. `collections` split between Industry & collection and
  // Relocation). It scopes the header's count and Clear to just that subset too, so a
  // block only reports and clears the values it actually shows, not the whole param.
  let {
    store,
    param,
    label,
    counts = null,
    options: optionsOverride,
  }: { store: FacetStore; param: string; label: string; counts?: FacetCounts | null; options?: FacetOption[] } =
    $props();

  const def = FACETS.find((d) => d.param === param);
  const excludable = def?.excludable ?? false;
  const st = $derived(store.facet(param));
  const scoped = $derived(optionsOverride !== undefined);
  const baseOptions = $derived(optionsOverride ?? def?.options ?? []);
  const scopedValues = $derived(new Set(baseOptions.map((o) => o.value)));
  const include = $derived(scoped ? st.include.filter((v) => scopedValues.has(v)) : st.include);
  const exclude = $derived(scoped ? st.exclude.filter((v) => scopedValues.has(v)) : st.exclude);
  const onToggle = (v: string) => (excludable ? store.cycle(param, v) : store.pick(param, v));

  // Merge the live distribution counts into the static registry options.
  const options = $derived.by(() => {
    const dist = counts?.facets?.[param];
    return dist ? baseOptions.map((o) => ({ ...o, count: dist[o.value] ?? 0 })) : baseOptions;
  });
</script>

<div>
  <FacetHeader {store} {param} {label} values={scoped ? [...scopedValues] : undefined} />
  <PillGroup {options} {include} {exclude} {excludable} {onToggle} />
</div>
