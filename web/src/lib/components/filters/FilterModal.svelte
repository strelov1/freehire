<script lang="ts">
  import { resolve } from '$app/paths';
  import { PUBLIC_OPEN_WITHIN } from '$app/env/public';
  import { openWithinEnabled } from '#lib/features.js';
  import { Bell, Info, UserRound } from '@lucide/svelte';
  import { Tooltip } from '#lib/ui/index.js';
  import { FACETS, JOB_COLLECTION } from '#lib/facets.js';
  import { companySearch } from '#lib/companySearch.js';
  import { searchCities } from '#lib/facetOptions.js';
  import { FILTER_SEARCH_MIN_QUERY, jobFilterSearchItems, type FilterSearchHit } from '#lib/filterSearch.js';
  import { isAuthenticated } from '#lib/auth.svelte.js';
  import { promptSignIn } from '#lib/signin.js';
  import { profileStore } from '#lib/profile.svelte.js';
  import { notifications } from '#lib/notifications.svelte.js';
  import { emptyFilters, queryNarrows, savedSearchQuery } from '#lib/filters.js';
  import { StagedFilters } from '#lib/stagedFilters.svelte.js';
  import { SHORT_APPLICATION_MAX } from '#lib/jobFacts.js';
  import { RAIL, RAIL_SECTIONS, type RailEntry, type RailSection } from '#lib/filterSections.js';
  import type { FacetCounts } from '#lib/types.js';
  import FacetSection from '../facets/FacetSection.svelte';
  import ChipFacet from './ChipFacet.svelte';
  import CategoryPane from './CategoryPane.svelte';
  import ExperiencePane from './ExperiencePane.svelte';
  import SalaryPane from './SalaryPane.svelte';
  import WorkPane from './WorkPane.svelte';
  import PostedPane from './PostedPane.svelte';
  import RelocationPane from './RelocationPane.svelte';
  import LocationPane from './LocationPane.svelte';
  import FilterModalShell from './FilterModalShell.svelte';
  import { railEntryCount, selCount } from './railCounts';
  import { searchHitSelection, splitAction } from './searchHitAction';
  import type { FilterModalProps } from './FilterModal.props';
  import AiFilterButton from './AiFilterButton.svelte';
  import SavedSearches from '../SavedSearches.svelte';

  // The job-search filter modal (see FilterModalProps).
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
    formerEmployersAvailable = false,
    watchlistAvailable = false,
    hideSeenAvailable = false,
    open = false,
    onClose,
    previewCount,
    stagedCounts,
    extra,
    matchAvailable = false,
  }: FilterModalProps = $props();

  const staged = new StagedFilters();

  // The "My filters" tab is present only on the full job modal — the caller enables saved
  // searches and doesn't restrict the rail to a facet subset (as the profile modal does).
  // Gates the tab itself (visibleRail), its data warm-up, and the footer nudge that jumps
  // to it, so the jump never lands on a missing tab.
  const hasSavedTab = $derived(savedSearches && !railKeys);
  const showHideSeen = $derived(hideSeenAvailable && !plain && isAuthenticated());
  // The personal filters are applied by the search only: /jobs/facets ignores them, so a
  // count or a facet distribution fetched with any staged would describe a wider set
  // than "Show results" returns.
  const personalStaged = $derived(
    staged.value.formerEmployers || staged.value.watchlist || (showHideSeen && staged.value.hideSeen),
  );
  const showFormerEmployers = $derived(formerEmployersAvailable && !plain && isAuthenticated());
  const showWatchlist = $derived(watchlistAvailable && !plain && isAuthenticated());

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

  function applyProfileOrSignIn() {
    if (profile) staged.applyProfile(profile);
    else promptSignIn();
  }

  // The footer nudge shows only when the My-filters tab exists (so the jump lands
  // somewhere), Telegram alerts are available, and there's a search worth saving.
  const showSaveNudge = $derived(
    hasSavedTab && notifications.telegram.enabled && queryNarrows(savedSearchQuery(staged.value)),
  );

  // The "My filters" (saved searches) tab. It heads the rail on the full job modal, but
  // not when the caller restricts the rail to a facet subset (e.g. the profile modal),
  // which has no saved-search context.
  const SAVED_ENTRY: RailEntry = { key: 'saved', label: 'My filters', section: 'SAVED', kind: 'saved' };
  const SECTIONS: RailSection[] = ['SAVED', ...RAIL_SECTIONS];

  // Rail entries visible under the current scope: restricted to `railKeys` when given,
  // and a 'facet' entry is hidden when its param is excluded (e.g. Company on a company page).
  const visibleRail = $derived([
    ...(hasSavedTab ? [SAVED_ENTRY] : []),
    ...RAIL.filter((e) => (!railKeys || railKeys.includes(e.key)) && !(e.facetParam && exclude.includes(e.facetParam))),
  ]);

  function entryCount(e: RailEntry): number {
    return railEntryCount(e, staged.value, { showHideSeen, matchAvailable, showFormerEmployers, showWatchlist });
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
    } else if (store) {
      staged.commit(store);
    }
  }

  const applyDisabled = $derived(canApply ? !canApply(staged.value) : false);
  // Read straight from the runtime env rather than threaded through a prop: five callers
  // mount this modal and none of them has an opinion about the flag.
  const openWithinAvailable = openWithinEnabled({ PUBLIC_OPEN_WITHIN });

  let filterQuery = $state('');
  const specializationHint = $derived.by(() => {
    const q = filterQuery.trim().toLowerCase();
    return q.length >= 2 && ('specialization'.startsWith(q) || 'specializations'.startsWith(q));
  });
  let remoteHits = $state<FilterSearchHit[]>([]);
  const searchItems = $derived<FilterSearchHit[]>([
    ...(hasSavedTab
      ? [
          {
            id: 'jump:saved:My filters',
            label: 'My filters',
            group: 'My filters',
            railKey: 'saved',
            railLabel: 'My filters',
            aliases: ['saved searches'],
          },
        ]
      : []),
    ...jobFilterSearchItems({
      exclude,
      railKeys,
      openWithin: openWithinAvailable,
      formerEmployers: showFormerEmployers,
      watchlist: showWatchlist,
      hideSeen: showHideSeen,
    }),
    ...remoteHits,
  ]);

  $effect(() => {
    const q = filterQuery.trim();
    const wantCompany =
      open &&
      q.length >= FILTER_SEARCH_MIN_QUERY &&
      !exclude.includes('company_slug') &&
      (!railKeys || railKeys.includes('company_slug'));
    const wantCities =
      open &&
      q.length >= FILTER_SEARCH_MIN_QUERY &&
      !exclude.includes('cities') &&
      (!railKeys || railKeys.includes('location'));
    if (!wantCompany && !wantCities) {
      if (remoteHits.length > 0) remoteHits = [];
      return;
    }
    if (remoteHits.length > 0) remoteHits = [];
    let cancelled = false;
    const timer = setTimeout(() => {
      void Promise.all([
        wantCompany ? companySearch(q).catch(() => []) : Promise.resolve([]),
        wantCities ? searchCities(q).catch(() => []) : Promise.resolve([]),
      ]).then(([companies, cities]) => {
        if (cancelled) return;
        remoteHits = [
          ...companies.slice(0, 8).map((company) => ({
            id: `company_slug:company_slug:${company.value}`,
            label: company.label,
            group: 'Company',
            railKey: 'company_slug',
            railLabel: 'Company',
            param: 'company_slug',
            value: company.value,
          })),
          ...cities.slice(0, 8).map((city) => ({
            id: `location:cities:${city.value}`,
            label: city.label,
            group: 'City',
            railKey: 'location',
            railLabel: 'Location',
            param: 'cities',
            value: city.value,
          })),
        ];
      });
    }, 200);
    return () => {
      cancelled = true;
      clearTimeout(timer);
    };
  });

  const searchSelection = (hit: FilterSearchHit) => searchHitSelection(hit, staged.value, plain);

  function onSearchToggle(hit: FilterSearchHit) {
    const action = splitAction(hit.action);
    switch (action?.kind) {
      case 'former-employers':
        if (showFormerEmployers) staged.setFormerEmployers(!staged.value.formerEmployers);
        return;
      case 'watchlist':
        if (showWatchlist) staged.setWatchlist(!staged.value.watchlist);
        return;
      case 'hide-seen':
        if (showHideSeen) staged.setHideSeen(!staged.value.hideSeen);
        return;
      case 'visa':
        staged.setVisa(!staged.value.visa);
        return;
      case 'hide-ai':
        staged.setHideAIInterview(!staged.value.hideAIInterview);
        return;
      case 'auto-apply':
        staged.setAutoApplyAvailable(!staged.value.autoApplyAvailable);
        return;
      case 'short-apply':
        staged.setApplyQuestionsMax(staged.value.applyQuestionsMax != null ? null : SHORT_APPLICATION_MAX);
        return;
      case 'posted':
        staged.setPostedWithinDays(staged.value.postedWithinDays === Number(action.rest) ? null : Number(action.rest));
        return;
      case 'open':
        staged.setOpenWithinDays(staged.value.openWithinDays === Number(action.rest) ? null : Number(action.rest));
        return;
      case 'experience':
        staged.setExperienceYearsMax(
          staged.value.experienceYearsMax === Number(action.rest) ? null : Number(action.rest),
        );
        return;
    }
    if (!hit.param || hit.value == null) return;
    const excludable = !plain && (FACETS.find((d) => d.param === hit.param)?.excludable ?? false);
    if (excludable) staged.cycle(hit.param, hit.value);
    else staged.pick(hit.param, hit.value);
  }

  // Whether this modal offers the "describe your filters in words" entry point. It needs
  // a live store to write into, and it makes no sense in `plain` reuse (the profile
  // editor), where a facet value is a plain choice rather than a search.
  const showAiEntry = $derived(Boolean(store) && !plain);
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
  countsFetch={personalStaged ? undefined : stagedCounts}
  {pane}
  headerAction={showProfileAction ? profileAction : undefined}
  {titleHint}
  extra={extra || showAiEntry ? aboveThePane : undefined}
  {footerNote}
  {searchItems}
  {onSearchToggle}
  {searchSelection}
  bind:filterQuery
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
      class="relative flex size-4 items-center justify-center rounded-full text-muted-foreground transition-colors after:absolute after:-inset-1 hover:text-foreground"
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
    <!-- Signed-out: click sends the visitor to /signin; signed-in with a profile: applies it. -->
    <button
      type="button"
      onclick={applyProfileOrSignIn}
      class="flex h-9 items-center gap-1.5 rounded-lg bg-brand px-3 text-sm font-medium text-brand-foreground transition-opacity hover:opacity-90 shadow-sm"
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

