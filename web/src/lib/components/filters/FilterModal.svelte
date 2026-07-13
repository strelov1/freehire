<script lang="ts">
  import type { Snippet } from 'svelte';
  import { Bell, UserRound } from '@lucide/svelte';
  import { FACETS } from '$lib/facets';
  import { isAuthenticated } from '$lib/auth.svelte';
  import { openAuthDialog } from '$lib/auth-dialog.svelte';
  import { profileStore } from '$lib/profile.svelte';
  import { notifications } from '$lib/notifications.svelte';
  import { emptyFilters, type FilterStore, type JobFilters } from '$lib/filters';
  import { StagedFilters } from '$lib/stagedFilters.svelte';
  import { RAIL, RAIL_SECTIONS, type RailEntry, type RailSection } from '$lib/filterSections';
  import type { FacetCounts } from '$lib/types';
  import { FRESHNESS_PRESETS, SALARY_MAX, SALARY_STEP, freshnessLabel } from '$lib/filterControls';
  import FacetSection from '../facets/FacetSection.svelte';
  import ChipFacet from './ChipFacet.svelte';
  import CategoryPane from './CategoryPane.svelte';
  import LocationPane from './LocationPane.svelte';
  import FilterModalShell from './FilterModalShell.svelte';
  import SavedSearches from '../SavedSearches.svelte';

  // The job-search filter modal: a thin wrapper over FilterModalShell that supplies the
  // job rail, the staged job filters, and the pane controls. The shell owns the chrome
  // (rail, footer, deferred apply); this file owns only what's job-specific.
  //
  // Reusable beyond the standalone list: `railKeys` restricts which rail panes show
  // (e.g. Specialization + Skills for a profile); `applyLabel`/`onApply` give the footer
  // a custom label and action (e.g. "Save" → persist a profile); `canApply` gates it.
  let {
    store,
    seed,
    counts = null,
    exclude = [],
    railKeys,
    title = 'All filters',
    applyLabel,
    onApply,
    canApply,
    plain = false,
    savedSearches = false,
    open = false,
    onClose,
    previewCount,
    stagedCounts,
    extra,
  }: {
    store?: FilterStore;
    seed?: JobFilters;
    counts?: FacetCounts | null;
    exclude?: string[];
    railKeys?: string[];
    // Show the "My filters" (saved searches) tab. Opt-in: the standalone job list enables
    // it; reuse like the profile comparison modal leaves it off.
    savedSearches?: boolean;
    title?: string;
    applyLabel?: string;
    onApply?: (staged: StagedFilters) => void | Promise<void>;
    canApply?: (f: JobFilters) => boolean;
    // Plain-select reuse (e.g. the profile editor): drop the search-only exclude/match
    // toggles so a facet value reads as a plain choice, not a filter.
    plain?: boolean;
    open?: boolean;
    onClose: () => void;
    previewCount?: (params: URLSearchParams) => Promise<number>;
    // Live disjunctive facet counts for the staged selection — when supplied, every
    // control shows counts that recompute as you pick (the job list). Absent for the
    // analytics/swipe reuse, which keep the applied `counts` + total-only preview.
    stagedCounts?: (params: URLSearchParams) => Promise<FacetCounts>;
    // Extra content rendered above the pane, handed the staged store so it can edit it
    // (e.g. the profile editor's "import skills from CV").
    extra?: Snippet<[StagedFilters]>;
  } = $props();

  const staged = new StagedFilters();

  // The "My filters" tab is present only on the full job modal — the caller enables saved
  // searches and doesn't restrict the rail to a facet subset (as the profile modal does).
  // Gates the tab itself (visibleRail), its data warm-up, and the footer nudge that jumps
  // to it, so the jump never lands on a missing tab.
  const hasSavedTab = $derived(savedSearches && !railKeys);

  // Warm the Telegram feature flag and the user's profile when the modal opens for a
  // signed-in user on the full job modal: the footer "save for TG alerts" nudge gates on
  // the flag, and the header "Apply my profile" action gates on the profile. Both are
  // no-ops off the browser / once loaded.
  $effect(() => {
    if (open && hasSavedTab && isAuthenticated()) {
      void notifications.ensureLoaded();
      void profileStore.ensureLoaded();
    }
  });

  // The header "Apply my profile" affordance shows on the full job modal (same scope as
  // the My-filters tab). Signed-out: the button still shows and its click opens the
  // sign-in dialog (apply after auth). Signed-in: gated on the profile load having
  // settled (so a user who has a profile never flashes the "create" link while it
  // loads) — the profile-derived Apply button when a profile exists, a create-profile
  // link when it doesn't.
  const showProfileAction = $derived(hasSavedTab && (!isAuthenticated() || profileStore.loaded));
  const profile = $derived(profileStore.profile);

  // The footer nudge shows only when the My-filters tab exists (so the jump lands
  // somewhere), Telegram alerts are available, and there's a search worth saving.
  const showSaveNudge = $derived(hasSavedTab && notifications.telegram.enabled && staged.active > 0);

  // The "My filters" (saved searches) tab. It heads the rail on the full job modal, but
  // not when the caller restricts the rail to a facet subset (e.g. the profile modal),
  // which has no saved-search context.
  const SAVED_ENTRY: RailEntry = { key: 'saved', label: 'My filters', section: 'SAVED', kind: 'saved' };
  const SECTIONS: RailSection[] = ['SAVED', ...RAIL_SECTIONS];

  // Rail entries visible under the current scope: restricted to `railKeys` when given,
  // and a 'facet' entry is hidden when its param is excluded (e.g. Company on a company
  // page).
  const visibleRail = $derived([
    ...(hasSavedTab ? [SAVED_ENTRY] : []),
    ...RAIL.filter((e) => (!railKeys || railKeys.includes(e.key)) && !(e.facetParam && exclude.includes(e.facetParam))),
  ]);

  // Values selected for one facet — included plus excluded — so the rail count reflects
  // any staged selection regardless of sign.
  function selCount(f: JobFilters, param: string): number {
    const st = f.facets[param];
    return st ? st.include.length + st.exclude.length : 0;
  }

  function entryCount(e: RailEntry): number {
    const f = staged.value;
    if (e.kind === 'category') return selCount(f, 'role') + selCount(f, 'category') + selCount(f, 'seniority');
    if (e.kind === 'location') return selCount(f, 'regions') + selCount(f, 'countries') + selCount(f, 'cities');
    if (e.kind === 'salary') return selCount(f, 'salary_currency') + (f.salaryMin != null ? 1 : 0);
    if (e.kind === 'work') return selCount(f, 'work_mode') + selCount(f, 'employment_type');
    if (e.kind === 'industry') return selCount(f, 'domains') + selCount(f, 'company_type') + selCount(f, 'collections');
    if (e.kind === 'language') return selCount(f, 'english_level') + selCount(f, 'posting_language');
    if (e.kind === 'relocation') return selCount(f, 'relocation') + (f.visa ? 1 : 0);
    if (e.kind === 'posted') return f.postedWithinDays != null ? 1 : 0;
    return selCount(f, e.facetParam ?? e.key);
  }

  const englishDef = FACETS.find((d) => d.param === 'english_level');
  const postingDef = FACETS.find((d) => d.param === 'posting_language');

  // In plain mode strip the search-only exclude/match toggles from a facet control.
  function facetDefFor(param: string | undefined) {
    const d = FACETS.find((x) => x.param === param);
    return d && plain ? { ...d, excludable: false, hasAndOr: false } : d;
  }

  function seedStaged() {
    staged.seed(seed ?? store?.value ?? emptyFilters());
  }

  async function apply() {
    if (onApply) {
      await onApply(staged);
      return;
    }
    if (store) staged.commit(store);
  }

  const applyDisabled = $derived(canApply ? !canApply(staged.value) : false);

  // A non-preset value (hand-edited URL) has no exact stop, so it reads as "Any"
  // (the rightmost stop) rather than snapping to "Today".
  const freshnessIndex = $derived.by(() => {
    const i = FRESHNESS_PRESETS.findIndex((p) => p.days === staged.value.postedWithinDays);
    return i < 0 ? FRESHNESS_PRESETS.length - 1 : i;
  });
