<script lang="ts">
  // The onboarding wizard: eight steps, all skippable, each saved as it is left.
  //
  // GATING (whether to be on this route at all) lives in the root layout and reads one
  // fact: `users.onboarding_completed_at`. Null routes the account here, a timestamp never
  // does. This page assumes it is here for a reason and runs the steps that still have
  // something to ask.
  //
  // An anonymous visitor has no steps to run: the effect below bounces them to /signin
  // (carrying this page's own URL, returnTo and all, as ITS returnTo) before any wizard
  // markup renders, and /signin sends them right back once they have an account.
  //
  // WHY EACH STEP SAVES ITSELF rather than one commit at the end: the wizard now runs once
  // per account and asks eight questions. A run abandoned on the sixth used to lose all
  // five earlier answers, and there was no second chance to collect them. Each step writes
  // to the store that already owns its fact — the search profile, the screening answers,
  // the candidate-owned résumé overlay, the survey — so a failure in one cannot corrupt
  // another, and no store here exists only to serve this page.
  import { goto } from '$app/navigation';
  import { page } from '$app/state';
  import { ArrowLeft, ArrowRight, LoaderCircle, X } from '@lucide/svelte';
  import { api } from '$lib/api';
  import { completeOnboarding, isAuthenticated } from '$lib/auth.svelte';
  import { onboardingGate } from '$lib/onboardingGate.svelte';
  import { ORDERED_STEPS, plannedSteps, type OnboardingAnswered, type StepKind } from '$lib/onboardingSteps';
  import { persistStep, type SaveDeps, type WizardAnswers } from '$lib/onboardingSave';
  import { splitProfileLinks, type ProfileLinks } from '$lib/profileLinks';
  import { profileStore } from '$lib/profile.svelte';
  import { safeRedirect } from '$lib/safeRedirect';
  import { signinUrl } from '$lib/signin';
  import { focusTrap } from '$lib/actions/focusTrap';
  import type { CandidateContacts, DerivedLocation, LocationPreferences } from '$lib/types';
  import type { MergedFacets } from '$lib/onboardingImport';
  import ChallengeStep from '$lib/components/onboarding/ChallengeStep.svelte';
  import ConfirmStep from '$lib/components/onboarding/ConfirmStep.svelte';
  import CvStep from '$lib/components/onboarding/CvStep.svelte';
  import ExperienceStep from '$lib/components/onboarding/ExperienceStep.svelte';
  import LocationStep from '$lib/components/onboarding/LocationStep.svelte';
  import MoneyStep from '$lib/components/onboarding/MoneyStep.svelte';
  import SkillsStep from '$lib/components/onboarding/SkillsStep.svelte';
  import StageStep from '$lib/components/onboarding/StageStep.svelte';

  // Where finishing or skipping sends the visitor — the page they were on when they landed
  // here, not always home. Validated the same way every other returnTo in the app is
  // (safeRedirect: same-origin relative path only), since it arrives as a query param a
  // visitor's browser holds.
  const returnTo = $derived(safeRedirect(page.url.searchParams.get('returnTo')) ?? '/');

  $effect(() => {
    if (isAuthenticated()) {
      void profileStore.ensureLoaded();
      return;
    }
    // `returnTo` for /signin is THIS page's own URL, so a successful sign-in lands back
    // here. `cancelTo` is deliberately NOT this page: it is where this page's own returnTo
    // points, so /signin's close button skips both the credential form and the wizard
    // instead of bouncing back here and right back to /signin in a loop.
    const here = page.url.pathname + page.url.search;
    // eslint-disable-next-line svelte/no-navigation-without-resolve -- signinUrl() wraps resolve('/signin'); the rule can't see through the appended query
    void goto(signinUrl({ returnTo: here, cancelTo: returnTo }));
  });

  // ---- staged answers ----
  //
  // Seeded once from what the account already has, so a step that IS shown (because its
  // sibling question on the same screen is unanswered) still opens with the stored value
  // rather than blank.

  let specializations = $state.raw<string[]>([]);
  let seniorities = $state.raw<string[]>([]);
  let skills = $state.raw<string[]>([]);
  let location = $state<LocationPreferences | null>(null);
  let links = $state<ProfileLinks>({ linkedin: '', github: '', other: [] });
  let linksPrefilled = $state(false);
  // The account's whole owned résumé overlay, held because every write to it is a FULL
  // REPLACE — see onboardingSave.ts's header. Kept current from each write's response.
  let contacts = $state<CandidateContacts>({});
  let totalYears = $state<number | null>(null);
  let derivedTotalYears = $state<number | null>(null);
  let currentIncome = $state<number | null>(null);
  let desiredSalary = $state<number | null>(null);
  let moneyCurrency = $state('USD');
  let moneyPeriod = $state('month');
  let searchStage = $state<string | null>(null);
  let challenge = $state<string | null>(null);
  let challengeNote = $state('');
  let importedLocation = $state<DerivedLocation | null>(null);

  // ---- what this account has already answered ----

  let steps = $state.raw<StepKind[]>(ORDERED_STEPS);
  let loaded = $state(false);

  // One load of everything the plan depends on. A failed read is not fatal: the wizard then
  // plans every step, which asks a question or two the candidate has already answered —
  // strictly better than skipping one it has not.
  $effect(() => {
    if (!isAuthenticated() || loaded) return;
    void (async () => {
      const [resume, screening, survey] = await Promise.all([
        api.getResume().catch(() => null),
        api.getScreeningAnswers().catch(() => null),
        api.getSurvey().catch(() => null),
      ]);
      await profileStore.ensureLoaded().catch(() => {});
      const profile = profileStore.profile;

      specializations = profile ? [...profile.specializations] : [];
      seniorities = profile ? [...profile.seniorities] : [];
      skills = profile ? [...profile.skills] : [];
      location = profile?.location_preferences ?? null;

      // Contacts are the candidate's own edits; the structured extract is what the CV said.
      // Prefer the former, fall back to the latter — the same precedence the résumé screens
      // use, so the wizard never shows a stale link beside a corrected one.
      contacts = resume?.contacts ?? {};
      const storedLinks = resume?.contacts?.links ?? resume?.structured?.links ?? [];
      if (storedLinks.length > 0) {
        links = splitProfileLinks(storedLinks);
        linksPrefilled = links.linkedin !== '' || links.github !== '';
      }

      const ownedYears = resume?.contacts?.total_years;
      const ownedYearsSet = resume?.contacts?.total_years_set === true;
      if (ownedYearsSet || (ownedYears ?? 0) > 0) totalYears = ownedYears ?? 0;
      derivedTotalYears = resume?.structured?.total_years ?? null;

      if (screening?.desired_salary_amount != null) {
        desiredSalary = screening.desired_salary_amount;
        if (screening.desired_salary_currency) moneyCurrency = screening.desired_salary_currency;
        if (screening.desired_salary_period) moneyPeriod = screening.desired_salary_period;
      }
      if (survey?.current_income_amount != null) currentIncome = survey.current_income_amount;
      searchStage = survey?.job_search_stage ?? null;
      challenge = survey?.biggest_challenge ?? null;
      challengeNote = survey?.biggest_challenge_note ?? '';

      const answered: OnboardingAnswered = {
        hasResume: resume?.present === true,
        hasSpecializations: specializations.length > 0,
        hasProfileLinks: storedLinks.length > 0,
        hasTotalYears: totalYears !== null,
        hasSkills: skills.length > 0,
        hasLocation: location !== null,
        hasDesiredSalary: desiredSalary !== null,
        hasCurrentIncome: currentIncome !== null,
        hasStage: searchStage !== null,
        hasChallenge: challenge !== null,
      };
      steps = plannedSteps(answered);
      // The plan replaces a provisional list of all eight steps, so the cursor has to go
      // back to the start with it — a Skip pressed while the load was still in flight would
      // otherwise leave it pointing past the end of the real plan, at nothing.
      index = 0;
      loaded = true;

      // An account with nothing left to ask should not be shown a wizard with no steps in
      // it. Mark it done and send it on — this is the path an existing, fully-filled-in
      // account takes on its one and only routing here.
      if (steps.length === 0) void leave();
    })();
  });

  let index = $state(0);
  // Undefined only in the instant between the plan coming back empty and `leave()` taking
  // the visitor away — the markup is gated on `loaded` and the plan is never empty past
  // that point, but the type has to admit the gap rather than be asserted away.
  const currentKind = $derived<StepKind | undefined>(steps[index]);
  const isLast = $derived(index >= steps.length - 1);

  // ---- saving ----

  let saving = $state(false);
  let saveError = $state<string | null>(null);

  // What the dispatch in onboardingSave.ts is handed. Assembled here rather than there so
  // that module stays Svelte-free and its tests need no runes.
  const wizardAnswers = $derived<WizardAnswers>({
    specializations,
    skills,
    seniorities,
    excludedSkills: profileStore.profile?.excluded_skills ?? [],
    location,
    links,
    contacts,
    totalYears,
    derivedTotalYears,
    currentIncome,
    desiredSalary,
    currency: moneyCurrency,
    period: moneyPeriod,
    stage: searchStage,
    challenge,
    challengeNote,
  });

  const saveDeps: SaveDeps = {
    saveProfile: (spec, sk, sen, excl, loc) => profileStore.save(spec, sk, sen, excl, loc),
    putResumeContacts: (c) => api.putResumeContacts(c),
    updateScreeningAnswers: (p) => api.updateScreeningAnswers(p),
    updateSurvey: (p) => api.updateSurvey(p),
  };

  // True when Level and/or Location have a pick the profile save is about to drop, for the
  // reason saveProfile explains. Shown as a heads-up rather than a blocker — the
  // alternative was the picks vanishing with no explanation.
  const willLoseOrphanedPicks = $derived(
    (seniorities.length > 0 || location !== null) && (specializations.length === 0 || skills.length === 0),
  );

  /** Save the current step and move on. On the last one, finish. */
  async function advance() {
    if (saving || !loaded || currentKind === undefined) return;
    const kind = currentKind;
    saving = true;
    saveError = null;
    try {
      // The response IS the new stored overlay, so keeping it is what stops a later contacts
      // write spreading something three screens stale — see onboardingSave.ts's header.
      contacts = await persistStep(kind, wizardAnswers, saveDeps);
    } catch {
      saveError = "Couldn't save that just now. Try again, or skip this step.";
      saving = false;
      return;
    }
    // A CV uploaded on the first step is parsed in the background, so what it yields is not
    // readable until after the step is left. Re-reading here is what lets the experience and
    // links steps open pre-filled for the account this whole wizard exists for — a brand new
    // one, which had no résumé at all when the page first loaded.
    if (kind === 'cv') await refreshFromResume();
    saving = false;
    if (isLast) await leave();
    else index += 1;
  }

  /** Move on WITHOUT saving. Skipping is a choice not to answer, so it must not persist a
   *  slider's resting position as though the candidate had put it there.
   *
   *  Guarded on `saving` for the same reason Continue is: without it, clicking Continue and
   *  then Skip advances the index twice — once here and once when the save resolves — and
   *  the step in between is never shown, never asked, never saved. */
  async function skip() {
    if (saving || !loaded) return;
    if (isLast) await leave();
    else index += 1;
  }

  /** Re-read the résumé and take anything it now offers that the candidate has not answered
   *  themselves. Best-effort and non-destructive: a value the candidate set always wins, and
   *  a parse still in flight simply yields nothing rather than blocking the wizard. */
  async function refreshFromResume() {
    const resume = await api.getResume().catch(() => null);
    if (!resume) return;
    contacts = resume.contacts ?? contacts;
    derivedTotalYears = resume.structured?.total_years ?? derivedTotalYears;
    const storedLinks = resume.contacts?.links ?? resume.structured?.links ?? [];
    if (storedLinks.length === 0) return;
    const found = splitProfileLinks(storedLinks);
    // Fill only the boxes the candidate has left empty, and keep every link we did not
    // recognise so a round trip cannot lose one.
    links = {
      linkedin: links.linkedin || found.linkedin,
      github: links.github || found.github,
      other: [...new Set([...links.other, ...found.other])],
    };
    linksPrefilled = linksPrefilled || links.linkedin !== '' || links.github !== '';
  }

  function back() {
    if (index > 0) index -= 1;
  }

  /** End the wizard for good: mark the account complete and go where the visitor was
   *  headed. Called on finishing the last step and on an explicit close — both are the
   *  candidate deciding they are done here.
   *
   *  Merely navigating away calls none of this: that account is asked again on a later
   *  visit, and `onboardingGate.dismiss()` is what stops the layout re-capturing them
   *  within this one. Best-effort on the marker — a failed write must not trap someone on
   *  this page; the worst case is being asked once more. */
  async function leave() {
    onboardingGate.dismiss();
    try {
      await completeOnboarding();
    } catch {
      // best-effort — see doc comment above.
    }
    // eslint-disable-next-line svelte/no-navigation-without-resolve -- returnTo is a validated same-origin path (safeRedirect), not a typed route
    void goto(returnTo);
  }

  function onExtracted(merged: MergedFacets) {
    specializations = merged.specializations;
    seniorities = merged.seniorities;
    skills = merged.skills;
  }

  function onLinkedInUrl(url: string) {
    if (links.linkedin !== '') return; // never overwrite one the candidate already has
    links = { ...links, linkedin: url };
    linksPrefilled = true;
  }

  /** Escape leaves the wizard for THIS VISIT only — it does not mark onboarding complete.
   *
   *  Deliberately weaker than the close button beside it. Escape is not always aimed at the
   *  dialog: the skills typeahead and the specialization search both use it to dismiss their
   *  own popups and do not stop it propagating, so a candidate closing a dropdown reaches
   *  this handler. While completion was a per-visit flag that misfire cost them nothing —
   *  they were asked again next time. Now that it is a permanent account fact, treating a
   *  stray keypress as "I am done with onboarding forever" would end the wizard for good for
   *  someone who never meant to leave it. */
  function onKeydown(e: KeyboardEvent) {
    if (e.key !== 'Escape') return;
    onboardingGate.dismiss();
    // eslint-disable-next-line svelte/no-navigation-without-resolve -- returnTo is a validated same-origin path (safeRedirect), not a typed route
    void goto(returnTo);
  }
