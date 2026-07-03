<script lang="ts">
  import { uniqueByValue, type FacetOption } from '$lib/facets';
  import { Input } from '$lib/ui';
  import { pillClass, pillTitle } from './pill';

  // A searchable multi-select: a filter field over the options rendered as a
  // scrollable list of three-state pills (off / include / exclude). Selected options
  // sort to the front. Used for the larger enum facets (specialization, industry,
  // skills) and, in plain mode (excludable=false), the profile's specialization
  // picker. Clicking calls `onToggle`, which the parent wires to cycle or to a plain
  // include toggle.
  let {
    options,
    include,
    exclude = [],
    excludable = false,
    placeholder,
    onToggle,
    clearOnSelect = false,
    expand = false,
  }: {
    options: FacetOption[];
    include: string[];
    exclude?: string[];
    excludable?: boolean;
    placeholder?: string;
    onToggle: (value: string) => void;
    // When set, the filter field is cleared after each toggle — suited to a
    // build-a-set form (search → pick → search the next), not the filter panel
    // where you toggle several visible pills after one search.
    clearOnSelect?: boolean;
    // Drop the scroll cap on the pill list — used in the roomy modal pane where the
    // full list should show, unlike the compact sidebar (which caps + scrolls).
    expand?: boolean;
  } = $props();

  let filter = $state('');

  function toggle(value: string) {
    onToggle(value);
    if (clearOnSelect) filter = '';
  }

  const isSelected = (v: string) => include.includes(v) || exclude.includes(v);

  const shown = $derived(
    uniqueByValue(options)
      .filter((o) => o.label.toLowerCase().includes(filter.trim().toLowerCase()))
      .toSorted((a, b) => Number(isSelected(b.value)) - Number(isSelected(a.value))),
  );
</script>

<div class="flex flex-col gap-2">
  <Input bind:value={filter} {placeholder} class="w-full" />
  <div class={expand ? 'flex flex-wrap gap-1.5' : 'flex max-h-44 flex-wrap gap-1.5 overflow-y-auto'}>
    {#each shown as opt (opt.value)}
      {@const excluded = exclude.includes(opt.value)}
      {@const included = include.includes(opt.value)}
      <button
        type="button"
        onclick={() => toggle(opt.value)}
        title={pillTitle(included, excluded, excludable)}
        class={pillClass(included || excluded, excluded, 'px-2.5 py-1 text-sm')}
      >
        {opt.label}{#if opt.count !== undefined}<span class="ml-1 opacity-60 tabular-nums">{opt.count.toLocaleString()}</span>{/if}
      </button>
    {/each}
    {#if shown.length === 0}
      <span class="px-1 py-1 text-xs text-muted-foreground">Nothing found</span>
    {/if}
  </div>
</div>
