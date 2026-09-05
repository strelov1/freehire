<script lang="ts">
  import { X } from '@lucide/svelte';
  import type { FacetOption } from '$lib/facets';
  import { dedupeByKey } from '$lib/paginated.svelte';
  import { Input } from '$lib/ui';
  import SkillIcon from '../SkillIcon.svelte';
  import { SvelteMap } from 'svelte/reactivity';
  import { pillClass } from './pill';

  // A server-backed multi-select: as the user types we query `search` (debounced)
  // and list the matching options with counts; an empty query lists the popular
  // first page. Used for entity facets too large to ship as a distribution
  // (company). Selected values render as chips below the search field — they stay
  // visible even when absent from the current results — labelled from what we have
  // seen, falling back to `fallbackLabel` for a value restored from the URL. A chip
  // is included or excluded; clicking it calls `onToggle` (cycle for excludable
  // facets, plain include toggle otherwise).
  //
  // Compact mode (expand=false, the default) renders the picker as a floating panel
  // anchored to the input — the same pattern as CompanyPicker.svelte — so it always
  // opens right under the field being typed into, however many chips are already
  // stacked below it. Expand mode keeps the picker inline instead: it's used only
  // inside the filter modal's own scroll pane, which already gives it room and
  // wants it visible without a focus/blur dance.
  let {
    search,
    include,
    exclude = [],
    placeholder,
    onToggle,
    fallbackLabel,
    clearOnSelect = false,
    expand = false,
    ready = true,
    techIcons = false,
  }: {
    search: (query: string) => Promise<FacetOption[]>;
    include: string[];
    exclude?: string[];
    placeholder?: string;
    onToggle: (value: string) => void;
    fallbackLabel: (value: string) => string;
    // When set, the search field is cleared after picking an option (search →
    // pick a chip → search the next), suited to a build-a-set form.
    clearOnSelect?: boolean;
    // Drop the floating panel for a roomy inline pane — for the modal's own scroll area.
    expand?: boolean;
    // False while the candidate list behind `search` (e.g. a facet dictionary) is
    // still loading. Blocks the debounced search until it flips true, so a load
    // that outruns the 250ms debounce gets its popular first page fetched once it
    // lands, instead of the query staying stuck on the empty result it raced into.
    ready?: boolean;
    // Show a brand logo beside a chip/option's label, where SkillIcon has one for
    // the value — set only by the skills picker (see SkillsPicker.svelte). Left
    // off for the other callers (company, location) whose slugs are a different
    // vocabulary that could coincidentally collide with a tech mark's key.
    techIcons?: boolean;
  } = $props();

  let query = $state('');
  let results = $state<FacetOption[]>([]);
  let loading = $state(false);
  let open = $state(false);
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
      // The option list is keyed by `value`, and a repeated key is not a duplicated
      // row — Svelte answers it by tearing down the whole block, so one doubled entry
      // empties the picker. `search` is a prop, so what it returns is outside this
      // component's reach; keeping the promise is therefore this component's job, and
      // it keeps the first of each value because that is the better-ranked one.
      results = dedupeByKey(opts, (o) => o.value, new Set());
    } catch {
      if (mine === gen) results = [];
    } finally {
      if (mine === gen) loading = false;
    }
  }

  // Debounce: re-run on every query change, cancelling the pending run. Fires once
  // on mount with an empty query to show the popular first page, and again
  // whenever `ready` flips true — reading it here is what makes the effect
  // re-fire once a still-loading dictionary lands.
  $effect(() => {
    const q = query.trim();
    if (!ready) {
      results = [];
      return;
    }
    const t = setTimeout(() => run(q), 250);
    return () => clearTimeout(t);
  });

  // Pick an option from the results; optionally reset the search so the field is
  // ready for the next one (chip removal keeps the query, so it is not routed here).
  function pick(value: string) {
    onToggle(value);
    if (clearOnSelect) query = '';
  }

  const selected = $derived([...include, ...exclude]);
  const labelOf = (value: string) => seen.get(value) ?? fallbackLabel(value);
  // Don't list an already-selected option in the picker — it's shown as a chip.
  const pickable = $derived(results.filter((o) => !selected.includes(o.value)));
  const emptyMessage = $derived(!ready ? 'Loading…' : loading ? 'Searching…' : query ? 'Nothing found' : 'No results');
</script>

{#snippet chips()}
  {#if selected.length > 0}
    <div class="flex flex-wrap gap-1.5">
      {#each selected as value (value)}
        {@const excluded = exclude.includes(value)}
        <button
          type="button"
          onclick={() => onToggle(value)}
          title={labelOf(value)}
          class={pillClass(true, excluded, 'inline-flex max-w-full items-center px-2.5 py-1 text-sm')}
        >
          {#if techIcons}<SkillIcon slug={value} class="mr-1 size-3.5 shrink-0" />{/if}
          <span class="min-w-0 truncate">{labelOf(value)}</span>
          <X class="ml-1 size-3 shrink-0" />
        </button>
      {/each}
    </div>
  {/if}
{/snippet}

{#snippet optionList()}
  {#each pickable as opt (opt.value)}
    <button
      type="button"
      role="option"
      aria-selected="false"
      onmousedown={(e) => {
        // mousedown (not click), preventDefault: keeps focus on the input instead
        // of moving it to the button, so the panel stays open for the next pick.
        e.preventDefault();
        pick(opt.value);
      }}
      class="flex items-center justify-between gap-2 rounded-md px-2 py-1 text-left text-sm hover:bg-accent"
    >
      <span class="flex min-w-0 items-center gap-1.5">
        {#if techIcons}<SkillIcon slug={opt.value} class="size-3.5 shrink-0" />{/if}
        <span class="truncate">{opt.label}</span>
      </span>
      {#if opt.count !== undefined}
        <span class="shrink-0 tabular-nums opacity-60">{opt.count.toLocaleString()}</span>
      {/if}
    </button>
  {/each}
  {#if pickable.length === 0}
    <span class="px-2 py-1 text-xs text-muted-foreground">{emptyMessage}</span>
  {/if}
{/snippet}

<div class="flex flex-col gap-2">
  {#if expand}
    <Input bind:value={query} {placeholder} class="w-full" />
    {@render chips()}
    <div class="flex flex-col gap-0.5">
      {@render optionList()}
    </div>
  {:else}
    <div class="relative">
      <Input
        bind:value={query}
        {placeholder}
        class="w-full"
        role="combobox"
        aria-expanded={open}
        aria-controls="remote-search-select-list"
        onfocus={() => (open = true)}
        onblur={() => setTimeout(() => (open = false), 120)}
        onkeydown={(e) => {
          if (e.key === 'Escape') open = false;
        }}
      />
      {#if open}
        <div
          id="remote-search-select-list"
          role="listbox"
          class="absolute inset-x-0 top-full z-10 mt-1 flex max-h-60 flex-col gap-0.5 overflow-y-auto rounded-md border border-border bg-popover p-1 shadow-lg"
        >
          {@render optionList()}
        </div>
      {/if}
    </div>
    {@render chips()}
  {/if}
</div>
