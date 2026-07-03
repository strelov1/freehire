<script lang="ts">
  import { X } from '@lucide/svelte';
  import type { FacetOption } from '$lib/facets';
  import { Input } from '$lib/ui';
  import { SvelteMap } from 'svelte/reactivity';
  import { pillClass } from './pill';

  // A server-backed multi-select: as the user types we query `search` (debounced)
  // and list the matching options with counts; an empty query lists the popular
  // first page. Used for entity facets too large to ship as a distribution
  // (company). Selected values render as removable chips above the search — they
  // stay visible even when absent from the current results — labelled from what
  // we have seen, falling back to `fallbackLabel` for a value restored from the URL.
  let {
    search,
    selected,
    exclude = false,
    placeholder,
    onToggle,
    fallbackLabel,
    clearOnSelect = false,
    expand = false,
  }: {
    search: (query: string) => Promise<FacetOption[]>;
    selected: string[];
    exclude?: boolean;
    placeholder?: string;
    onToggle: (value: string) => void;
    fallbackLabel: (value: string) => string;
    // When set, the search field is cleared after picking an option (search →
    // pick a chip → search the next), suited to a build-a-set form.
    clearOnSelect?: boolean;
    // Drop the scroll cap on the results list — for the roomy modal pane.
    expand?: boolean;
  } = $props();

  let query = $state('');
  let results = $state<FacetOption[]>([]);
  let loading = $state(false);
  // value → display label, accumulated from every result we have rendered, so a
  // selected company shows its real name. A reactive SvelteMap so a chip rendered
  // from the URL fallback upgrades to the real name once a later search reveals it
  // — direct mutation triggers the update, no copy-and-reassign needed.
  let seen = new SvelteMap<string, string>();
  // Monotonic request id: a slow earlier query must not overwrite a newer one.
  let gen = 0;

  async function run(q: string) {
    const mine = ++gen;
    loading = true;
    try {
      const opts = await search(q);
      if (mine !== gen) return;
      for (const o of opts) seen.set(o.value, o.label);
      results = opts;
    } catch {
      if (mine === gen) results = [];
    } finally {
      if (mine === gen) loading = false;
    }
  }

  // Debounce: re-run on every query change, cancelling the pending run. Fires once
  // on mount with an empty query to show the popular first page.
  $effect(() => {
    const q = query.trim();
    const t = setTimeout(() => run(q), 250);
    return () => clearTimeout(t);
  });

  // Pick an option from the results; optionally reset the search so the field is
  // ready for the next one (chip removal keeps the query, so it is not routed here).
  function pick(value: string) {
    onToggle(value);
    if (clearOnSelect) query = '';
  }

  const labelOf = (value: string) => seen.get(value) ?? fallbackLabel(value);
  // Don't list an already-selected option in the picker — it's shown as a chip.
  const pickable = $derived(results.filter((o) => !selected.includes(o.value)));
</script>

<div class="flex flex-col gap-2">
  {#if selected.length > 0}
    <div class="flex flex-wrap gap-1.5">
      {#each selected as value (value)}
        <button
          type="button"
          onclick={() => onToggle(value)}
          title={labelOf(value)}
          class={pillClass(true, exclude, 'inline-flex max-w-full items-center px-2.5 py-1 text-sm')}
        >
          <span class="min-w-0 truncate">{labelOf(value)}</span>
          <X class="ml-1 size-3 shrink-0" />
        </button>
      {/each}
    </div>
  {/if}

  <Input bind:value={query} {placeholder} class="w-full" />

  <div class={expand ? 'flex flex-col gap-0.5' : 'flex max-h-44 flex-col gap-0.5 overflow-y-auto'}>
    {#each pickable as opt (opt.value)}
      <button
        type="button"
        onclick={() => pick(opt.value)}
        class="flex items-center justify-between gap-2 rounded-md px-2 py-1 text-left text-sm hover:bg-accent"
      >
        <span class="truncate">{opt.label}</span>
        {#if opt.count !== undefined}
          <span class="shrink-0 tabular-nums opacity-60">{opt.count.toLocaleString()}</span>
        {/if}
      </button>
    {/each}
    {#if pickable.length === 0}
      <span class="px-1 py-1 text-xs text-muted-foreground">
        {loading ? 'Searching…' : query ? 'Nothing found' : 'No results'}
      </span>
    {/if}
  </div>
</div>