</script>

<svelte:window onkeydown={onKeydown} />

{#if !isAuthenticated()}
  <!-- Redirecting to /signin — see the effect above. Nothing to render meanwhile, since the
       wizard below assumes an account already exists. -->
{:else}
  <div class="fixed inset-0 z-50 flex flex-col bg-background">
    <div
      class="flex h-full w-full flex-col overflow-hidden"
      role="dialog"
      aria-modal="true"
      aria-label="Finish setting up your account"
      {@attach focusTrap()}
    >
      <!-- header: step dots + skip + close -->
      <div class="flex items-center gap-3 border-b border-border px-5 py-3">
        <div class="flex flex-1 items-center gap-1.5" aria-hidden="true">
          {#each steps as _, i (i)}
            <span class={['h-1 w-7 rounded-full transition-colors', i <= index ? 'bg-brand' : 'bg-border']}></span>
          {/each}
        </div>
        <button
          type="button"
          onclick={() => void skip()}
          disabled={saving || !loaded}
          class="text-sm font-medium text-muted-foreground transition-colors hover:text-foreground disabled:opacity-60"
        >
          Skip
        </button>
        <button
          type="button"
          onclick={() => void leave()}
          aria-label="Close"
          class="flex size-8 items-center justify-center rounded-lg border border-border text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
        >
          <X class="size-4" />
        </button>
      </div>

      <!-- body -->
      <div class="no-scrollbar mx-auto min-h-0 w-full max-w-lg flex-1 overflow-y-auto px-5 py-6">
        {#if !loaded}
          <div class="flex items-center gap-2 py-10 text-sm text-muted-foreground">
            <LoaderCircle class="size-4 animate-spin" aria-hidden="true" /> Getting your details…
          </div>
        {:else}
          <div class="mb-1 inline-flex items-center gap-1.5 text-xs font-semibold uppercase tracking-wide text-brand-strong">
            Step {index + 1} of {steps.length}
          </div>
          {#if currentKind === 'cv'}
            <CvStep
              staged={{ specializations, seniorities, skills }}
              {onExtracted}
              onDerivedLocation={(loc) => (importedLocation = loc)}
              {onLinkedInUrl}
            />
          {:else if currentKind === 'confirm'}
            <ConfirmStep
              {specializations}
              {seniorities}
              {links}
              {linksPrefilled}
              onSpecializationsChange={(next) => (specializations = next)}
              onSenioritiesChange={(next) => (seniorities = next)}
              onLinksChange={(next) => (links = next)}
            />
          {:else if currentKind === 'experience'}
            <ExperienceStep
              value={totalYears}
              fromCv={derivedTotalYears}
              onChange={(years) => (totalYears = years)}
            />
          {:else if currentKind === 'skills'}
            <SkillsStep {skills} onChange={(next) => (skills = next)} />
          {:else if currentKind === 'location'}
            <LocationStep
              value={location}
              derivedLocation={importedLocation ?? profileStore.profile?.derived_location}
              onChange={(next) => (location = next)}
            />
          {:else if currentKind === 'money'}
            <MoneyStep
              {currentIncome}
              {desiredSalary}
              currency={moneyCurrency}
              period={moneyPeriod}
              onCurrentIncomeChange={(amount) => (currentIncome = amount)}
              onDesiredSalaryChange={(amount) => (desiredSalary = amount)}
              onCurrencyChange={(c) => (moneyCurrency = c)}
              onPeriodChange={(p) => (moneyPeriod = p)}
            />
          {:else if currentKind === 'stage'}
            <StageStep value={searchStage} onChange={(v) => (searchStage = v)} />
          {:else if currentKind === 'challenge'}
            <ChallengeStep
              value={challenge}
              note={challengeNote}
              onChange={(v) => (challenge = v)}
              onNoteChange={(n) => (challengeNote = n)}
            />
          {/if}
        {/if}
      </div>

      <!-- footer: back / continue|finish -->
      <div class="mx-auto w-full max-w-lg border-t border-border px-5 py-3">
        {#if saveError}
          <p class="mb-2 text-xs text-destructive">{saveError}</p>
        {:else if willLoseOrphanedPicks && (currentKind === 'skills' || currentKind === 'location')}
          <p class="mb-2 text-xs text-muted-foreground">
            Add a specialization and a skill (on the Confirm step) to save your level/location picks — otherwise they won't be kept.
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
          <button
            type="button"
            onclick={() => void advance()}
            disabled={saving || !loaded}
            class="inline-flex h-11 items-center gap-1.5 rounded-lg bg-brand px-6 text-sm font-semibold text-brand-foreground transition-opacity hover:opacity-90 disabled:opacity-60"
          >
            {#if saving}
              <LoaderCircle class="size-4 animate-spin" aria-hidden="true" /> Saving…
            {:else}
              {isLast ? 'Finish' : 'Continue'} <ArrowRight class="size-4" />
            {/if}
          </button>
        </div>
      </div>
    </div>
  </div>
{/if}
