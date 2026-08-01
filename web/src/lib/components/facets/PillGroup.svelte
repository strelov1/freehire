<script lang="ts">
  import { uniqueByValue, type FacetOption } from '$lib/facets';
  import { pillClass, pillTitle } from './pill';

  // A wrap of three-state pills for one facet. Each pill is off (idle), included
  // (primary fill), or excluded (muted destructive, struck through). Clicking calls
  // `onToggle`, which the parent wires to cycle (excludable facets: off → include →
  // exclude → off) or to a plain include toggle (non-excludable). Stateless —
  // selection and the transition come from the store.
  let {
    options,
    include,
    exclude = [],
    excludable = true,
    onToggle,
  }: {
    options: FacetOption[];
    include: string[];
    exclude?: string[];
    excludable?: boolean;
    onToggle: (value: string) => void;
  } = $props();

  // A selected value with no matching option — e.g. a vocabulary value removed
  // since an old bookmark or saved search was created — still renders as a pill so
  // it stays removable instead of becoming an invisible, stuck filter that silently
  // constrains results.
  const shown = $derived(
    uniqueByValue([
      ...options,
      // Ungrouped so it cannot appear under a heading it has nothing to do with;
      // `shown` is re-sorted below to keep the grouped options contiguous.
      ...[...include, ...exclude]
        .filter((v) => !options.some((o) => o.value === v))
        .map((v) => ({ value: v, label: v, group: undefined })),
    ]).sort((a, b) => Number(!!a.group) - Number(!!b.group)),
  );
</script>

<div class="flex flex-wrap gap-2">
  {#each shown as opt, i (opt.value)}
    <!-- An option opening a new group gets a full-width sub-heading above it, so a
         facet mixing two sorts of option (curated collections and verifiable
         credentials) reads as two lists without becoming two facets. -->
    {#if opt.group && opt.group !== shown[i - 1]?.group}
      <p class="w-full pt-1 text-xs font-medium text-muted-foreground">{opt.group}</p>
    {/if}
    {@const excluded = exclude.includes(opt.value)}
    {@const included = include.includes(opt.value)}
    <button
      type="button"
      onclick={() => onToggle(opt.value)}
      title={pillTitle(included, excluded, excludable)}
      class={pillClass(included || excluded, excluded, 'inline-flex items-center gap-1.5 px-3 py-1.5 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50')}
    >
      <!-- A brand mark, where the option has one (the backer collections). Purely
           decorative: the label right beside it says the same thing, so announcing
           the image too would only repeat it. -->
      {#if opt.icon}
        <img src={opt.icon} alt="" class="size-4 shrink-0 rounded-sm object-contain" />
      {/if}
      {opt.label}{#if opt.count !== undefined}<span class="ml-1 opacity-60 tabular-nums">{opt.count.toLocaleString()}</span>{/if}
    </button>
  {/each}
</div>
