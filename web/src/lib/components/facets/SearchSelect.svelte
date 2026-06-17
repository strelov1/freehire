<script lang="ts">
  import { uniqueByValue, type FacetOption } from '$lib/facets';
  import { Input } from '$lib/ui';
  import { pillClass } from './pill';

  // A searchable multi-select: a filter field over the options rendered as a
  // scrollable pill list. Selected options sort to the front. Used for the larger
  // enum facets (specialization, industry).
  let {
    options,
    selected,
    exclude = false,
    placeholder,
    onToggle,
  }: {
    options: FacetOption[];
    selected: string[];
    exclude?: boolean;
    placeholder?: string;
    onToggle: (value: string) => void;
  } = $props();

  let filter = $state('');

  const shown = $derived(
    uniqueByValue(options)
      .filter((o) => o.label.toLowerCase().includes(filter.trim().toLowerCase()))
      .sort((a, b) => Number(selected.includes(b.value)) - Number(selected.includes(a.value))),
  );
</script>

<div class="flex flex-col gap-2">
  <Input bind:value={filter} {placeholder} class="w-full" />
  <div class="flex max-h-44 flex-wrap gap-1.5 overflow-y-auto">
    {#each shown as opt (opt.value)}
      {@const active = selected.includes(opt.value)}
      <button
        type="button"
        onclick={() => onToggle(opt.value)}
        class={pillClass(active, exclude, 'px-2.5 py-1 text-sm')}
      >
        {opt.label}{#if opt.count !== undefined}<span class="ml-1 opacity-60 tabular-nums">{opt.count.toLocaleString()}</span>{/if}
      </button>
    {/each}
    {#if shown.length === 0}
      <span class="px-1 py-1 text-xs text-muted-foreground">Nothing found</span>
    {/if}
  </div>
</div>
