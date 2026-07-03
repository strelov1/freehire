<script lang="ts">
  import { X } from '@lucide/svelte';
  import { FACETS } from '$lib/facets';
  import type { FilterStore, JobFilters } from '$lib/filters';
  import { StagedFilters } from '$lib/stagedFilters.svelte';
  import { RAIL, RAIL_SECTIONS, type RailEntry } from '$lib/filterSections';
  import type { FacetCounts } from '$lib/types';
  import { FRESHNESS_PRESETS, SALARY_MAX, SALARY_STEP, freshnessLabel } from '$lib/filterControls';
  import FacetSection from '../facets/FacetSection.svelte';
  import ChipFacet from './ChipFacet.svelte';
  import CategoryPane from './CategoryPane.svelte';
  import LocationPane from './LocationPane.svelte';

  // The two-pane filter modal. It edits a StagedFilters copy (seeded from the live
  // store on open) and applies it only on "Show results" — the live store/URL is
  // untouched until then. `previewCount(params)` returns the job count for a staged
  // filter set (merged with any fixed scope by the caller), driving the button.
  let {
    store,
    counts = null,
    exclude = [],
    open = false,
    onClose,
    previewCount,
  }: {
    store: FilterStore;
    counts?: FacetCounts | null;
    exclude?: string[];
    open?: boolean;
    onClose: () => void;
    previewCount: (params: URLSearchParams) => Promise<number>;
  } = $props();

  const staged = new StagedFilters();

  // Rail entries visible under the current scope: a 'facet' entry is hidden when its
  // param is excluded (e.g. Source/Company on a company page). Composite entries stay.
  const visibleRail = $derived(RAIL.filter((e) => !(e.facetParam && exclude.includes(e.facetParam))));

  let active = $state<string>('category');

  // Seed staged from the applied filters each time the modal opens, and land on the
  // first visible rail entry.
  let wasOpen = false;
  $effect(() => {
    if (open && !wasOpen) {
      staged.seed(store.value);
      if (!visibleRail.some((e) => e.key === active)) active = visibleRail[0]?.key ?? 'category';
    }
    wasOpen = open;
  });

  const activeEntry = $derived(visibleRail.find((e) => e.key === active) ?? visibleRail[0]);

  function entryCount(e: RailEntry, f: JobFilters): number {
    if (e.kind === 'category') return (f.facets.category?.values.length ?? 0) + (f.facets.seniority?.values.length ?? 0);
    if (e.kind === 'location')
      return (f.facets.regions?.values.length ?? 0) + (f.facets.countries?.values.length ?? 0) + (f.facets.cities?.values.length ?? 0);
    if (e.kind === 'salary') return (f.facets.salary_currency?.values.length ?? 0) + (f.salaryMin != null ? 1 : 0);
    if (e.kind === 'work') return (f.facets.work_mode?.values.length ?? 0) + (f.facets.employment_type?.values.length ?? 0);
    if (e.kind === 'industry')
      return (f.facets.domains?.values.length ?? 0) + (f.facets.company_type?.values.length ?? 0) + (f.facets.collections?.values.length ?? 0);
    if (e.kind === 'language') return (f.facets.english_level?.values.length ?? 0) + (f.facets.posting_language?.values.length ?? 0);
    if (e.kind === 'relocation') return (f.facets.relocation?.values.length ?? 0) + (f.visa ? 1 : 0);
    if (e.kind === 'posted') return f.postedWithinDays != null ? 1 : 0;
    return f.facets[e.facetParam ?? e.key]?.values.length ?? 0;
  }

  // Panes that reuse FacetSection (dynamic controls); ChipFacet resolves its own
  // options from the registry, so composite panes only name the facet param.
  const facetDef = $derived(activeEntry?.kind === 'facet' ? FACETS.find((d) => d.param === activeEntry!.facetParam) : undefined);
  const englishDef = FACETS.find((d) => d.param === 'english_level');
  const postingDef = FACETS.find((d) => d.param === 'posting_language');

  // Debounced live count for the staged filters — the same cheap facet call the panel
  // already makes, keyed by a monotonic gen so a slow response can't overwrite a newer.
  let showCount = $state<number | null>(null);
  let countGen = 0;
  $effect(() => {
    if (!open) return;
    const params = staged.params(); // tracks staged.value
    const gen = ++countGen;
    const t = setTimeout(() => {
      previewCount(params)
        .then((n) => {
          if (gen === countGen) showCount = n;
        })
        .catch(() => {});
    }, 200);
    return () => clearTimeout(t);
  });

  function apply() {
    staged.commit(store);
    onClose();
  }

  function onKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') onClose();
  }

  // A non-preset value (hand-edited URL) has no exact stop, so it reads as "Any"
  // (the rightmost stop) — matching the label — rather than snapping to "Today".
  const freshnessIndex = $derived.by(() => {
    const i = FRESHNESS_PRESETS.findIndex((p) => p.days === staged.value.postedWithinDays);
    return i < 0 ? FRESHNESS_PRESETS.length - 1 : i;
  });
</script>

<svelte:window onkeydown={open ? onKeydown : undefined} />

