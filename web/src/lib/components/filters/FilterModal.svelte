<script lang="ts">
  import type { Snippet } from 'svelte';
  import { resolve } from '$app/paths';
  import { Bell, Info, UserRound } from '@lucide/svelte';
  import { Tooltip } from '$lib/ui';
  import { EMPLOYER_CREDENTIALS, FACETS, JOB_COLLECTION } from '$lib/facets';
  import { isAuthenticated } from '$lib/auth.svelte';
  import { openAuthDialog } from '$lib/auth-dialog.svelte';
  import { profileStore } from '$lib/profile.svelte';
  import { notifications } from '$lib/notifications.svelte';
  import { emptyFilters, type FilterStore, type JobFilters } from '$lib/filters';
  import { StagedFilters } from '$lib/stagedFilters.svelte';
  import { RAIL, RAIL_SECTIONS, type RailEntry, type RailSection } from '$lib/filterSections';
  import type { FacetCounts } from '$lib/types';
  import {
    EXPERIENCE_PRESETS,
    FRESHNESS_PRESETS,
    SALARY_MAX,
    SALARY_STEP,
    experienceLabel,
    freshnessLabel,
  } from '$lib/filterControls';
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
    matchAvailable = false,
    minMatch = null,
    onMinMatchChange,
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
    // The "Minimum skill match" slider atop the Skills pane: a client-only threshold
    // over the viewer's own profile skills, not a JobFilters facet (the match percent
    // depends on who's looking, so there's nothing to put in JobFilters/the URL — see
    // JobsView's `minMatch`). Hidden unless the caller has a real percent to filter on.
    matchAvailable?: boolean;
    minMatch?: number | null;
    onMinMatchChange?: (value: number | null) => void;
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
  // and a 'facet' entry is hidden when its param is excluded (e.g. Company on a company page).
  const visibleRail = $derived([
    ...(hasSavedTab ? [SAVED_ENTRY] : []),
    ...RAIL.filter(
      (e) => (!railKeys || railKeys.includes(e.key)) && !(e.facetParam && exclude.includes(e.facetParam)),
    ),
  ]);

  const jobCollectionValues = JOB_COLLECTION.map((o) => o.value);
  const employerCredentialValues = EMPLOYER_CREDENTIALS.map((o) => o.value);

  // Values selected for one facet — included plus excluded — so the rail count reflects
  // any staged selection regardless of sign. `values`, when passed, scopes the count to
  // just that subset — for a param split across two panes (see ChipFacet's `options`
  // override), so a badge doesn't count values shown under a different tab.
  function selCount(f: JobFilters, param: string, values?: string[]): number {
    const st = f.facets[param];
    if (!st) return 0;
    if (!values) return st.include.length + st.exclude.length;
    const allowed = new Set(values);
    return st.include.filter((v) => allowed.has(v)).length + st.exclude.filter((v) => allowed.has(v)).length;
  }

  function entryCount(e: RailEntry): number {
    const f = staged.value;
    if (e.kind === 'category') return selCount(f, 'role') + selCount(f, 'category') + selCount(f, 'ai_archetype');
    if (e.kind === 'experience')
      return selCount(f, 'seniority') + selCount(f, 'role_type') + (f.experienceYearsMax != null ? 1 : 0);
    if (e.kind === 'location') return selCount(f, 'regions') + selCount(f, 'countries') + selCount(f, 'cities');
    if (e.kind === 'salary') return selCount(f, 'salary_currency') + (f.salaryMin != null ? 1 : 0);
    if (e.kind === 'work') return selCount(f, 'work_mode') + selCount(f, 'employment_type');
    if (e.kind === 'industry')
      return selCount(f, 'domains') + selCount(f, 'company_type') + selCount(f, 'collections', jobCollectionValues);
    if (e.kind === 'language') return selCount(f, 'english_level') + selCount(f, 'posting_language');
    if (e.kind === 'relocation')
      return selCount(f, 'relocation') + (f.visa ? 1 : 0) + selCount(f, 'collections', employerCredentialValues);
    if (e.kind === 'posted') return f.postedWithinDays != null ? 1 : 0;
    // The Minimum skill match threshold lives at the top of the Skills pane, so it
    // counts toward that tab's badge alongside the skills facet selections.
    if (e.key === 'skills') return selCount(f, 'skills') + (minMatch != null ? 1 : 0);
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

  // Same snap-to-Any rule as freshness above: an off-preset bound (hand-edited or
  // shared URL) has no exact stop, so the handle rests on the rightmost one. The
  // LABEL still reports the real bound — see experienceLabel — so the two do not
  // agree here on purpose: the handle says "no stop matches", the text says what
  // is actually filtering.
  const experienceIndex = $derived.by(() => {
    const i = EXPERIENCE_PRESETS.findIndex((p) => p.years === staged.value.experienceYearsMax);
    return i < 0 ? EXPERIENCE_PRESETS.length - 1 : i;
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
  {titleHint}
  extra={extra ? extraStaged : undefined}
  {footerNote}
/>

<!-- Most facets are three-state (off / include / exclude); one quiet hint in the
     header covers all of them rather than repeating the explanation on every
     excludable section. -->
{#snippet titleHint()}
  <!-- side="right", not "bottom": the trigger sits at the modal's left edge, and a
       centered tooltip (left-1/2 -translate-x-1/2) grows equally both ways —
       overflowing the modal to the left no matter how short the content is.
       Growing rightward only stays inside it. -->
  <Tooltip side="right">
    <button
      type="button"
      aria-label="How filters work"
      class="flex size-4 items-center justify-center rounded-full text-muted-foreground transition-colors hover:text-foreground"
    >
      <Info class="size-3.5" aria-hidden="true" />
    </button>
    {#snippet content()}
      <a
        href={resolve('/features/advanced-search')}
        class="block whitespace-nowrap font-medium text-foreground underline-offset-2 hover:underline"
      >
        See how filters work →
      </a>
    {/snippet}
  </Tooltip>
{/snippet}

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
      href={resolve('/my/profile')}
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
    <CategoryPane store={staged} {plain} counts={c} />
    {#if !exclude.includes('ai_archetype')}
      {@const aiArchetypeDef = facetDefFor('ai_archetype')}
      {#if aiArchetypeDef}
        <div class="mt-6"><FacetSection def={aiArchetypeDef} store={staged} counts={c} expand /></div>
      {/if}
    {/if}
  {:else if entry.kind === 'location'}
    <LocationPane store={staged} counts={c} />
  {:else if entry.kind === 'facet'}
    {@const def = facetDefFor(entry.facetParam)}
    {#if entry.key === 'skills' && matchAvailable}
      <!-- Client-only post-filter (see `minMatch`/`onMinMatchChange` props): re-filters
           the already-fetched page in memory, so it applies immediately, no debounce. -->
      <div class="mb-2 flex items-center justify-between">
        <h3 class="text-sm font-semibold tracking-tight">Minimum skill match</h3>
        <span class="text-xs font-medium text-muted-foreground">{minMatch != null ? `${minMatch}%+` : 'Any'}</span>
      </div>
      <input
        type="range"
        min="0"
        max="100"
        step="5"
        value={minMatch ?? 0}
        oninput={(e) => onMinMatchChange?.(Number(e.currentTarget.value) || null)}
        aria-label="Minimum skill match"
        class="mb-6 w-full accent-primary"
      />
    {/if}
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
  {:else if entry.kind === 'experience'}
    {@const showSeniority = !exclude.includes('seniority')}
    {@const showRoleType = !exclude.includes('role_type')}
    {#if showSeniority}
      <ChipFacet store={staged} param="seniority" label="Seniority" counts={c} />
    {/if}
    <!-- Directly beneath seniority on purpose: the two are the axes users conflate.
         "Lead" reads to many as a management grade, while in this catalogue it names
         the IC ladder — of the 116,893 postings at seniority=lead, only 3,303 carry
         any management marker. Adjacency is what makes them read as two questions. -->
    {#if showRoleType}
      <div class:mt-6={showSeniority}>
        <ChipFacet store={staged} param="role_type" label="Role type" counts={c} />
      </div>
    {/if}
    <div class:mt-6={showSeniority || showRoleType}>
      <div class="mb-2 flex items-center justify-between">
        <h3 class="text-sm font-semibold tracking-tight">Years of experience</h3>
        <span class="text-xs font-medium text-muted-foreground"
          >{experienceLabel(staged.value.experienceYearsMax)}</span
        >
      </div>
      <input
        type="range"
        min="0"
        max={EXPERIENCE_PRESETS.length - 1}
        step="1"
        value={experienceIndex}
        oninput={(e) => staged.setExperienceYearsMax(EXPERIENCE_PRESETS[Number(e.currentTarget.value)]?.years ?? null)}
        aria-label="Maximum years of experience"
        class="w-full accent-primary"
      />
      <!-- Roughly half the catalogue states no experience requirement at all, and a
           bound excludes every one of those postings. Shown permanently rather than
           only once bounded: a result count that collapses without explanation is
           read as a broken filter, and by then the user has already been misled. The
           sentence is therefore about what SETTING a bound does, so it stays true at
           the unbounded "Any" stop instead of describing a filter that is not on. -->
      <p class="mt-2 text-xs text-muted-foreground">
        Setting a limit matches only postings that state an experience requirement — about half of them.
      </p>
    </div>
  {:else if entry.kind === 'work'}
    <ChipFacet store={staged} param="work_mode" label="Work format" counts={c} />
    <div class="mt-6"><ChipFacet store={staged} param="employment_type" label="Employment type" counts={c} /></div>
  {:else if entry.kind === 'industry'}
    <ChipFacet store={staged} param="domains" label="Industry" counts={c} />
    <div class="mt-6"><ChipFacet store={staged} param="company_type" label="Company type" counts={c} /></div>
    <div class="mt-6">
      <ChipFacet store={staged} param="collections" label="Collection" counts={c} options={JOB_COLLECTION} />
    </div>
  {:else if entry.kind === 'language'}
    {#if englishDef}<FacetSection def={englishDef} store={staged} counts={c} expand />{/if}
    <div class="mt-4">{#if postingDef}<FacetSection def={postingDef} store={staged} counts={c} expand />{/if}</div>
  {:else if entry.kind === 'relocation'}
    <ChipFacet store={staged} param="relocation" label="Relocation" counts={c} />
    <div class="mt-6">
      <ChipFacet
        store={staged}
        param="collections"
        label="Employer credentials"
        counts={c}
        options={EMPLOYER_CREDENTIALS}
      />
    </div>
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
