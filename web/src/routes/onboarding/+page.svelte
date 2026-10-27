<script lang="ts">
  // The onboarding tour: up to seven steps, all skippable — the CV, the feed, alerts, the
  // AI assistant, what is in the way, and the two paid features.
  //
  // GATING (whether to be on this route at all) lives in the root layout and reads one
  // fact: `users.onboarding_completed_at`. Null routes the account here. This page marks it
  // the moment the tour is SHOWN, not when it ends, so the redirect fires at most once — a
  // visitor who leaves on the first step is never pulled back.
  //
  // Every account comes through here once, new or existing (migration 0207 re-asked the
  // accounts the old wizard had marked), so the plan skips what an account already has and
  // every shown step opens with what is stored. An account that lacks nothing is marked
  // complete and sent on without a screen.
  //
  // An anonymous visitor has no steps to run: the effect below bounces them to /signin
  // (carrying this page's own URL, returnTo and all, as ITS returnTo) before any markup
  // renders, and /signin sends them right back once they have an account.
  import { onDestroy, untrack } from 'svelte';
  import { goto } from '$app/navigation';
  import { gotoPath } from '#lib/gotoPath.js';
  import { page } from '$app/state';
  import { ArrowLeft, ArrowRight, LoaderCircle, X } from '@lucide/svelte';
  import { api, ApiError } from '#lib/api.js';
  import { isAuthenticated } from '#lib/auth.svelte.js';
  import { filtersFromProfile, filtersToParams } from '#lib/filters.js';
  import { notifications } from '#lib/notifications.svelte.js';
  import { onboardingGate } from '#lib/onboardingGate.svelte.js';
  import { nextResumePollDelayMs } from '#lib/onboardingResumeWait.js';
  import { persistStep, type SaveDeps, type TourAnswers } from '#lib/onboardingSave.js';
  import { plannedSteps, TOUR_STEPS, type StepKind } from '#lib/onboardingSteps.js';
  import { profileStore } from '#lib/profile.svelte.js';
  import { profileAlertSearch } from '#lib/profileAlert.js';
  import { syncProfileAlert } from '#lib/profileAlertSync.js';
  import { safeRedirect } from '#lib/safeRedirect.js';
  import { savedSearches } from '#lib/savedSearches.svelte.js';
  import { signinUrl } from '#lib/signin.js';
  import { focusTrap } from '#lib/actions/focusTrap.js';
  import { Button } from '#lib/ui/index.js';
  import type { LocationPreferences } from '#lib/types.js';
  import type { MergedFacets } from '#lib/onboardingImport.js';
  import AgentStep from '#lib/components/onboarding/AgentStep.svelte';
  import AlertChannelsStep from '#lib/components/onboarding/AlertChannelsStep.svelte';
  import ChallengeStep from '#lib/components/onboarding/ChallengeStep.svelte';
  import CvStep from '#lib/components/onboarding/CvStep.svelte';
  import ExtensionStep from '#lib/components/onboarding/ExtensionStep.svelte';
  import FeedStep from '#lib/components/onboarding/FeedStep.svelte';
  import TailorStep from '#lib/components/onboarding/TailorStep.svelte';

  // Where finishing or skipping sends the visitor — the page they were on when they landed
  // here, not always home. Validated like every other returnTo (same-origin path only).
  const returnTo = $derived(safeRedirect(page.url.searchParams.get('returnTo')) ?? '/');

  $effect(() => {
    if (isAuthenticated()) return;
    // `returnTo` for /signin is THIS page's own URL, so a successful sign-in lands back
    // here. `cancelTo` is where this page's own returnTo points, so /signin's close button
    // skips both the credential form and the tour instead of looping back to /signin.
    const here = page.url.pathname + page.url.search;
    // eslint-disable-next-line svelte/no-navigation-without-resolve -- signinUrl() wraps resolve('/signin'); the rule can't see through the appended query
    void goto(signinUrl({ returnTo: here, cancelTo: returnTo }));
  });

  // ---- what the tour is holding ----

  let specializations = $state.raw<string[]>([]);
  let seniorities = $state.raw<string[]>([]);
  let skills = $state.raw<string[]>([]);
  let location = $state<LocationPreferences | null>(null);
  let challenges = $state.raw<string[]>([]);
  let challengeNote = $state('');
  // The feed step's values came from a CV parsed on this run.
  let fromCv = $state(false);

  let steps = $state.raw<StepKind[]>(TOUR_STEPS);
  let loaded = $state(false);
  let index = $state(0);
  const currentKind = $derived<StepKind | undefined>(steps[index]);
  const isLast = $derived(index >= steps.length - 1);

  // One load of everything the plan depends on. Every read must succeed before the plan is
  // made: a failed one would read as "nothing stored", and since showing the tour marks it
  // complete, an outage would spend the account's one tour on the wrong steps. A failure
  // shows a retry instead and records nothing.
  //
  // Run once, untracked: the stores' own `loaded` flags flip while this reads them, and a
  // tracked read would start a second load that lands after the first and overwrites
  // whatever the candidate picked in between.
  let started = false;
  $effect(() => {
    if (!isAuthenticated() || started) return;
    started = true;
    untrack(() => void load());
  });

  let loadFailed = $state(false);

  async function load() {
    loadFailed = false;
    let failed = false;
    const failure = () => {
      failed = true;
      return null;
    };
    // The stores swallow their own errors and stay not-loaded, so their flags are the
    // success signal; the two direct reads report through `failure`.
    const [resume, survey] = await Promise.all([
      api.getResume().catch(failure),
      api.getSurvey().catch(failure),
      profileStore.ensureLoaded(),
      savedSearches.ensureLoaded(),
      notifications.ensureLoaded(),
    ]);
    if (failed || !profileStore.loaded || !savedSearches.loaded || !notifications.loaded) {
      loadFailed = true;
      return;
    }
    const profile = profileStore.profile;
    specializations = profile ? [...profile.specializations] : [];
    seniorities = profile ? [...profile.seniorities] : [];
    skills = profile ? [...profile.skills] : [];
    location = profile?.location_preferences ?? null;
    challenges = survey?.biggest_challenges ?? [];
    challengeNote = survey?.biggest_challenge_note ?? '';

    const alert = profileAlertSearch();
    steps = plannedSteps({
      hasResume: resume?.present === true,
      hasFeed: profile !== null && profile.specializations.length > 0 && profile.skills.length > 0,
      hasAlert: alert !== null && notifications.subscriptions.some((s) => s.saved_search_id === alert.id),
      hasChallenge: challenges.length > 0,
    });
    index = 0;
    loaded = true;

    // Shown means done: mark it now, so leaving on any step never brings the tour back.
    // Best-effort — a failed write only means being offered the tour once more.
    //
    // The raw call, not auth's invalidating one: re-reading the session here would hand the
    // layout a completed account while it is ON /onboarding, and its arrival check would
    // send the candidate away from the tour they were just shown. For the rest of this
    // visit the gate's dismissal keeps them from being routed back; the next load reads
    // the marker fresh.
    onboardingGate.dismiss();
    void api.completeOnboarding().catch(() => {});
    // Nothing to ask — go straight on.
    if (steps.length === 0) leave();
  }

  // ---- the feed step's answer, live ----

  // How many open jobs the feed step's picks match. One debounced read, cancelled by the
  // next change; a failure hides the figure rather than leaving a stale one.
  let matchCount = $state<number | null>(null);
  $effect(() => {
    if (currentKind !== 'feed') return;
    // The picks over whatever the stored profile carries besides them (its exclusions), so
    // the figure is the feed the save is about to produce.
    const params = filtersToParams(
      filtersFromProfile({
        excluded_skills: [],
        excluded_sources: [],
        excluded_companies: [],
        derived_location: null,
        cv: null,
        created_at: null,
        updated_at: null,
        ...untrack(() => profileStore.profile),
        specializations,
        seniorities,
        skills,
        location_preferences: location,
      }),
    );
    const ctrl = new AbortController();
    const timer = setTimeout(() => {
      api
        .searchJobs(params, 1, 0, ctrl.signal)
        .then((s) => (matchCount = s.total ?? null))
        .catch(() => {
          if (!ctrl.signal.aborted) matchCount = null;
        });
    }, 300);
    return () => {
      clearTimeout(timer);
      ctrl.abort();
    };
  });

  // ---- saving ----

  let saving = $state(false);
  let saveError = $state<string | null>(null);

  const answers = $derived<TourAnswers>({
    specializations,
    skills,
    seniorities,
    excludedSkills: profileStore.profile?.excluded_skills ?? [],
    excludedSources: profileStore.profile?.excluded_sources ?? [],
    excludedCompanies: profileStore.profile?.excluded_companies ?? [],
    location,
    challenges,
    challengeNote,
  });

  const saveDeps: SaveDeps = {
    saveProfile: async (spec, sk, sen, excl, exclSources, exclCompanies, loc) => {
      const result = await profileStore.save(spec, sk, sen, excl, exclSources, exclCompanies, loc);
      void syncProfileAlert();
      return result;
    },
    updateSurvey: (p) => api.updateSurvey(p),
  };

  /** Save the current step and move on. On the last one, finish. */
  async function advance() {
    if (saving || !loaded || currentKind === undefined) return;
    saving = true;
    saveError = null;
    try {
      await persistStep(currentKind, answers, saveDeps);
    } catch (err) {
      // The server's own message when there is one: a rejection names the field it
      // refused, which is something the candidate can act on.
      saveError =
        err instanceof ApiError
          ? `${err.message}. Fix it above, or skip this step.`
          : "Couldn't save that just now. Try again, or skip this step.";
      saving = false;
      return;
    }
    saving = false;
    next();
  }

  /** Move on WITHOUT saving. Guarded on `saving` so Continue-then-Skip cannot advance twice
   *  and pass a step unseen. */
  function skip() {
    if (saving || !loaded) return;
    next();
  }

  function next() {
    if (isLast) leave();
    else index += 1;
  }

  function back() {
    if (index > 0) index -= 1;
  }

  /** Go where the visitor was headed. Completion was recorded when the tour was shown. */
  function leave() {
    onboardingGate.dismiss();
    void gotoPath(returnTo);
  }

  // ---- the CV ----

  function onExtracted(merged: MergedFacets) {
    specializations = merged.specializations;
    seniorities = merged.seniorities;
    skills = merged.skills;
    fromCv = merged.resolved;
  }

  // A stored CV starts a background parse; once it lands the server can derive where the
  // candidate is, which reaches the feed step's location field only through the profile
  // store — read on arrival, before this CV existed. Poll until the parse settles, then
  // refresh it. Best-effort and bounded by the backoff table.
  let waitToken = 0;
  async function waitForResumeStructure() {
    const token = ++waitToken;
    for (let attempt = 0; ; attempt++) {
      // oxlint-disable-next-line no-await-in-loop -- a backoff poll IS sequential: each read decides whether there is a next one
      const resume = await api.getResume().catch(() => null);
      if (token !== waitToken) return;
      if (resume && (!resume.enabled || !resume.present)) return;
      if (resume && resume.parse_status !== 'pending') {
        if (resume.parse_status !== 'failed') void profileStore.refresh();
        return;
      }
      const delay = nextResumePollDelayMs(attempt);
      if (delay === null) return;
      // oxlint-disable-next-line no-await-in-loop -- the wait between reads is the point
      await new Promise((resolve) => setTimeout(resolve, delay));
      if (token !== waitToken) return;
    }
  }

  // Stop polling when the tour goes away — and, after a sign-out, for an account that is no
  // longer the caller.
  onDestroy(() => {
    waitToken++;
  });
