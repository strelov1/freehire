<script lang="ts">
  import { Plus } from '@lucide/svelte';
  import { optionMatches, relatedOptions, uniqueByValue, type FacetOption } from '$lib/facets';
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
    cap,
    related,
    searchAliases,
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
    // Show at most `cap` options while the search field is empty, so a huge
    // distribution (role: hundreds of values) stays readable — the rest surface
    // as you type. Selected options always show. Unset = no cap.
    cap?: number;
    // Curated base-slug → related-slug map. When set (role facet), a "Related"
    // chip row appears under the list once a search narrows it, suggesting
    // siblings the text search can't name (type "mobile" → iOS/Android chips).
    related?: Record<string, string[]>;
    // Curated slug → shorthand-aliases map (role facet). When set, the search
    // matches an option by its aliases too, so "swe"/"sre"/"devrel" surface the
    // right role, not just a label substring.
    searchAliases?: Record<string, readonly string[]>;
  } = $props();

  let filter = $state('');

  function toggle(value: string) {
    onToggle(value);
    if (clearOnSelect) filter = '';
  }

  const isSelected = (v: string) => include.includes(v) || exclude.includes(v);

  // Typo-tolerant filter (fuzzyMatch falls back to substring, so it only ever
  // adds matches — no exact hit is lost) over the label and any shorthand aliases.
  // Keeps the busiest-first order (selected pinned first); the parent already
  // sorted options by count.
  const matched = $derived(
    uniqueByValue(options)
      .filter((o) => optionMatches(o, filter, searchAliases))
      .toSorted((a, b) => Number(isSelected(b.value)) - Number(isSelected(a.value))),
  );

  // While unfiltered, cap the list to the top `cap` (already busiest-first) plus
  // any selected options, so nothing selected is hidden. Typing lifts the cap.
  const capped = $derived(cap && !filter.trim() && matched.length > cap);
  const shown = $derived(
    capped ? matched.filter((o, i) => i < cap! || isSelected(o.value)) : matched,
  );
  const hiddenCount = $derived(capped ? matched.length - shown.length : 0);

  // "Related" suggestions: only once a search narrows the list (an unfiltered
  // picker would suggest everything). Sourced from what the search surfaced
  // (`shown`), excluding anything already shown or selected — so the chips are
  // specifically the siblings the text search couldn't reach.
  const suggestions = $derived(
    related && filter.trim()
      ? relatedOptions(
          uniqueByValue(options),
          shown.map((o) => o.value),
          [...include, ...exclude],
          related,
        )
      : [],
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
        class={pillClass(included || excluded, excluded, 'inline-flex max-w-full items-center px-2.5 py-1 text-sm')}
      >
        <!-- A very long value (roles like "Senior Business Development Representative")
             truncates to one line with an ellipsis instead of wrapping the pill onto
             two rows; the full label surfaces on hover via title. -->
        <span class="min-w-0 truncate" title={opt.label}>{opt.label}</span>{#if opt.count !== undefined}<span class="ml-1 shrink-0 opacity-60 tabular-nums">{opt.count.toLocaleString()}</span>{/if}
      </button>
    {/each}
    {#if shown.length === 0}
      <span class="px-1 py-1 text-xs text-muted-foreground">Nothing found</span>
    {/if}
  </div>
  {#if hiddenCount > 0}
    <span class="px-1 text-xs text-muted-foreground">+{hiddenCount.toLocaleString()} more — type to search</span>
  {/if}
  {#if suggestions.length > 0}
    <div class="mt-1 flex flex-wrap items-center gap-1.5 border-t border-dashed border-border pt-2">
      <span class="text-xs font-medium text-muted-foreground">Related</span>
      {#each suggestions as opt (opt.value)}
        <button
          type="button"
          onclick={() => toggle(opt.value)}
          title="Add {opt.label}"
          class="inline-flex items-center gap-1 rounded-full border border-dashed border-border px-2.5 py-1 text-sm text-muted-foreground transition-colors hover:border-solid hover:bg-accent hover:text-foreground"
        >
          <Plus class="size-3" />{opt.label}{#if opt.count !== undefined}<span class="opacity-60 tabular-nums">{opt.count.toLocaleString()}</span>{/if}
        </button>
      {/each}
    </div>
  {/if}
</div>
