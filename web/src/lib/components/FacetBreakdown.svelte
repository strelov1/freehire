<script lang="ts">
  import type { FacetDef } from '$lib/facets';
  import type { FilterStore } from '$lib/filters';

  // One facet's distribution as a horizontal bar chart. `dist` is the value→count
  // map for this facet (from the analytics endpoint); clicking a bar toggles that
  // value in the shared filter store, which drives the drill-down.
  let {
    def,
    dist,
    store,
  }: { def: FacetDef; dist: Record<string, number> | undefined; store: FilterStore } = $props();

  // Cap the rows shown per facet — high-cardinality facets (skills, countries)
  // can carry hundreds of values; the rest are summarized as "+N more".
  const TOP = 15;

  // value → label from the facet's option list; tokens facets (skills/countries)
  // have no options, so the raw value is its own label.
  const labels = $derived(new Map((def.options ?? []).map((o) => [o.value, o.label])));

  // Entries sorted by count descending; bar widths scale to the top count.
  const entries = $derived(Object.entries(dist ?? {}).toSorted((a, b) => b[1] - a[1]));
  const shown = $derived(entries.slice(0, TOP));
  const hidden = $derived(entries.length - shown.length);
  const max = $derived(shown[0]?.[1] ?? 0);

  const selected = $derived(new Set(store.facet(def.param).include));
</script>

<section class="rounded-xl border border-border bg-card p-4">
  <h3 class="mb-3 text-sm font-semibold tracking-tight">{def.label}</h3>
  {#if shown.length === 0}
    <p class="text-xs text-muted-foreground">No data</p>
  {:else}
    <ul class="flex flex-col gap-1.5">
      {#each shown as [value, count] (value)}
        <li>
          <button
            type="button"
            onclick={() => store.pick(def.param, value)}
            aria-pressed={selected.has(value)}
            class={[
              'group relative flex w-full items-center justify-between gap-2 overflow-hidden rounded-md px-2 py-1 text-left text-sm transition-colors',
              selected.has(value) ? 'ring-1 ring-primary' : 'hover:bg-accent',
            ]}
          >
            <span
              class="pointer-events-none absolute inset-y-0 left-0 rounded-md bg-primary/10 transition-[width] group-hover:bg-primary/15"
              style:width={`${max > 0 ? (count / max) * 100 : 0}%`}
            ></span>
            <span class="relative truncate">{labels.get(value) ?? value}</span>
            <span class="relative shrink-0 font-medium tabular-nums text-muted-foreground">
              {count.toLocaleString()}
            </span>
          </button>
        </li>
      {/each}
    </ul>
    {#if hidden > 0}
      <p class="mt-2 text-xs text-muted-foreground">+{hidden.toLocaleString()} more</p>
    {/if}
  {/if}
</section>
