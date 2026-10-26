<script lang="ts">
  import { tick, type Snippet } from 'svelte';
  import { ArrowRight, LoaderCircle, Search, X } from '@lucide/svelte';
  import type { RailEntry, RailSection } from '#lib/filterSections.js';
  import { FILTER_SEARCH_MIN_QUERY, type FilterSearchHit } from '#lib/filterSearch.js';
  import type { FacetCounts } from '#lib/types.js';
  import { focusTrap } from '#lib/actions/focusTrap.js';
  import { lockScroll, unlockScroll } from '#lib/ui/index.js';
  import { errorMessage } from '#lib/utils.js';
  import FilterSearchResults from './FilterSearchResults.svelte';

  // The reusable two-pane filter-modal chrome: backdrop, header, the sectioned left
  // rail, and the deferred footer (Clear all / Apply / live preview). It knows nothing
  // about jobs vs companies — a domain wrapper (FilterModal, CompanyFilterModal) passes
  // the rail, a `staged` edit store, per-entry counts, a `seed` thunk run on open, an
  // `apply` action, and a `pane` snippet that renders the active entry's controls.
  //
  // Deferred-apply contract: the pane edits the caller's staged copy; nothing reaches
  // the live list until `apply` runs on the footer button.

  // The minimal staged-store surface the shell drives; the wrapper's StagedFilters /
  // StagedCompanyFilters both satisfy it (seed/commit are closed over by the wrapper's
  // `seed`/`apply` thunks, so the shell stays domain-agnostic).
  interface StagedSurface {
    readonly active: number;
    params(): URLSearchParams;
    clear(): void;
  }

  let {
    open = false,
    onClose,
    title = 'All filters',
    rail,
    sections,
    staged,
    entryCount,
    seed,
    apply,
    onClear,
    applyDisabled = false,
    applyLabel,
    previewCount,
    countsFetch,
    initialKey,
    pane,
    headerAction,
    titleHint,
    extra,
    footerNote,
    searchItems,
    onSearchToggle,
    searchSelection,
    filterQuery = $bindable(''),
  }: {
    open?: boolean;
    onClose: () => void;
    title?: string;
    rail: RailEntry[];
    sections: RailSection[];
    staged: StagedSurface;
    entryCount: (e: RailEntry) => number;
    /** Seed the staged copy from the live state — run once per open. */
    seed: () => void;
    /** Commit the staged copy (or a custom save). A thrown error keeps the modal open. */
    apply: () => void | Promise<void>;
    /** Run after the footer's Clear all resets `staged` — for a draft the wrapper stages
     *  outside it (the job modal's minimum skill match), so Clear all clears that too. */
    onClear?: () => void;
    applyDisabled?: boolean;
    /** Custom footer label (e.g. "Save"); when absent the button shows the live preview. */
    applyLabel?: string;
    /** Legacy total-only preview: the footer count. Used when `countsFetch` is absent. */
    previewCount?: (params: URLSearchParams) => Promise<number>;
    /** Live disjunctive facet counts for the staged selection. When set, the shell fetches
     *  the full distribution (debounced), drives the footer total + a loading spinner, and
     *  hands the distribution to the `pane` snippet — so every control shows live counts. */
    countsFetch?: (params: URLSearchParams) => Promise<FacetCounts>;
    /** Rail entry to land on when the modal opens (defaults to the first entry) — lets a
     *  caller keep a lead tab (e.g. "My filters") in the rail without opening on it. */
    initialKey?: string;
    /** Renders the active entry's controls into the right pane, given the live staged
     *  facet counts (null until the first fetch resolves or when `countsFetch` is absent). */
    pane: Snippet<[RailEntry, FacetCounts | null]>;
    /** Optional control rendered in the header row, between the title and Close (e.g. the
     *  job modal's "Apply my profile"). Absent for callers that don't need one. */
    headerAction?: Snippet;
    /** Optional small hint rendered right after the title (e.g. the job modal's
     *  include/exclude tooltip). Absent for callers with nothing to explain there —
     *  the companies modal has no excludable facets, so it passes none. */
    titleHint?: Snippet;
    /** Optional content above the pane (e.g. the profile editor's "import from CV"). */
    extra?: Snippet;
    /** Optional footer nudge above the action row, handed a `jumpTo` to switch panes and
     *  the current `activeKey` — lets a domain wrapper add copy (e.g. "save this search")
     *  that jumps to another tab without the shell knowing what the tabs mean. */
    footerNote?: Snippet<[{ jumpTo: (key: string) => void; activeKey: string }]>;
    searchItems?: readonly FilterSearchHit[];
    onSearchToggle?: (hit: FilterSearchHit) => void;
    searchSelection?: (hit: FilterSearchHit) => { included: boolean; excluded: boolean; excludable: boolean };
    filterQuery?: string;
  } = $props();

  // Set on the first open (see the seed effect); until then activeEntry falls back to
  // the first rail entry.
  let active = $state<string>('');

  // Seed the staged copy from the live state each time the modal opens, and land on the
  // `initialKey` entry (falling back to the first) when the current one isn't in this rail.
  let wasOpen = false;
  let searchInput = $state<HTMLInputElement | null>(null);
  const querySection = $derived.by(() => {
    const q = filterQuery.trim().toLowerCase();
    if (!searchItems || q.length < FILTER_SEARCH_MIN_QUERY) return undefined;
    const matched = rail.filter((entry) => {
      const names = [entry.label.toLowerCase()];
      for (const hit of searchItems) {
        if (hit.railKey !== entry.key || hit.param || hit.action) continue;
        if (hit.label.toLowerCase() !== entry.label.toLowerCase()) continue;
        for (const alias of hit.aliases ?? []) names.push(alias.toLowerCase());
      }
      return names.some((name) => name.startsWith(q));
    });
    return matched.length === 1 ? matched[0] : undefined;
  });
  const searching = $derived(
    searchItems != null && filterQuery.trim().length >= FILTER_SEARCH_MIN_QUERY && !querySection,
  );
  $effect(() => {
    if (open && !wasOpen) {
      seed();
      if (!rail.some((e) => e.key === active)) {
        active = (initialKey && rail.some((e) => e.key === initialKey) ? initialKey : rail[0]?.key) ?? '';
      }
      // Focus the search on a wide screen only: on a phone it would raise the keyboard
      // over half the modal before the visitor asked to type.
      if (searchItems && matchMedia('(min-width: 640px)').matches) void tick().then(() => searchInput?.focus());
    }
    if (!open && filterQuery) filterQuery = '';
    wasOpen = open;
  });

  function jumpTo(key: string) {
    active = key;
    filterQuery = '';
  }

  const shownKey = $derived(querySection?.key ?? active);
  const activeEntry = $derived(querySection ?? rail.find((e) => e.key === active) ?? rail[0]);

  // Debounced live preview for the staged filters, keyed by a monotonic gen so a slow
  // response can't overwrite a newer one. With `countsFetch` the shell holds the full
  // distribution (`liveCounts`) + a `loading` flag driving the footer spinner; otherwise
  // it falls back to the total-only `previewCount`.
  let showCount = $state<number | null>(null);
  let liveCounts = $state<FacetCounts | null>(null);
  let loading = $state(false);
  // The latest count request failed: the footer then says "Show jobs" rather than a
  // number that belongs to an earlier selection (or a blank where the number goes).
  let countFailed = $state(false);
  let countGen = 0;
  $effect(() => {
    if (!open) return;
    const params = staged.params(); // tracks the staged edits
    const gen = ++countGen;
    let t: ReturnType<typeof setTimeout> | undefined;
    if (countsFetch) {
      const cf = countsFetch;
      loading = true;
      t = setTimeout(() => {
        cf(params)
          .then((c) => {
            if (gen === countGen) {
              liveCounts = c;
              loading = false;
              countFailed = false;
            }
          })
          .catch(() => {
            if (gen === countGen) {
              loading = false;
              countFailed = true;
            }
          });
      }, 200);
    } else if (previewCount) {
      const pc = previewCount;
      t = setTimeout(() => {
        pc(params)
          .then((n) => {
            if (gen === countGen) {
              showCount = n;
              countFailed = false;
            }
          })
          .catch(() => {
            if (gen === countGen) countFailed = true;
          });
      }, 200);
    }
    return () => clearTimeout(t);
  });

  // The footer total: the live distribution's total when count-fetching, else the
  // legacy preview number.
  const total = $derived.by(() => {
    if (countFailed) return null;
    if (countsFetch) return liveCounts?.total ?? null;
    return showCount;
  });
  const jobsText = (n: number) => `${n.toLocaleString('en-US')} ${n === 1 ? 'job' : 'jobs'}`;
  const showLabel = $derived(total != null ? `Show ${jobsText(total)}` : 'Show jobs');
  // What the polite live region below says once a count settles. The button text alone
  // changes silently, so a screen-reader user picking filters would otherwise never hear
  // the result shrink. Empty while counting, so only the settled figure is announced.
  const countAnnouncement = $derived(applyLabel || loading || total == null ? '' : `${jobsText(total)} match`);

  // Ids that tie the pane to the rail entry naming it.
  const uid = $props.id();
  const railLabelId = (key: string) => `${uid}-rail-${key}`;

  let applyBusy = $state(false);
  let applyError = $state<string | null>(null);

  async function runApply() {
    if (applyDisabled || applyBusy) return;
    applyBusy = true;
    applyError = null;
    try {
      await apply();
      onClose();
    } catch (e) {
      applyError = errorMessage(e, 'Something went wrong. Please try again.');
    } finally {
      applyBusy = false;
    }
  }

  $effect(() => {
    if (!open) return;
    lockScroll();
    return unlockScroll;
  });

  let dialogEl = $state<HTMLElement>();

  // Escape closes the modal unless something inside it claimed the key first. A child
  // that handles Escape itself (the AI filter dialog) stops it or prevents its default;
  // the design-system tooltip does neither — it listens on the window too, and both
  // window listeners fire for one press — so an open tooltip inside the dialog is taken
  // as the claim: that press dismisses the tooltip, the next one the modal. The tooltip
  // is still in the DOM here, since its own hide only re-renders after this dispatch.
  function onKeydown(e: KeyboardEvent) {
    if (e.key !== 'Escape' || e.defaultPrevented) return;
    if (dialogEl?.querySelector('[role="tooltip"]')) return;
    if (filterQuery) {
      filterQuery = '';
      return;
    }
    onClose();
  }

  function clearAll() {
    staged.clear();
    onClear?.();
  }

  function selectionOf(hit: FilterSearchHit) {
    return searchSelection?.(hit) ?? { included: false, excluded: false, excludable: false };
  }
