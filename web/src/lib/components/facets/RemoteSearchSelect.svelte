<script lang="ts">
  import { X } from '@lucide/svelte';
  import type { FacetOption } from '#lib/facets.js';
  import { dedupeByKey } from '#lib/paginated.svelte.js';
  import { Input } from '#lib/ui/index.js';
  import SkillIcon from '../SkillIcon.svelte';
  import { SvelteMap } from 'svelte/reactivity';
  import { pillClass } from './pill';
  import { ComboboxNav } from '#lib/comboboxNav.svelte.js';

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
  // wants it visible without a focus/blur dance. Both modes are the same ARIA combobox
  // (see comboboxNav.svelte.ts): the input keeps focus and the arrows walk the list
  // through aria-activedescendant — the expanded list simply never closes.
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
    label,
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
    // The search field's accessible name. A placeholder is not one, so it is only
    // the fallback (callers' placeholders read "Search companies", which names it).
    label?: string;
  } = $props();

  const fieldLabel = $derived(label ?? placeholder ?? 'Search options');

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
      // The option list is keyed by `value`, and a repeated key is not a duplicated
      // row — Svelte answers it by tearing down the whole block, so one doubled entry
      // empties the picker. `search` is a prop, so what it returns is outside this
      // component's reach; keeping the promise is therefore this component's job, and
      // it keeps the first of each value because that is the better-ranked one.
      results = dedupeByKey(opts, (o) => o.value, new Set());
      // A new result set invalidates whatever row the arrows had highlighted.
      nav.active = -1;
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

  // Per instance, so two pickers on one page (AvoidCard renders three) don't share ids.
  const uid = $props.id();
  const nav = new ComboboxNav<FacetOption>({
    id: uid,
    items: () => pickable,
    // The status row (Searching… / Nothing found) is drawn whenever there is nothing to
    // pick, so an open list always has something in it.
    loading: () => pickable.length === 0,
    pick: (o) => pick(o.value),
  });

  // Picking removes the row from the list (it becomes a chip), so a highlight on the
  // last row would point past the end; the next row slides under the others by itself.
  $effect(() => {
    if (nav.active >= pickable.length) nav.active = pickable.length - 1;
  });

  // Typing reopens a list dismissed with Escape, and the old highlight no longer
  // names a row the visitor is looking at.
  function onInput() {
    nav.open = true;
    nav.active = -1;
  }

  function onKeydown(e: KeyboardEvent) {
    // The inline list has nothing to close: leave Escape to the dialog around it.
    if (expand && e.key === 'Escape') return;
    nav.onKeydown(e);
  }

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
  <ul
    id={nav.listId}
    role="listbox"
    aria-label={fieldLabel}
    class={expand
      ? 'flex flex-col gap-0.5'
      : 'absolute inset-x-0 top-full z-10 mt-1 flex max-h-60 flex-col gap-0.5 overflow-y-auto rounded-md border border-border bg-popover p-1 shadow-lg'}
  >
    {#each pickable as opt, i (opt.value)}
      <!-- mousedown (not click), preventDefault: keeps focus on the input instead of
           moving it to the row, so the panel stays open for the next pick; tabindex -1
           keeps the rows out of the Tab order (the input owns the keyboard). -->
      <li
        id={nav.optionId(i)}
        role="option"
        tabindex="-1"
        aria-selected={nav.active === i}
        onmouseenter={() => (nav.active = i)}
        onmousedown={(e) => {
          e.preventDefault();
          pick(opt.value);
        }}
        class="flex cursor-pointer items-center justify-between gap-2 rounded-md px-2 py-1 text-left text-sm hover:bg-accent {nav.active ===
        i
          ? 'bg-accent'
          : ''}"
      >
        <span class="flex min-w-0 items-center gap-1.5">
          {#if techIcons}<SkillIcon slug={opt.value} class="size-3.5 shrink-0" />{/if}
          <span class="truncate">{opt.label}</span>
        </span>
        {#if opt.count !== undefined}
          <span class="shrink-0 tabular-nums opacity-60">{opt.count.toLocaleString()}</span>
        {/if}
      </li>
    {/each}
    {#if pickable.length === 0}
      <li role="option" aria-selected="false" aria-disabled="true" class="px-2 py-1 text-xs text-muted-foreground">
        {emptyMessage}
      </li>
    {/if}
  </ul>
{/snippet}

<div class="flex flex-col gap-2">
  {#if expand}
    <Input
      bind:value={query}
      {placeholder}
      aria-label={fieldLabel}
      class="w-full"
      autocomplete="off"
      role="combobox"
      aria-expanded="true"
      aria-controls={nav.listId}
      aria-autocomplete="list"
      aria-activedescendant={nav.activeDescendant}
      onfocus={() => (nav.open = true)}
      oninput={onInput}
      onkeydown={onKeydown}
    />
    {@render chips()}
    {@render optionList()}
  {:else}
    <div class="relative">
      <Input
        bind:value={query}
        {placeholder}
        aria-label={fieldLabel}
        class="w-full"
        autocomplete="off"
        role="combobox"
        aria-expanded={nav.listOpen}
        aria-controls={nav.listId}
        aria-autocomplete="list"
        aria-activedescendant={nav.activeDescendant}
        onfocus={() => (nav.open = true)}
        oninput={onInput}
        onblur={() => setTimeout(nav.close, 120)}
        onkeydown={onKeydown}
      />
      {#if nav.listOpen}
        {@render optionList()}
      {/if}
    </div>
    {@render chips()}
  {/if}
</div>
