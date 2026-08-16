<script lang="ts">
  import {
    Briefcase,
    ClipboardList,
    Contact,
    GraduationCap,
    Globe,
    MapPin,
    Settings as SettingsIcon,
    Tags,
    User,
    VenetianMask,
  } from '@lucide/svelte';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { filtersFromProfile, filtersToParams } from '$lib/filters';
  import { currentUser, isAuthenticated } from '$lib/auth.svelte';
  import { savedSearches } from '$lib/savedSearches.svelte';
  import { resumeStore } from '$lib/resume.svelte';
  import AccountPreferences from '$lib/components/AccountPreferences.svelte';
  import ExperienceBankView from '$lib/components/ExperienceBankView.svelte';
  import ProfileForm from '$lib/components/ProfileForm.svelte';
  import ScreeningAnswersForm from '$lib/components/ScreeningAnswersForm.svelte';
  import CandidateContactsEditor from '$lib/components/CandidateContactsEditor.svelte';
  import CvSummaryCard from '$lib/components/profile/CvSummaryCard.svelte';
  import EducationCard from '$lib/components/profile/EducationCard.svelte';
  import LocationCard from '$lib/components/profile/LocationCard.svelte';
  import RoleCard from '$lib/components/profile/RoleCard.svelte';
  import SkillsCard from '$lib/components/profile/SkillsCard.svelte';
  import States from '$lib/components/States.svelte';
  import { profileStore } from '$lib/profile.svelte';
  import { api } from '$lib/api';
  import { BASE_REFRESH_MESSAGE, offerCvRefresh } from '$lib/cvRefreshOffer';
  import { askCvRefresh } from '$lib/cvRefreshDialog.svelte';
  import type { TalentNetworkVisibility } from '$lib/types';
  import type { Answers } from '$lib/generated/contracts';
  import { Button } from '$lib/ui';

  const profile = $derived(profileStore.profile);
  const resumeMeta = $derived(resumeStore.meta);

  let status = $state<'loading' | 'error' | 'ready'>('loading');
  let screeningAnswers = $state<Answers | null>(null);
  // Eight lightweight views, not a tab strip: Profile (the default — CV upload/photo,
  // the read-only CV summary, and Roles), Contacts, Location, Skills, Experience,
  // Education (education + certifications), Screening answers, and Settings
  // (account-level config: timezone/language; account deletion lives on /my/security).
  // Contacts/Location/Skills/Experience/Education/Screening each get their own view
  // rather than an inline expand — every one of them carries as much content as the
  // last, so none earns being "the one that stays on the Profile view".
  //
  // CV readiness is deliberately NOT one of these — it's not published yet. It still
  // works at /my/profile/cv-readiness for anyone who holds the link (see that route),
  // same "unlisted, not deleted" posture as before; nothing in this page links to it.
  const VIEWS = [
    { id: 'profile', label: 'Profile', icon: User },
    { id: 'contacts', label: 'Contacts', icon: Contact },
    { id: 'location', label: 'Location', icon: MapPin },
    { id: 'skills', label: 'Skills', icon: Tags },
    { id: 'experience', label: 'Experience', icon: Briefcase },
    { id: 'education', label: 'Education', icon: GraduationCap },
    { id: 'screening', label: 'Screening answers', icon: ClipboardList },
    { id: 'settings', label: 'Settings', icon: SettingsIcon },
  ] as const;
  type ViewId = (typeof VIEWS)[number]['id'];
  function isViewId(id: string | null): id is ViewId {
    return VIEWS.some((v) => v.id === id);
  }
  // The four sub-routes this page used to have (contacts/experience/screening/skills)
  // now redirect here with ?tab=<id> (see their +page.ts) — honor it as the initial
  // view so an old bookmark or shared link still lands on the right section.
  const initialTab = page.url.searchParams.get('tab');
  let view = $state<ViewId>(isViewId(initialTab) ? initialTab : 'profile');
  let actionError = $state<string | null>(null);

  // A stale error from one view (currently only Experience's bank-edit refresh offer)
  // must not keep showing once the visitor has moved to an unrelated tab.
  $effect(() => {
    void view;
    actionError = null;
  });

  // Status-aware Talent Network entry point, next to the tab row — `null` means "not
  // yet loaded", which reads as the "off" (join) state until the fetch resolves, same
  // fail-safe posture as a load failure below.
  let talentNetworkVisibility = $state<TalentNetworkVisibility | null>(null);

  async function load() {
    status = 'loading';
    try {
      await profileStore.ensureLoaded();
      status = 'ready';
    } catch {
      status = 'error';
    }
    void resumeStore.ensureLoaded();
    void loadScreeningAnswers();
  }

  // Best-effort — a failure here leaves the section blank on next load rather than
  // erroring the whole profile page.
  async function loadScreeningAnswers() {
    try {
      screeningAnswers = await api.getScreeningAnswers();
    } catch {
      screeningAnswers = null;
    }
  }

  // Seeds the header's Talent Network pill only — never blocking, and no error surfaced
  // here. A failure defaults it to the "off" (join) state; the panel's own fetch (on open)
  // is the informative path if the setting genuinely can't be read.
  async function loadTalentNetwork() {
    try {
      const setting = await api.getTalentNetwork();
      talentNetworkVisibility = setting.talent_network_visibility;
    } catch {
      talentNetworkVisibility = 'off';
    }
  }

  // (Re)load once the session resolves. Talent Network is beta-gated: skip its fetch
  // entirely for a non-beta account, same as the pill being hidden for one.
  $effect(() => {
    if (isAuthenticated()) {
      void load();
      if (currentUser()?.beta_tester) void loadTalentNetwork();
    }
  });

  // Fired after any Role/Skills/Location change, wherever it happens now (ProfileForm's
  // batched Save during set-up, or a view's own per-field autosave): keeps the
  // profile-derived saved search aligned.
  function handleSaved() {
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

  // Roving-tabindex arrow-key navigation, matching the ARIA tabs pattern (WAI-ARIA
  // Authoring Practices) — the mouse-only onclick above doesn't give keyboard users the
  // same left/right/home/end movement a native tablist would.
  function handleTabKeydown(e: KeyboardEvent, current: ViewId) {
    const idx = VIEWS.findIndex((v) => v.id === current);
    let nextIdx: number | null = null;
    if (e.key === 'ArrowRight') nextIdx = (idx + 1) % VIEWS.length;
    else if (e.key === 'ArrowLeft') nextIdx = (idx - 1 + VIEWS.length) % VIEWS.length;
    else if (e.key === 'Home') nextIdx = 0;
    else if (e.key === 'End') nextIdx = VIEWS.length - 1;
    if (nextIdx === null) return;
    const next = VIEWS[nextIdx];
    if (!next) return;
    e.preventDefault();
    view = next.id;
    document.getElementById(`profile-tab-${next.id}`)?.focus();
  }

  function handleCvUploaded() {
    resumeStore.noteUpload();
  }

  function handleCvDeleted() {
    void resumeStore.refresh();
    // The deleted CV's headline/summary/education/certifications live on profile.cv,
    // sourced from resume_structured — clearing the file server-side does not touch
    // user_profiles, so this store has to re-fetch too or those sections stay stale.
    void profileStore.refresh();
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
</script>

<svelte:head>
  <title>Profile — freehire</title>
</svelte:head>

<!-- The account shell (my/+layout) owns the container, auth gate, and noindex. -->
{#if status === 'loading'}
  <States state="loading" />
{:else if status === 'error'}
  <States state="error" message="Couldn't load your profile." />
{:else if profile === null}
  <!-- Set-up: the inline form only; coverage appears once a profile exists. -->
  <div class="mb-6 flex flex-col gap-1">
    <h1 class="text-2xl font-semibold tracking-tight">Profile</h1>
    <p class="text-sm text-muted-foreground">
      Your CV, skills and role — measured against live market demand.
    </p>
  </div>
  <div class="w-full">
    <ProfileForm
      profile={null}
      hasCv={resumeStore.present}
      onSaved={handleSaved}
      onCvUploaded={handleCvUploaded}
      onCvDeleted={handleCvDeleted}
    />
    <!-- Set-up is Settings before there are views, so account-level (not candidate-profile)
         settings live here too rather than waiting on a CV upload. -->
    <AccountPreferences class="mt-6" />
  </div>
{:else}
  <!-- Underline tabs, same style as the Inbox page's Inbox/Settings switch. Icons match
       the account sidebar's convention (see accountNavIcons.ts) so the row scans the same
       way collapsed; the row scrolls horizontally on narrow viewports rather than wrapping,
       same as the account nav's own mobile strip in my/+layout.svelte. -->
  <div class="mb-6 flex items-end justify-between gap-4 border-b border-border text-sm">
    <div class="flex min-w-0 gap-4 overflow-x-auto" role="tablist" aria-label="Profile sections">
      {#each VIEWS as v (v.id)}
        {@const Icon = v.icon}
        <button
          type="button"
          role="tab"
          id="profile-tab-{v.id}"
          aria-selected={view === v.id}
          aria-controls="profile-panel"
          tabindex={view === v.id ? 0 : -1}
          onclick={() => (view = v.id)}
          onkeydown={(e) => handleTabKeydown(e, v.id)}
          class="-mb-px flex shrink-0 items-center gap-1.5 whitespace-nowrap border-b-2 px-1 py-2 transition-colors {view === v.id
            ? 'border-brand font-medium text-foreground'
            : 'border-transparent text-muted-foreground hover:text-foreground'}"
        >
          <Icon class="size-4" aria-hidden="true" />
          {v.label}
        </button>
      {/each}
    </div>
    {#if currentUser()?.beta_tester}
      <Button
        variant={talentNetworkVisibility === 'public' || talentNetworkVisibility === 'anonymous'
          ? 'outline'
          : 'primary'}
        size="sm"
        class="mb-2 shrink-0"
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

  <div id="profile-panel" role="tabpanel" aria-labelledby="profile-tab-{view}" tabindex="0">
  {#if view === 'profile'}
    {#key profile.updated_at}
      <ProfileForm
        {profile}
        hasCv={resumeStore.present}
        onSaved={handleSaved}
        onCvUploaded={handleCvUploaded}
        onCvDeleted={handleCvDeleted}
      />
    {/key}

    <div class="mt-4">
      <CvSummaryCard cv={profile.cv} />
    </div>

    <div class="mt-4">
      <RoleCard {profile} onProfileChanged={handleSaved} />
    </div>
  {:else if view === 'contacts'}
    <CandidateContactsEditor
      contacts={resumeMeta?.contacts ?? null}
      parseStatus={resumeMeta?.parse_status ?? ''}
      parseDetail={resumeMeta?.parse_detail ?? ''}
      structurePending={Boolean(resumeMeta?.structure_pending)}
      onSaved={() => void resumeStore.refresh()}
    />
  {:else if view === 'location'}
    <!-- LocationPreferencesFields seeds its local edit state once from `profile`, on the
         explicit contract that the caller remounts it on a genuinely different value (see
         its own doc comment) — profileStore.refresh() (CV delete/upload) can replace
         `profile` while this view is open, so key it the same way the Profile view keys
         ProfileForm. -->
    {#key profile.updated_at}
      <LocationCard {profile} onProfileChanged={handleSaved} />
    {/key}
  {:else if view === 'skills'}
    <SkillsCard />
  {:else if view === 'experience'}
    <!-- What the product has recorded about what this person has done, and the only
         place they can correct or remove it. -->
    <ExperienceBankView onBankMutated={offerRefreshAfterBankEdit} />
  {:else if view === 'education'}
    <EducationCard cv={profile.cv} />
  {:else if view === 'screening'}
    <ScreeningAnswersForm answers={screeningAnswers} onSaved={() => void loadScreeningAnswers()} />
  {:else}
    <AccountPreferences />
  {/if}
  </div>
{/if}
