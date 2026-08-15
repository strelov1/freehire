<script lang="ts">
  import { untrack } from 'svelte';
  import { Globe, ScanSearch, Trash2, VenetianMask } from '@lucide/svelte';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { api } from '$lib/api';
  import { BASE_REFRESH_MESSAGE, offerCvRefresh } from '$lib/cvRefreshOffer';
  import { askCvRefresh } from '$lib/cvRefreshDialog.svelte';
  import { currentUser, isAuthenticated } from '$lib/auth.svelte';
  import { FilterStore, filtersFromProfile, filtersToParams } from '$lib/filters';
  import { savedSearches } from '$lib/savedSearches.svelte';
  import ATSReportView from '$lib/components/ATSReportView.svelte';
  import AccountLanguage from '$lib/components/AccountLanguage.svelte';
  import AccountTimezone from '$lib/components/AccountTimezone.svelte';
  import CandidateContactsEditor from '$lib/components/CandidateContactsEditor.svelte';
  import DeleteAccountButton from '$lib/components/DeleteAccountButton.svelte';
  import ExperienceBankView from '$lib/components/ExperienceBankView.svelte';
  import FilterSummary from '$lib/components/filters/FilterSummary.svelte';
  import FilterModal from '$lib/components/filters/FilterModal.svelte';
  import FilterEdgeTab from '$lib/components/FilterEdgeTab.svelte';
  import ProfileForm from '$lib/components/ProfileForm.svelte';
  import ScreeningAnswersForm from '$lib/components/ScreeningAnswersForm.svelte';
  import SkillsView from '$lib/components/SkillsView.svelte';
  import States from '$lib/components/States.svelte';
  import { ConfirmDialog, TabStrip, tabStripId } from '$lib/ui';
  import { profileStore } from '$lib/profile.svelte';
  import type {
    ATSResponse,
    CandidateContacts,
    FacetCounts,
    TalentNetworkVisibility,
  } from '$lib/types';
  import type { Answers } from '$lib/generated/contracts';
  import { Button } from '$lib/ui';

  const profile = $derived(profileStore.profile);

  // Skills are the measured set (from the profile), never a market filter — hide the
  // skills facet so the sidebar can't turn them into one.
  const excludeFacets = ['skills'];

  let status = $state<'loading' | 'error' | 'ready'>('loading');
  let filters = $state<FilterStore | null>(null);
  let counts = $state<FacetCounts | null>(null);
  let ats = $state<ATSResponse | null>(null);
  // structurePending is true while a newer upload's extract has not stamped yet —
  // contacts may still be provisional from the previous parse.
  let structurePending = $state(false);
  let parseStatus = $state('');
  let parseDetail = $state('');
  let contacts = $state<CandidateContacts | null>(null);
  let screeningAnswers = $state<Answers | null>(null);
  let loadError = $state(false);
  // The tab strip's sections. `as const` ties `tab` to this list, so a section can't be
  // referenced by an id the strip doesn't offer.
  const TABS = [
    { id: 'settings', label: 'Settings' },
    { id: 'screening', label: 'Screening answers' },
    { id: 'skills', label: 'Skills' },
    { id: 'structured', label: 'Profile' },
    { id: 'experience', label: 'Experience' },
    { id: 'readiness', label: 'CV readiness' },
  ] as const;
  const PANEL_ID = 'profile-panel';
  let tab = $state<'settings' | 'screening' | 'skills' | 'structured' | 'experience' | 'readiness'>('settings');
  let modalOpen = $state(false);
  let actionError = $state<string | null>(null);

  // Status-aware Talent Network entry button in the page header — links to
  // /my/talent-network. `null` means "not yet loaded" — the button renders in its
  // "off" (join) state until the fetch resolves, same fail-safe posture as a load
  // failure below.
  let talentNetworkVisibility = $state<TalentNetworkVisibility | null>(null);

  // Optimistic CV flag: a résumé upload stores the CV server-side before the next ATS
  // fetch resolves (and before any profile exists during set-up), so reflect it at once.
  let cvUploaded = $state(false);
  const hasCv = $derived((ats?.has_cv ?? false) || cvUploaded);

  // Job-count preview for the modal's staged filters — the same facet call, total only.
  const previewCount = (params: URLSearchParams) => api.facetCounts(params).then((c) => c.total);

  // AI review state.
  let reviewBusy = $state(false);
  let reviewUnavailable = $state(false);

  // Run the optional LLM review over the stored CV; folds content-quality + suggestions
  // into the report. When the server has no LLM the report comes back unreviewed — flag
  // that so the UI stops offering the button.
  async function runReview() {
    reviewBusy = true;
    reviewUnavailable = false;
    try {
      const params = filters ? filtersToParams(filters.applied) : undefined;
      const next = await api.runATSReview(params);
      ats = next;
      if (next.has_cv && next.report && !next.report.reviewed) {
        reviewUnavailable = true;
      }
    } catch {
      reviewUnavailable = true;
    } finally {
      reviewBusy = false;
    }
  }

  // Seed the comparison filter from the profile's specializations (unless the URL already
  // carries a category) — so it opens on the profile's own role, which the user can then
  // change to compare against another position without touching the saved profile.
  function buildFilters(specializations: string[]): FilterStore {
    // eslint-disable-next-line svelte/prefer-svelte-reactivity -- transient: seeds a FilterStore once, never stored as reactive state
    const seed = new URLSearchParams(page.url.searchParams);
    if (!seed.getAll('category').some((c) => c !== '')) {
      for (const spec of specializations) seed.append('category', spec);
    }
    return new FilterStore(seed);
  }

  async function load() {
    status = 'loading';
    try {
      await profileStore.ensureLoaded();
      status = 'ready';
    } catch {
      status = 'error';
    }
    void loadStructured();
    void loadScreeningAnswers();
  }

  // Best-effort, independent of the filter-driven reload — a failure here leaves the
  // section blank on next load rather than erroring the whole profile page.
  async function loadScreeningAnswers() {
    try {
      screeningAnswers = await api.getScreeningAnswers();
    } catch {
      screeningAnswers = null;
    }
  }

  // Fetch the read-only structured résumé independently of the filter-driven reload.
  // Best-effort: any failure (or none current) leaves the section hidden, never an error.
  async function loadStructured() {
    try {
      const meta = await api.getResume();
      structurePending = Boolean(meta.structure_pending);
      parseStatus = meta.parse_status ?? '';
      parseDetail = meta.parse_detail ?? '';
      contacts = meta.contacts ?? null;
    } catch {
      structurePending = false;
      parseStatus = '';
      parseDetail = '';
      contacts = null;
    }
  }

  // Seeds the header button's display only — never blocking, and no error surfaced here.
  // A failure defaults the button to its "off" (join) state; the panel's own fetch (on
  // open) is the informative path if the setting genuinely can't be read.
  async function loadTalentNetwork() {
    try {
      const setting = await api.getTalentNetwork();
      talentNetworkVisibility = setting.talent_network_visibility;
    } catch {
      talentNetworkVisibility = 'off';
    }
  }

  // (Re)load once the session resolves. Talent Network is beta-gated: skip its fetch
  // entirely for a non-beta account, same as the button being hidden for one.
  $effect(() => {
    if (isAuthenticated()) {
      void load();
      if (currentUser()?.beta_tester) void loadTalentNetwork();
    }
  });

  // Build the filter only on the profile null↔exists transition, never on a plain edit —
  // so refining the comparison role survives a skills/role save. Delete drops it back to
  // the set-up form.
  $effect(() => {
    const p = profile;
    untrack(() => {
      if (p && !filters) {
        filters = buildFilters(p.specializations);
      } else if (!p && filters) {
        filters.dispose();
        filters = null;
      }
    });
  });

  // Reload the facet counts + ATS report whenever the applied (debounced) filters
  // change. No filter (no profile) → nothing to compute.
  $effect(() => {
    const f = filters;
    if (!f) return;
    void f.applied;
    void reload();
  });

  // reloadGeneration guards against out-of-order responses: fast filter changes can have
  // an older request resolve after a newer one, so only the latest reload commits.
  let reloadGeneration = 0;
  async function reload() {
    if (!filters) return;
    const gen = ++reloadGeneration;
    const params = filtersToParams(filters.applied);
    // Settled separately: a Meili facet-settings lag (new filterable attr not applied yet)
    // must not blank the ATS report when that endpoint is fine. Facet counts degrade to
    // empty; the report still loads.
    const [a, c] = await Promise.allSettled([api.getATSReport(params), api.facetCounts(params)]);
    if (gen !== reloadGeneration) return;
    if (a.status !== 'fulfilled') {
      loadError = true;
      return;
    }
    ats = a.value;
    counts = c.status === 'fulfilled' ? c.value : null;
    loadError = false;
  }

  // ProfileForm callbacks: a save re-fetches coverage; a CV upload also refreshes the
  // stored-CV state. During set-up both reloads are no-ops (reload bails without a
  // filter), but cvUploaded still flips so the drop-zone shows the uploaded state at once.
  function handleSaved() {
    void reload();
    void syncProfileAlert();
  }

  // Keep the profile-derived saved search (the "notify me about jobs matching my
  // profile" toggle, on the Search alerts page — ProfileAlertToggle) in step with a
  // changed role/skills/location — otherwise it would keep alerting on the profile as it
  // stood when first enabled. Best-effort: a failure here never blocks or rolls back the
  // profile save itself, it just leaves the alert stale until the next successful save.
  async function syncProfileAlert() {
    const p = profileStore.profile;
    if (!p) return;
    await savedSearches.ensureLoaded();
    const existing = savedSearches.items.find((s) => s.derived_from_profile);
    if (!existing) return;
    try {
      await savedSearches.update(existing.id, {
        query: filtersToParams(filtersFromProfile(p)).toString(),
      });
    } catch {
      // best-effort — see doc comment.
    }
  }
  function handleCvUploaded() {
    cvUploaded = true;
    void reload();
    // The structured résumé is derived in the background (seconds), so it usually is not
    // ready this instant; re-fetch anyway — it lands on the next profile visit otherwise.
    void loadStructured();
  }

  function offerRefreshAfterBankEdit() {
    void offerCvRefresh({
      message: BASE_REFRESH_MESSAGE,
      confirm: askCvRefresh,
      apply: async () => {
        // Cleared on the way in, so a failure from one edit does not outlive the next one that
        // succeeds — the banner sits under the page header and nothing else would ever drop it.
        actionError = null;
        try {
          await api.resetBaseCvFromResume();
        } catch {
          actionError = 'Could not update your base CV. Try Reset from résumé in a tailoring workspace.';
        }
      },
    });
  }

  let confirmRemoveOpen = $state(false);

  async function remove() {
    actionError = null;
    try {
      await profileStore.clear();
      cvUploaded = false;
    } catch {
      actionError = 'Could not delete the profile. Please try again.';
    }
  }