{#snippet aboveThePane()}
  <!-- The AI entry point lives here rather than in the filters sidebar, which is
       `hidden md:block` — so on a phone, where the modal is the ONLY way to reach
       filters at all, the feature did not exist. Above the pane because it is a way to
       FILL the filters, so it reads before the controls that refine them.

       `onApplied={onClose}` is not decoration: the dialog writes straight to the live
       store, while this modal defers its own edits until Apply. Left open, it would
       later apply what it had staged before the dialog ran and silently undo it. -->
  {#if showAiEntry && store}
    <div class="mb-4">
      <AiFilterButton {store} onApplied={onClose} />
    </div>
  {/if}
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

<!-- A specialization or skill search that matches nothing hands the term to /dictionary, where
     a signed-in visitor can ask for it to be added; the link under each pane opens the same
     page empty. -->
{#snippet unrecognized(noun: string, param: 'specialization' | 'skill', term: string)}
  <p class="py-1 text-sm text-muted-foreground">
    No {noun} matches “{term}”.
    <!-- eslint-disable svelte/no-navigation-without-resolve -- resolve()d /dictionary plus a query string -->
    <a
      href={`${resolve('/dictionary')}?${new URLSearchParams({ [param]: term })}`}
      class="font-medium text-foreground underline underline-offset-4">Help us recognize it</a
    ><!-- eslint-enable svelte/no-navigation-without-resolve -->
  </p>
{/snippet}
{#snippet unrecognizedRole(term: string)}{@render unrecognized('specialization', 'specialization', term)}{/snippet}
{#snippet unrecognizedSkill(term: string)}{@render unrecognized('skill', 'skill', term)}{/snippet}

{#snippet recognizeLink(noun: string, query: string)}
  <!-- eslint-disable svelte/no-navigation-without-resolve -- resolve()d /dictionary plus a query string -->
  <a
    href={`${resolve('/dictionary')}${query}`}
    class="mt-6 inline-block text-sm text-muted-foreground underline-offset-4 hover:text-foreground hover:underline"
  >
    Can't find your {noun}? Help us recognize it →
  </a><!-- eslint-enable svelte/no-navigation-without-resolve -->
{/snippet}

{#snippet pane(entry: RailEntry, live: FacetCounts | null)}
  {@const c = personalStaged ? null : (live ?? counts)}
  {#if entry.kind === 'saved'}
    <SavedSearches store={staged} />
  {:else if entry.kind === 'category'}
    <CategoryPane store={staged} {plain} counts={c} empty={unrecognizedRole} hintSearch={specializationHint} />
    {#if !exclude.includes('ai_archetype')}
      {@const aiArchetypeDef = facetDefFor('ai_archetype')}
      {#if aiArchetypeDef}
        <div class="mt-6"><FacetSection def={aiArchetypeDef} store={staged} counts={c} expand /></div>
      {/if}
    {/if}
    {@render recognizeLink('role', '')}
  {:else if entry.kind === 'location'}
    <LocationPane store={staged} counts={c} />
  {:else if entry.kind === 'facet'}
    {@const def = facetDefFor(entry.facetParam)}
    {#if entry.key === 'company_slug' && showFormerEmployers}
      <div class="mb-6">
        <label class="flex cursor-pointer items-center gap-2 text-sm">
          <input
            type="checkbox"
            class="size-4 rounded border-border"
            checked={staged.value.formerEmployers}
            onchange={(e) => staged.setFormerEmployers(e.currentTarget.checked)}
          />
          <span>Hiring again</span>
        </label>
        <p class="mt-1 text-xs text-muted-foreground">Only jobs at companies you have worked at and left.</p>
      </div>
    {/if}
    {#if entry.key === 'company_slug' && showWatchlist}
      <div class="mb-6">
        <label class="flex cursor-pointer items-center gap-2 text-sm">
          <input
            type="checkbox"
            class="size-4 rounded border-border"
            checked={staged.value.watchlist}
            onchange={(e) => staged.setWatchlist(e.currentTarget.checked)}
          />
          <span>Your watchlist</span>
        </label>
        <p class="mt-1 text-xs text-muted-foreground">Only jobs at the companies you follow.</p>
      </div>
    {/if}
    {#if entry.key === 'skills' && matchAvailable}
      <!-- Under best match the server applies `minSkillMatch` across the catalogue; under any
           other ordering it narrows the fetched page in memory, so the footer's count does not move. -->
      <div class="mb-2 flex items-center justify-between">
        <h3 class="text-sm font-semibold tracking-tight">Minimum skill match</h3>
        <span class="text-xs font-medium text-muted-foreground"
          >{staged.value.minSkillMatch != null ? `${staged.value.minSkillMatch}%+` : 'Any'}</span
        >
      </div>
      <input
        type="range"
        min="0"
        max="100"
        step="5"
        value={staged.value.minSkillMatch ?? 0}
        oninput={(e) => staged.setMinSkillMatch(Number(e.currentTarget.value) || null)}
        aria-label="Minimum skill match"
        aria-valuetext={staged.value.minSkillMatch != null ? `${staged.value.minSkillMatch}% or more` : 'Any'}
        class="mb-6 h-6 w-full accent-primary"
      />
    {/if}
    {#if entry.key === 'source'}
      <!-- The kind of source sits above the source picker: "everything but aggregators"
           is one pill here, where it would be a hundred source exclusions below. -->
      <div class="mb-6"><ChipFacet store={staged} param="source_type" label="Source type" counts={c} /></div>
    {/if}
    {#if def}<FacetSection
        {def}
        store={staged}
        counts={c}
        expand
        empty={entry.key === 'skills' ? unrecognizedSkill : undefined}
      />{/if}
    {#if entry.key === 'skills'}{@render recognizeLink('skill', '?tab=skill')}{/if}
    {#if entry.key === 'perks'}
      <p class="mt-2 text-xs text-muted-foreground">Based on job descriptions — not every employer lists every perk.</p>
    {/if}
  {:else if entry.kind === 'salary'}
    <SalaryPane store={staged} counts={c} />
  {:else if entry.kind === 'experience'}
    <ExperiencePane store={staged} counts={c} {exclude} />
  {:else if entry.kind === 'work'}
    <WorkPane store={staged} counts={c} />
  {:else if entry.kind === 'industry'}
    <ChipFacet store={staged} param="domains" label="Industry" counts={c} />
    <div class="mt-6"><ChipFacet store={staged} param="company_type" label="Company type" counts={c} /></div>
    <div class="mt-6">
      <ChipFacet store={staged} param="collections" label="Collection" counts={c} options={JOB_COLLECTION} />
    </div>
  {:else if entry.kind === 'language'}
    {#if englishDef}<FacetSection def={englishDef} store={staged} counts={c} expand />{/if}
    <div class="mt-4">
      {#if postingDef}<FacetSection def={postingDef} store={staged} counts={c} expand />{/if}
    </div>
  {:else if entry.kind === 'relocation'}
    <RelocationPane store={staged} counts={c} clearancePicked={selCount(staged.value, 'clearance_levels')} />
  {:else if entry.kind === 'posted'}
    <PostedPane store={staged} counts={c} {showHideSeen} {openWithinAvailable} />
  {/if}
{/snippet}