</script>

<FilterModalShell
  {open}
  {onClose}
  {title}
  rail={visibleRail}
  sections={SECTIONS}
  {staged}
  {entryCount}
  seed={seedStaged}
  initialKey={RAIL[0]?.key}
  {apply}
  {applyDisabled}
  {applyLabel}
  {previewCount}
  countsFetch={stagedCounts}
  {pane}
  headerAction={showProfileAction ? profileAction : undefined}
  extra={extra ? extraStaged : undefined}
  {footerNote}
/>

{#snippet profileAction()}
  {#if !isAuthenticated() || profile}
    <!-- Signed-out: click opens the sign-in dialog; signed-in with a profile: applies it. -->
    <button
      type="button"
      onclick={() => (profile ? staged.applyProfile(profile) : openAuthDialog('login'))}
      class="flex h-9 items-center gap-1.5 rounded-lg bg-brand px-3 text-sm font-medium text-brand-foreground transition-opacity hover:opacity-90"
    >
      <UserRound class="size-4 shrink-0" aria-hidden="true" />
      Apply my profile
    </button>
  {:else}
    <a
      href="/my/profile"
      class="flex h-9 items-center gap-1.5 rounded-lg border border-dashed border-border px-3 text-sm font-medium text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
    >
      <UserRound class="size-4 shrink-0" aria-hidden="true" />
      Create a profile
    </a>
  {/if}
{/snippet}

{#snippet extraStaged()}
  {@render extra?.(staged)}
{/snippet}

{#snippet footerNote({ jumpTo, activeKey }: { jumpTo: (key: string) => void; activeKey: string })}
  {#if showSaveNudge && activeKey !== 'saved'}
    <p class="flex items-center justify-end gap-1.5 text-xs text-muted-foreground">
      <Bell class="size-3.5 shrink-0" aria-hidden="true" />
      <span>
        Want new jobs for this search via Email/Telegram?
        <button
          type="button"
          onclick={() => jumpTo('saved')}
          class="font-medium text-foreground underline underline-offset-2 hover:opacity-80"
        >
          Save it to My filters
        </button>.
      </span>
    </p>
  {/if}
{/snippet}

{#snippet pane(entry: RailEntry, live: FacetCounts | null)}
  {@const c = live ?? counts}
  {#if entry.kind === 'saved'}
    <SavedSearches store={staged} />
  {:else if entry.kind === 'category'}
    {@const roleDef = facetDefFor('role')}
    {#if roleDef && !exclude.includes('role')}
      <div class="mb-6"><FacetSection def={roleDef} store={staged} counts={c} expand /></div>
    {/if}
    {#if !exclude.includes('seniority')}
      <ChipFacet store={staged} param="seniority" label="Seniority" counts={c} />
      <div class="mt-6"><CategoryPane store={staged} {plain} counts={c} /></div>
    {:else}
      <CategoryPane store={staged} {plain} counts={c} />
    {/if}
  {:else if entry.kind === 'location'}
    <LocationPane store={staged} counts={c} />
  {:else if entry.kind === 'facet'}
    {@const def = facetDefFor(entry.facetParam)}
    {#if def}<FacetSection {def} store={staged} counts={c} expand />{/if}
  {:else if entry.kind === 'salary'}
    <ChipFacet store={staged} param="salary_currency" label="Currency" counts={c} />
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
  {:else if entry.kind === 'work'}
    <ChipFacet store={staged} param="work_mode" label="Work format" counts={c} />
    <div class="mt-6"><ChipFacet store={staged} param="employment_type" label="Employment type" counts={c} /></div>
  {:else if entry.kind === 'industry'}
    <ChipFacet store={staged} param="domains" label="Industry" counts={c} />
    <div class="mt-6"><ChipFacet store={staged} param="company_type" label="Company type" counts={c} /></div>
    <div class="mt-6"><ChipFacet store={staged} param="collections" label="Collection" counts={c} /></div>
  {:else if entry.kind === 'language'}
    {#if englishDef}<FacetSection def={englishDef} store={staged} counts={c} expand />{/if}
    <div class="mt-4">{#if postingDef}<FacetSection def={postingDef} store={staged} counts={c} expand />{/if}</div>
  {:else if entry.kind === 'relocation'}
    <ChipFacet store={staged} param="relocation" label="Relocation" counts={c} />
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
  {:else if entry.kind === 'posted'}
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
{/snippet}