</script>

<svelte:head>
  <title>Profile — freehire</title>
</svelte:head>

<!-- The account shell (my/+layout) owns the container, auth gate, and noindex. -->
{#if status === 'loading'}
  <States state="loading" />
{:else if status === 'error'}
  <States state="error" message="Couldn't load your profile." />
{:else}
  <!-- Header -->
  <div class="mb-6 flex flex-col items-start gap-4 sm:flex-row sm:items-start sm:justify-between">
    <div class="flex flex-col gap-1">
      <h1 class="text-2xl font-semibold tracking-tight">Profile</h1>
      <p class="text-sm text-muted-foreground">
        Your CV, skills and role — measured against live market demand.
      </p>
    </div>
    <!-- Off/not-yet-loaded is a filled call-to-action (the opt-in is the interesting
         choice to make); once public or anonymous the button becomes a low-key status
         readout, since Off is the default and the other two are already a
         deliberate, weighty decision the /my/talent-network page itself re-explains.
         Beta-gated: hidden entirely for a non-beta account, not just disabled — the
         feature isn't ready for a general audience yet. -->
    {#if currentUser()?.beta_tester}
      <Button
        variant={talentNetworkVisibility === 'public' || talentNetworkVisibility === 'anonymous'
          ? 'outline'
          : 'primary'}
        class="shrink-0"
        href={resolve('/my/talent-network')}
      >
        {#if talentNetworkVisibility === 'public'}
          <Globe class="size-4" aria-hidden="true" /> Talent Network: Public
        {:else if talentNetworkVisibility === 'anonymous'}
          <VenetianMask class="size-4" aria-hidden="true" /> Talent Network: Anonymous
        {:else}
          Join Talent Network
        {/if}
      </Button>
    {/if}
  </div>

  {#if actionError}
    <p class="mb-4 text-sm text-destructive">{actionError}</p>
  {/if}

  <!-- Run / Re-run AI review control, rendered inside the CV-readiness section header
       (via ATSReportView's `action` slot) rather than crammed into the tab row. -->
  {#snippet reviewAction()}
    {#if ats?.report && !ats.report.reviewed && !reviewUnavailable}
      <Button variant="primary" onclick={runReview} disabled={reviewBusy}>
        <ScanSearch class="size-4 {reviewBusy ? 'animate-pulse' : ''}" />
        {reviewBusy ? 'Reviewing…' : 'Run AI review'}
      </Button>
    {:else if ats?.report?.reviewed}
      <Button variant="ghost" onclick={runReview} disabled={reviewBusy}>
        <ScanSearch class="size-4 {reviewBusy ? 'animate-pulse' : ''}" />
        {reviewBusy ? 'Reviewing…' : 'Re-run AI review'}
      </Button>
    {/if}
  {/snippet}

  {#if profile === null}
    <!-- Set-up: the inline form only; coverage appears once a profile exists. Full-width to
         match the loaded profile tab, whose main column also spans the container (no aside). -->
    <div class="w-full">
      <ProfileForm profile={null} {hasCv} onSaved={handleSaved} onCvUploaded={handleCvUploaded} />
      <!-- Same reasoning as the timezone field's placement below: set-up is the
           settings tab before there are tabs, so account-level (not
           candidate-profile) settings live here too rather than waiting on a
           CV upload. -->
      <div class="mt-6 flex flex-col gap-4">
        <AccountTimezone />
        <AccountLanguage />
      </div>
      <!-- Set-up is the settings tab before there are tabs, so leaving is offered here
           too: someone who signed up, filled in nothing and wants out must not have to
           create a profile first to find the way back out. -->
      <div class="mt-2 flex justify-end border-t border-border pt-4">
        <DeleteAccountButton />
      </div>
    </div>
  {:else}
    <div class="flex gap-6">
      <main class="flex min-w-0 flex-1 flex-col gap-6">
        <!-- Tabs -->
        <TabStrip
          tabs={TABS}
          active={tab}
          onSelect={(id) => (tab = id)}
          label="Profile sections"
          panelId={PANEL_ID}
        />

        <!-- Body -->
        <div
          id={PANEL_ID}
          role="tabpanel"
          aria-labelledby={tabStripId(PANEL_ID, tab)}
          class="flex flex-col gap-6"
        >
        {#if tab === 'experience'}
          <!-- What the product has recorded about what this person has done, and the only
               place they can correct or remove it. -->
          <ExperienceBankView onBankMutated={offerRefreshAfterBankEdit} />
        {:else if tab === 'settings'}
          {#key profile.updated_at}
            <ProfileForm {profile} {hasCv} onSaved={handleSaved} onCvUploaded={handleCvUploaded} />
          {/key}
          <div class="mt-6 flex flex-col gap-4">
            <AccountTimezone />
            <AccountLanguage />
          </div>
          <!-- Destructive actions live at the foot of the settings tab, out of
               the page header (where they crowded the title on narrow viewports) and
               off the other tabs, which are readings of the market, not account
               settings. Both open behind a confirmation. -->
          <div class="mt-2 flex justify-end gap-2 border-t border-border pt-4">
            <Button
              variant="ghost"
              size="sm"
              onclick={() => (confirmRemoveOpen = true)}
              class="text-muted-foreground hover:bg-destructive/10 hover:text-destructive"
            >
              <Trash2 class="size-4" />
              Delete profile
            </Button>
            <DeleteAccountButton />
          </div>
        {:else if tab === 'screening'}
          <!-- Screening answers: a separate concern from the role/skills profile
               (facts the candidate states directly, not a targeting profile), and its own
               tab rather than a section of Settings — the six fields plus the assistant/
               autofill wiring behind them earn their own place in the nav. The component
               re-seeds its own fields from `answers` on reload (dirty-guarded, same pattern
               as CandidateContactsEditor), since there is no single identity field here to
               key a remount on. -->
          <ScreeningAnswersForm answers={screeningAnswers} onSaved={() => void loadScreeningAnswers()} />
        {:else if tab === 'skills'}
          <SkillsView />
        {:else if tab === 'structured'}
          <!-- Profile: editable contacts, parsed from (and kept in sync with) the stored CV. -->
          <CandidateContactsEditor
            {contacts}
            {parseStatus}
            {parseDetail}
            {structurePending}
            onSaved={() => void loadStructured()}
          />
        {:else if loadError}
          <States state="error" message="Couldn't load the report." />
        {:else if ats === null}
          <States state="loading" />
        {:else}
          <!-- CV readiness: the ATS-readiness score and the optional AI review. -->
          <div class="flex flex-col gap-6">
            {#if ats?.has_cv && ats.report}
              <div class="flex flex-col gap-5">
                {#if reviewUnavailable}
                  <p class="text-xs text-muted-foreground">AI review is not available right now.</p>
                {/if}
                <ATSReportView report={ats.report} action={reviewAction} />
              </div>
            {:else}
              <!-- No CV yet: uploaded via the Settings tab. -->
              <div class="flex flex-col items-start gap-2 rounded-xl border border-dashed border-border p-6">
                <p class="text-sm font-medium">Add your CV to score its ATS readiness</p>
                <p class="text-sm text-muted-foreground">
                  Upload your CV in the <button type="button" class="font-medium text-foreground underline underline-offset-2" onclick={() => (tab = 'settings')}>Settings</button> tab to check ATS readability and this role's keywords.
                </p>
              </div>
            {/if}
          </div>
        {/if}
        </div>
      </main>

      <!-- Filters refine CV readiness's keyword-match role only, so the summary sidebar
           shows on that tab alone — to the right of the content, clear of the account
           nav sidebar. -->
      {#if filters && tab === 'readiness'}
        <aside class="hidden w-72 shrink-0 md:block">
          <div class="sticky top-6 flex max-h-[calc(100vh-5rem)] flex-col gap-4 overflow-y-auto">
            <div class="rounded-xl border border-border bg-card p-4">
              <FilterSummary
                store={filters}
                exclude={excludeFacets}
                onOpen={() => (modalOpen = true)}
                description="Compare your CV's keyword strength against a role, region or seniority you choose."
              />
            </div>
          </div>
        </aside>
      {/if}
    </div>

    {#if filters && tab === 'readiness'}
      <FilterEdgeTab
        active={filters.active}
        onclick={() => (modalOpen = true)}
        side="right"
        class="top-[5.5rem]"
      />
      <FilterModal
        store={filters}
        {counts}
        exclude={excludeFacets}
        savedSearches
        open={modalOpen}
        onClose={() => (modalOpen = false)}
        {previewCount}
      />
    {/if}
  {/if}
{/if}

<ConfirmDialog
  bind:open={confirmRemoveOpen}
  title="Delete your profile?"
  confirmLabel="Delete"
  variant="destructive"
  onConfirm={remove}
/>