{#if open}
  <div class="fixed inset-0 z-50 flex items-stretch justify-center sm:items-center sm:p-6">
    <!-- backdrop -->
    <button class="absolute inset-0 bg-foreground/35" aria-label="Close filters" onclick={onClose}></button>

    <div
      class="relative flex h-full w-full flex-col overflow-hidden bg-background shadow-2xl sm:h-auto sm:max-h-[calc(100vh-3rem)] sm:max-w-4xl sm:rounded-2xl sm:border sm:border-border"
      role="dialog"
      aria-modal="true"
      aria-label="All filters"
    >
      <!-- header -->
      <div class="flex items-center gap-3 border-b border-border px-4 py-3">
        <h2 class="flex-1 text-base font-semibold tracking-tight">All filters</h2>
        <button
          type="button"
          onclick={onClose}
          aria-label="Close"
          class="flex size-9 items-center justify-center rounded-lg border border-border text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
        >
          <X class="size-4" />
        </button>
      </div>

      <div class="flex min-h-0 flex-1">
        <!-- rail -->
        <nav class="w-40 shrink-0 overflow-y-auto border-r border-border py-2 sm:w-56">
          {#each RAIL_SECTIONS as section (section)}
            {@const entries = visibleRail.filter((e) => e.section === section)}
            {#if entries.length}
              <div class="px-3 pb-1 pt-3 text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">{section}</div>
              {#each entries as e (e.key)}
                {@const n = entryCount(e, staged.value)}
                <button
                  type="button"
                  onclick={() => (active = e.key)}
                  class={[
                    'flex w-full items-center justify-between gap-2 border-l-2 px-3 py-2 text-left text-sm transition-colors',
                    active === e.key
                      ? 'border-foreground bg-accent font-semibold'
                      : 'border-transparent font-medium hover:bg-accent',
                  ]}
                >
                  <span class="truncate">{e.label}</span>
                  {#if n > 0}
                    <span
                      class={[
                        'shrink-0 rounded-full px-1.5 text-[11px] font-semibold',
                        active === e.key ? 'bg-primary text-primary-foreground' : 'bg-secondary text-muted-foreground',
                      ]}>{n}</span
                    >
                  {/if}
                </button>
              {/each}
            {/if}
          {/each}
        </nav>

        <!-- pane -->
        <section class="min-w-0 flex-1 overflow-y-auto p-4 sm:p-6">
          {#if activeEntry?.kind === 'category'}
            <ChipFacet store={staged} param="seniority" label="Seniority" />
            <div class="mt-6"><CategoryPane store={staged} /></div>
          {:else if activeEntry?.kind === 'location'}
            <LocationPane store={staged} {counts} />
          {:else if activeEntry?.kind === 'facet' && facetDef}
            <FacetSection def={facetDef} store={staged} {counts} expand />
          {:else if activeEntry?.kind === 'salary'}
            <ChipFacet store={staged} param="salary_currency" label="Currency" />
            <div class="mb-2 mt-6 flex items-center justify-between">
              <h3 class="text-sm font-semibold tracking-tight">Minimum salary</h3>
              <span class="text-xs font-medium text-muted-foreground"
                >{staged.value.salaryMin ? `${staged.value.salaryMin.toLocaleString('en-US')}+` : 'Any'}</span
              >
            </div>
            <input
              type="range"
              min="0"
              max={SALARY_MAX}
              step={SALARY_STEP}
              value={staged.value.salaryMin ?? 0}
              oninput={(e) => staged.setSalaryMin(Number(e.currentTarget.value) || null)}
              aria-label="Minimum salary"
              class="w-full accent-primary"
            />
          {:else if activeEntry?.kind === 'work'}
            <ChipFacet store={staged} param="work_mode" label="Work format" />
            <div class="mt-6"><ChipFacet store={staged} param="employment_type" label="Employment type" /></div>
          {:else if activeEntry?.kind === 'industry'}
            <ChipFacet store={staged} param="domains" label="Industry" />
            <div class="mt-6"><ChipFacet store={staged} param="company_type" label="Company type" /></div>
            <div class="mt-6"><ChipFacet store={staged} param="collections" label="Collection" /></div>
          {:else if activeEntry?.kind === 'language'}
            {#if englishDef}<FacetSection def={englishDef} store={staged} {counts} expand />{/if}
            <div class="mt-4">{#if postingDef}<FacetSection def={postingDef} store={staged} {counts} expand />{/if}</div>
          {:else if activeEntry?.kind === 'relocation'}
            <ChipFacet store={staged} param="relocation" label="Relocation" />
            <h3 class="mb-2 mt-6 text-sm font-semibold tracking-tight">Visa</h3>
            <label class="flex cursor-pointer items-center gap-2 text-sm">
              <input
                type="checkbox"
                class="size-4 rounded border-border"
                checked={staged.value.visa}
                onchange={(e) => staged.setVisa(e.currentTarget.checked)}
              />
              <span>Offers visa sponsorship</span>
            </label>
          {:else if activeEntry?.kind === 'posted'}
            <div class="mb-2 flex items-center justify-between">
              <h3 class="text-sm font-semibold tracking-tight">Posted within</h3>
              <span class="text-xs font-medium text-muted-foreground">{freshnessLabel(staged.value.postedWithinDays)}</span>
            </div>
            <input
              type="range"
              min="0"
              max={FRESHNESS_PRESETS.length - 1}
              step="1"
              value={freshnessIndex}
              oninput={(e) => staged.setPostedWithinDays(FRESHNESS_PRESETS[Number(e.currentTarget.value)]?.days ?? null)}
              aria-label="Posted within"
              class="w-full accent-primary"
            />
          {/if}
        </section>
      </div>

      <!-- footer -->
      <div class="flex items-center justify-between border-t border-border px-4 py-3">
        <button
          type="button"
          onclick={() => staged.clear()}
          class="text-sm font-medium text-muted-foreground transition-colors hover:text-foreground">Clear all</button
        >
        <button
          type="button"
          onclick={apply}
          class="inline-flex h-11 items-center gap-1.5 rounded-lg bg-primary px-6 text-sm font-semibold text-primary-foreground transition-opacity hover:opacity-90"
        >
          Show {showCount != null ? showCount.toLocaleString('en-US') : ''} {showCount === 1 ? 'job' : 'jobs'}
        </button>
      </div>
    </div>
  </div>
{/if}