</script>

{#if !isAuthenticated()}
  <!-- Redirecting to /signin — see the effect above. -->
{:else}
  <div class="fixed inset-0 z-50 flex flex-col bg-background">
    <div
      class="flex h-full w-full flex-col overflow-hidden"
      role="dialog"
      aria-modal="true"
      aria-label="Get to know freehire"
      {@attach focusTrap()}
    >
      <!-- header: progress + skip + close -->
      <div class="flex items-center gap-3 border-b border-border px-5 py-3">
        <div class="flex flex-1 items-center gap-1.5" aria-hidden="true">
          {#each steps as _, i (i)}
            <span class={['h-1 w-7 rounded-full transition-colors', i <= index ? 'bg-brand' : 'bg-border']}></span>
          {/each}
        </div>
        <button
          type="button"
          onclick={skip}
          disabled={saving || !loaded}
          class="text-sm font-medium text-muted-foreground transition-colors hover:text-foreground disabled:opacity-60"
        >
          Skip
        </button>
        <button
          type="button"
          onclick={leave}
          aria-label="Close"
          class="flex size-8 items-center justify-center rounded-lg border border-border text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
        >
          <X class="size-4" />
        </button>
      </div>

      <!-- body -->
      <div class="no-scrollbar mx-auto min-h-0 w-full max-w-2xl flex-1 overflow-y-auto px-5 py-6">
        <h1 class="sr-only">Get to know freehire</h1>
        {#if loadFailed}
          <div class="py-10 text-sm" role="alert">
            <p class="font-medium">We couldn't load your details.</p>
            <p class="mt-1 text-muted-foreground">Check your connection and try again.</p>
            <Button variant="outline" size="sm" class="mt-4" onclick={() => void load()}>Try again</Button>
          </div>
        {:else if !loaded}
          <div class="flex items-center gap-2 py-10 text-sm text-muted-foreground">
            <LoaderCircle class="size-4 animate-spin" aria-hidden="true" /> Getting your details…
          </div>
        {:else}
          <p class="mb-1 text-xs font-semibold tabular-nums text-brand-strong">{index + 1} / {steps.length}</p>
          {#if currentKind === 'cv'}
            <CvStep
              staged={{ specializations, seniorities, skills }}
              {onExtracted}
              onCvUploaded={() => void waitForResumeStructure()}
              onAdvance={() => void advance()}
              onSkip={skip}
            />
          {:else if currentKind === 'feed'}
            <FeedStep
              {specializations}
              {seniorities}
              {skills}
              {location}
              derivedLocation={profileStore.profile?.derived_location}
              {fromCv}
              onSpecializationsChange={(v) => (specializations = v)}
              onSenioritiesChange={(v) => (seniorities = v)}
              onSkillsChange={(v) => (skills = v)}
              onLocationChange={(v) => (location = v)}
            />
          {:else if currentKind === 'alerts'}
            <AlertChannelsStep profile={profileStore.profile} />
          {:else if currentKind === 'agent'}
            <AgentStep />
          {:else if currentKind === 'challenge'}
            <ChallengeStep
              value={challenges}
              note={challengeNote}
              onChange={(v) => (challenges = v)}
              onNoteChange={(n) => (challengeNote = n)}
            />
          {:else if currentKind === 'tailor'}
            <TailorStep />
          {:else if currentKind === 'extension'}
            <ExtensionStep />
          {/if}
        {/if}
      </div>

      <!-- footer: back · match count · continue/finish -->
      <div class="mx-auto w-full max-w-2xl border-t border-border px-5 py-3">
        {#if saveError}
          <p class="mb-2 text-xs text-destructive">{saveError}</p>
        {:else if currentKind === 'feed' && (specializations.length === 0 || skills.length === 0)}
          <p class="mb-2 text-xs text-muted-foreground">
            Pick at least one specialization and one skill to save your feed.
          </p>
        {/if}
        <div class="flex items-center gap-3">
          {#if index > 0}
            <button
              type="button"
              onclick={back}
              class="inline-flex items-center gap-1.5 text-sm font-medium text-muted-foreground transition-colors hover:text-foreground"
            >
              <ArrowLeft class="size-4" /> Back
            </button>
          {/if}
          <div class="flex-1"></div>
          {#if currentKind === 'feed' && matchCount !== null}
            <p class="text-right text-sm leading-tight text-muted-foreground tabular-nums" aria-live="polite">
              <span class="font-semibold text-foreground">{matchCount.toLocaleString()}</span>
              <br class="sm:hidden" /> jobs match
            </p>
          {/if}
          <Button variant="cta" size="lg" onclick={() => void advance()} disabled={saving || !loaded}>
            {#if saving}
              <LoaderCircle class="size-4 animate-spin" aria-hidden="true" /> Saving…
            {:else}
              {isLast ? 'Go to my feed' : 'Continue'} <ArrowRight class="size-4" />
            {/if}
          </Button>
        </div>
      </div>
    </div>
  </div>
{/if}