</script>

<svelte:window onkeydown={open ? onKeydown : undefined} />

{#if open}
  <div class="fixed inset-0 z-50 flex items-stretch justify-center sm:items-center sm:p-6">
    <!-- backdrop -->
    <button class="absolute inset-0 animate-modal-fade bg-foreground/35" aria-label="Close filters" onclick={onClose}></button>

    <div
      class={[
        'relative flex h-full w-full animate-modal-pop flex-col overflow-hidden bg-background shadow-2xl sm:max-h-[calc(100vh-3rem)] sm:max-w-4xl sm:rounded-2xl sm:border sm:border-border',
        searching ? 'sm:h-screen' : 'sm:h-auto',
      ]}
      role="dialog"
      aria-modal="true"
      aria-label={title}
      bind:this={dialogEl}
      {@attach focusTrap()}
    >
      <!-- header -->
      <!-- On a phone the header actions leave the search no width, so the search wraps
           onto its own full-width row under them. -->
      <div class="flex flex-wrap items-center gap-x-3 gap-y-2 border-b border-border px-4 py-3 sm:flex-nowrap">
        <h2 class="flex shrink-0 items-center gap-1.5 text-base font-semibold tracking-tight">
          {title}
          {#if titleHint}{@render titleHint()}{/if}
        </h2>
        {#if searchItems}
          <div
            class="order-last flex min-w-0 basis-full items-center gap-2 rounded-lg border border-input px-3 focus-within:ring-2 focus-within:ring-ring sm:order-none sm:flex-1 sm:basis-auto"
          >
            <Search class="size-4 shrink-0 text-muted-foreground" />
            <input
              bind:this={searchInput}
              bind:value={filterQuery}
              type="text"
              placeholder="Search filters"
              aria-label="Search filters"
              autocomplete="off"
              spellcheck="false"
              class="h-9 w-full bg-transparent text-sm outline-none placeholder:text-muted-foreground"
            />
            {#if filterQuery}
              <button
                type="button"
                aria-label="Clear filter search"
                onclick={() => (filterQuery = '')}
                class="flex size-7 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
              >
                <X class="size-3.5" />
              </button>
            {/if}
          </div>
          <div class="flex-1 sm:hidden"></div>
        {:else}
          <div class="flex-1"></div>
        {/if}
        {#if searchItems && headerAction}
          <div class="mx-2 hidden h-6 w-px shrink-0 bg-border sm:block" aria-hidden="true"></div>
        {/if}
        {#if headerAction}{@render headerAction()}{/if}
        <button
          type="button"
          onclick={onClose}
          aria-label="Close"
          class="flex size-9 shrink-0 items-center justify-center rounded-lg border border-border text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
        >
          <X class="size-4" />
        </button>
      </div>

      <div class="flex min-h-0 flex-1">
        <!-- rail -->
        <!-- Below sm the rail is w-32 (128px), not the w-40 it was: on a 360px phone that
             gives the pane 232px instead of 200px, and its search field and pills need it
             more than the rail does. Labels wrap to a second line rather than truncate,
             which at that width they would ("Work & employme…"). -->
        <nav aria-label="Filter sections" class="w-32 shrink-0 overflow-y-auto overscroll-contain border-r border-border py-2 sm:w-56">
          {#each sections as section (section)}
            {@const entries = rail.filter((e) => e.section === section)}
            {#if entries.length}
              <div class="px-2.5 pb-1 pt-3 text-[11px] font-semibold uppercase tracking-wider text-muted-foreground sm:px-3">{section}</div>
              {#each entries as e (e.key)}
                {@const n = entryCount(e)}
                <button
                  type="button"
                  onclick={() => jumpTo(e.key)}
                  aria-current={shownKey === e.key ? 'true' : undefined}
                  class={[
                    'flex w-full items-center justify-between gap-1.5 border-l-2 px-2.5 py-2 text-left text-sm transition-colors sm:gap-2 sm:px-3',
                    shownKey === e.key
                      ? 'border-brand bg-accent font-semibold'
                      : 'border-transparent font-medium hover:bg-accent',
                  ]}
                >
                  <span id={railLabelId(e.key)} class="min-w-0 hyphens-auto break-words sm:truncate">{e.label}</span>
                  {#if n > 0}
                    <span
                      class={[
                        'shrink-0 rounded-full px-1.5 text-[11px] font-semibold',
                        shownKey === e.key ? 'bg-brand text-brand-foreground' : 'bg-secondary text-muted-foreground',
                      ]}><span aria-hidden="true">{n}</span><span class="sr-only">, {n} selected</span></span
                    >
                  {/if}
                </button>
              {/each}
            {/if}
          {/each}
        </nav>

        <!-- pane -->
        <!-- Named by the active rail entry, so a screen reader entering the pane hears
             which section it is in. -->
        <section
          aria-labelledby={activeEntry && !searching ? railLabelId(activeEntry.key) : undefined}
          aria-label={searching ? 'Matching filters' : undefined}
          class="min-w-0 flex-1 overflow-y-auto overscroll-contain p-4 sm:p-6"
        >
          {#if extra && !searching}<div class="mb-6">{@render extra()}</div>{/if}
          {#if searching && searchItems}
            <FilterSearchResults
              items={searchItems}
              query={filterQuery}
              counts={liveCounts}
              onToggle={(hit) => onSearchToggle?.(hit)}
              onJump={jumpTo}
              selection={selectionOf}
            />
          {:else if activeEntry}
            {@render pane(activeEntry, liveCounts)}
          {/if}
        </section>
      </div>

      <!-- footer -->
      <!-- The bottom padding never drops under the home indicator on a notched phone,
           where the modal is full-screen and this row sits at the very bottom. -->
      <div class="flex flex-col gap-2 border-t border-border px-4 pt-3 pb-safe">
        {#if applyError}
          <p class="text-sm text-destructive">{applyError}</p>
        {/if}
        <div class="flex items-center justify-between">
          <button
            type="button"
            onclick={clearAll}
            class="-ml-3 inline-flex h-11 items-center gap-1.5 rounded-lg px-3 text-sm font-medium text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
            ><X class="size-3.5" aria-hidden="true" />Clear all</button
          >
          <button
            type="button"
            onclick={runApply}
            disabled={applyDisabled || applyBusy}
            class="inline-flex h-11 items-center gap-1.5 rounded-lg bg-brand px-6 text-sm font-semibold text-brand-foreground transition-opacity hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-50"
          >
            {#if applyLabel}
              {applyBusy ? 'Saving…' : applyLabel}
              {#if !applyBusy}<ArrowRight class="size-4" aria-hidden="true" />{/if}
            {:else if loading}
              Show <LoaderCircle class="size-4 animate-spin" aria-hidden="true" /><span class="sr-only">(counting)</span> jobs
            {:else}
              {showLabel}
              <ArrowRight class="size-4" aria-hidden="true" />
            {/if}
          </button>
        </div>
        <p class="sr-only" aria-live="polite">{countAnnouncement}</p>
        {#if footerNote}{@render footerNote({ jumpTo, activeKey: active })}{/if}
      </div>
    </div>
  </div>
{/if}
