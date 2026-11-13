<script lang="ts">
  // The CV-first onboarding flow, open to anybody: drop a CV, see what was read from it,
  // answer three one-tap questions, and get the matching open jobs and the CV's ATS score —
  // all before any account exists. Signing up from the results carries the CV, the search
  // profile, an email alert and the "in your way" answers into the new account (openspec
  // cv-first-onboarding).
  //
  // One card, one step revealed under another as the flow advances, so the visitor watches
  // their own search being built. The phase machine and every mapping are pure and live in
  // #lib/onboardingFlow; carrying into the account is #lib/onboardingCarry; the browser-side
  // draft that survives an OAuth round trip is #lib/onboardingDraft.
  //
  // GATING lives in the root layout: an account that has not been through here is routed
  // here once (carrying `returnTo`, where "Skip for now" and the finished flow go), and
  // this page marks it complete the moment it is shown, so leaving on the first step never
  // brings it back. Nothing routes anybody away from here.
  //
  // A DRAFT REACHES AN ACCOUNT ONLY WHEN THIS BROWSER STARTED A SIGN-UP FROM IT. In a shared
  // browser the draft may be somebody else's anonymous run; see `arrivalAction`.
  import { onDestroy, untrack } from 'svelte';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { ArrowRight, FileUp, MessageCircleQuestionMark, RotateCcw, Search, UserRound } from '@lucide/svelte';
  import { api, ApiError, RESUME_MAX_MB } from '#lib/api.js';
  import { currentUser, isAuthenticated } from '#lib/auth.svelte.js';
  import { filtersFromProfile, filtersToParams } from '#lib/facetModel.js';
  import { countryLabel } from '#lib/facets.js';
  import { gotoPath } from '#lib/gotoPath.js';
  import { carryFailureMessage, carryInto, type CarryDeps, type CarryStep } from '#lib/onboardingCarry.js';
  import { arrivalAction, onboardingDraft } from '#lib/onboardingDraft.js';
  import {
    answerChips,
    companiesLine,
    asProfile,
    currentQuestion,
    FIRST_WINDOW_DAYS,
    matchQuery,
    matchSkills,
    nextPhase,
    profileFromAnswers,
    roleFacetQuery,
    widenedWindow,
    type Answers,
    type FlowEvent,
    type Phase,
    type QuestionKey,
  } from '#lib/onboardingFlow.js';
  import { onboardingGate } from '#lib/onboardingGate.svelte.js';
  import { profileStore } from '#lib/profile.svelte.js';
  import { enableProfileAlert } from '#lib/profileAlert.js';
  import { syncProfileAlert } from '#lib/profileAlertSync.js';
  import { resumeStore } from '#lib/resume.svelte.js';
  import { safeRedirect } from '#lib/safeRedirect.js';
  import { savedSearches } from '#lib/savedSearches.svelte.js';
  import { signinUrl } from '#lib/signin.js';
  import type { CatalogScale, OnboardingMatch, OnboardingParse } from '#lib/types.js';
  import { Button, Card, Chip } from '#lib/ui/index.js';
  import FlowStep from '#lib/components/onboarding/FlowStep.svelte';
  import OnboardingQuestions from '#lib/components/onboarding/OnboardingQuestions.svelte';
  import OnboardingResults from '#lib/components/onboarding/OnboardingResults.svelte';
  import CompanyLogoStack from '#lib/components/CompanyLogoStack.svelte';

  let phase = $state<Phase>('drop');
  function step(event: FlowEvent) {
    phase = nextPhase(phase, event);
  }

  // ---- what the flow holds ----

  let file = $state.raw<Blob | null>(null);
  let fileName = $state('cv.pdf');
  let parse = $state.raw<OnboardingParse | null>(null);
  let answers = $state.raw<Answers>({});
  // An answered question the visitor tapped to change; otherwise the next unanswered one.
  let reopened = $state<QuestionKey | null>(null);
  const question = $derived(reopened ?? currentQuestion(answers));

  /** Keep the draft for the hop to the sign-in page, so signing up — through an OAuth
   *  provider too — carries it. Written only then; the page deletes it on reading it back. */
  function persist(pendingSignupAt: number): Promise<void> {
    if (isAuthenticated() || !file || !parse) return Promise.resolve();
    // IndexedDB stores by structured clone, which refuses a Svelte proxy outright — and the
    // draft store swallows the failure, losing the save (and its sign-up mark) silently.
    const plainAnswers = $state.snapshot(answers);
    return onboardingDraft.save({ file, fileName, parse, answers: plainAnswers, savedAt: Date.now(), pendingSignupAt });
  }

  /** Where a signed-in visitor was headed when the layout sent them here. */
  const returnTo = $derived(safeRedirect(page.url.searchParams.get('returnTo')));
  /** This page as the sign-in page should bring the visitor back to it — still carrying
   *  where they were headed, so saving can end there. */
  const here = $derived(
    returnTo ? `${resolve('/onboarding')}?${new URLSearchParams({ returnTo })}` : resolve('/onboarding'),
  );

  // ---- arrival ----

  // Once, on the first client render. Untracked: the reads below must not re-run arrival
  // when the session or a store changes later (a sign-up mid-flow carries by itself).
  let arrived = false;
  $effect(() => {
    if (arrived) return;
    arrived = true;
    untrack(() => void arrive());
  });

  async function arrive() {
    const authenticated = isAuthenticated();
    // Read before marking: the gate's mark is written on arrival.
    const onboardedBefore = currentUser()?.onboarding_completed_at != null;
    if (authenticated && !onboardedBefore) {
      // Shown means done. The raw call, not auth's invalidating one: re-reading the session
      // here buys nothing, and the gate's dismissal covers the rest of this visit.
      onboardingGate.dismiss();
      void api.completeOnboarding().catch(() => {});
    }
    const draft = await onboardingDraft.load();
    if (!draft) return;
    // Single use: read back once and gone, whatever happens next. A sign-up left on the
    // sign-in page must not leave a marked draft for the next person at this browser.
    await onboardingDraft.clear();
    const created = Date.parse(currentUser()?.created_at ?? '');
    const accountCreatedAt = Number.isNaN(created) ? null : created;
    const action = arrivalAction(draft, { authenticated, accountCreatedAt, now: Date.now() });
    if (action === 'discard' || phase !== 'drop') return;
    file = draft.file;
    fileName = draft.fileName;
    parse = draft.parse;
    answers = draft.answers;
    loadCatalog();
    if (action === 'carry') {
      // Back from a sign-up this browser started, into the account it created.
      phase = 'carrying';
      void carry();
      return;
    }
    phase = 'questions';
    // Everything answered: show the results again — with the explicit save for an account
    // that existed before ('offer-save').
    if (currentQuestion(answers) === null) void search();
  }

  // ---- the CV ----

  let fileInput = $state<HTMLInputElement>();
  let dragActive = $state(false);
  let dropError = $state<string | null>(null);
  let limited = $state(false);
  let readGen = 0;

  const isPdf = (f: File) => f.type === 'application/pdf' || f.name.toLowerCase().endsWith('.pdf');

  async function read(f: File) {
    if (!isPdf(f)) {
      dropError = 'Please choose a PDF file.';
      return;
    }
    const gen = ++readGen;
    dropError = null;
    limited = false;
    file = f;
    fileName = f.name;
    step('file');
    try {
      const result = await api.onboardingParseCv(f);
      if (gen !== readGen) return;
      parse = result;
      answers = {};
      reopened = null;
      step('parsed');
      loadCatalog();
    } catch (e) {
      if (gen !== readGen) return;
      file = null;
      limited = e instanceof ApiError && e.status === 429;
      dropError =
        e instanceof ApiError && (e.status === 429 || e.status === 413)
          ? e.message
          : "We couldn't read that file. Try a PDF with selectable text.";
      step('failed');
    }
  }

  function onPick(e: Event) {
    const input = e.currentTarget as HTMLInputElement;
    const f = input.files?.[0];
    input.value = ''; // the same file can be picked again after an error
    if (f) void read(f);
  }

  function onDrop(e: DragEvent) {
    e.preventDefault();
    dragActive = false;
    const f = e.dataTransfer?.files?.[0];
    if (f && phase === 'drop') void read(f);
  }

  const cvTitle = $derived(parse?.first_name ? `Read ${parse.first_name}'s CV` : 'Read your CV');
  const cvLine = $derived.by(() => {
    if (!parse) return undefined;
    if (!parse.structured_available) return 'We read your skills and level; the rest of the CV did not come through.';
    return [parse.headline, parse.country ? countryLabel(parse.country) : ''].filter(Boolean).join(' · ') || undefined;
  });

  // ---- the questions ----

  function answer(patch: Partial<Answers>) {
    answers = { ...answers, ...patch };
    reopened = null;
    if (currentQuestion(answers) === null) void search();
  }

  function reopen(key: QuestionKey) {
    reopened = key;
    if (phase === 'results') step('edit');
  }

  // The role question offers what the catalogue holds for the CV's skills under the
  // answers given so far, so it is counted when it comes up. A failed count leaves the
  // CV's own roles, which need no catalogue.
  let roleCounts = $state.raw<Record<string, number> | undefined>(undefined);
  let roleGen = 0;
  $effect(() => {
    if (question !== 'role' || !parse) return;
    const gen = ++roleGen;
    const params = roleFacetQuery(parse, answers);
    untrack(() => {
      roleCounts = undefined;
      api
        .facetCounts(params, { facets: ['category'] })
        .then((res) => {
          if (gen === roleGen) roleCounts = res.facets.category ?? {};
        })
        .catch(() => {});
    });
  });

  // ---- the search ----

  // The published catalogue snapshot, read once: the searching step names what it
  // searches. Figures that are not measured are left out, never shown as zero.
  let catalog: Promise<unknown> | null = null;
  let scale = $state.raw<CatalogScale | null>(null);
  let scaleRead = $state(false);
  function loadCatalog() {
    catalog ??= api
      .catalogScale()
      .then((s) => {
        scale = s;
      })
      .catch(() => {})
      .finally(() => (scaleRead = true));
  }
  const catalogLine = $derived.by(() => {
    if (!scale?.exact || !(scale.unique_open_jobs > 0)) return 'Searching every open job we hold';
    const parts = [`Searching ${scale.unique_open_jobs.toLocaleString('en')} open jobs`];
    if (scale.sources > 0) parts.push(`from ${scale.sources.toLocaleString('en')} sources`);
    if (scale.ats_platforms > 0) parts.push(`across ${scale.ats_platforms.toLocaleString('en')} ATS platforms`);
    return parts.join(' ');
  });

  let matches = $state.raw<OnboardingMatch[]>([]);
  let total = $state(0);
  let days = $state(FIRST_WINDOW_DAYS);
  let widened = $state(false);
  let searching = $state(false);
  let searchError = $state<string | null>(null);
  // A carry straight after an OAuth return ran no search, so it has no results to show.
  let searched = $state(false);
  let searchGen = 0;

  async function search() {
    if (!parse) return;
    const gen = ++searchGen;
    step('answered');
    searching = true;
    searchError = null;
    widened = false;
    days = FIRST_WINDOW_DAYS;
    loadCatalog();
    const skills = matchSkills(parse);
    try {
      let res = await api.onboardingMatches(skills, matchQuery(parse, answers, days));
      const wider = widenedWindow(days, res.meta.total);
      if (wider !== null && gen === searchGen) {
        widened = true;
        days = wider;
        res = await api.onboardingMatches(skills, matchQuery(parse, answers, days));
      }
      if (gen !== searchGen) return;
      matches = res.data;
      total = res.meta.total;
      searched = true;
      step('found');
    } catch {
      if (gen !== searchGen) return;
      searchError = "The search didn't go through. Change an answer or try again.";
      step('failed');
    } finally {
      if (gen === searchGen) searching = false;
    }
  }

  // ---- into the account ----

  let carryFailed = $state.raw<CarryStep[]>([]);

  const carryDeps: CarryDeps = {
    async uploadCv(f) {
      // The plain store, which fails when the file was not stored (a 501 when storage is
      // off) — unlike the extract route, which would answer 200 either way.
      await api.putResume(f);
      resumeStore.noteUpload();
    },
    async saveProfile(d) {
      // Whatever the account already excludes stays excluded.
      await profileStore.ensureLoaded();
      const current = profileStore.profile;
      const saved = await profileStore.save(
        d.specializations,
        d.skills,
        d.seniorities,
        current?.excluded_skills ?? [],
        current?.excluded_sources ?? [],
        current?.excluded_companies ?? [],
        d.location,
      );
      await syncProfileAlert();
      return saved;
    },
    async enableAlert(profile) {
      // The existing profile search, if any, must be known before one is created.
      await savedSearches.ensureLoaded();
      await enableProfileAlert(profile, 'email');
    },
    completeOnboarding: () => api.completeOnboarding(),
    saveSurvey: (patch) => api.updateSurvey(patch),
  };

  async function carry(steps?: CarryStep[]) {
    if (!file || !parse) return;
    step('carry');
    // Leaving now (the header is still there) must not land back here.
    onboardingGate.dismiss();
    carryFailed = [];
    const failed = await carryInto({ file, fileName, parse, answers }, carryDeps, steps);
    // Every step has been tried: the draft has done its job. What failed stays in memory
    // for "Try again"; a later visit must not run the whole carry a second time.
    await onboardingDraft.clear();
    carryFailed = failed;
    // Saving outlives the page: a visitor who navigated away meanwhile stays where they went.
    if (failed.length === 0 && !left) await goToFeed();
  }

  /** Where the visitor was headed when the layout sent them here, else the account's feed:
   *  the jobs search under the saved profile's own filters. */
  async function goToFeed() {
    if (returnTo) return gotoPath(returnTo);
    const profile = profileStore.profile ?? (parse ? asProfile(profileFromAnswers(parse, answers)) : null);
    const query = profile ? filtersToParams(filtersFromProfile(profile)).toString() : '';
    await gotoPath(`${resolve('/jobs')}${query ? `?${query}` : ''}`);
  }

  /** A signed-in visitor who has nothing to drop goes where they were headed. */
  function skip() {
    onboardingGate.dismiss();
    void gotoPath(returnTo ?? resolve('/'));
  }

  let left = false;
  onDestroy(() => (left = true));

  /** Sign-up from the results goes through the sign-in page and comes back here. The draft
   *  is marked first: only a marked draft is carried on the way back, so a browser that
   *  never started a sign-up from here never carries one (see `arrivalAction`). */
  async function signUp() {
    await persist(Date.now());
    await gotoPath(signinUrl({ returnTo: here }));
  }

  async function startOver() {
    ++readGen;
    ++searchGen;
    await onboardingDraft.clear();
    file = null;
    parse = null;
    answers = {};
    reopened = null;
    matches = [];
    searched = false;
    carryFailed = [];
    dropError = null;
    limited = false;
    step('restart');
  }

  const showQuestions = $derived(parse !== null && phase !== 'drop' && phase !== 'parsing');
  const showSearch = $derived(phase === 'searching' || phase === 'results' || phase === 'carrying');
  const showResults = $derived((phase === 'results' || phase === 'carrying') && parse !== null && searched);
  const skillCount = $derived(parse ? matchSkills(parse).length : 0);
  const chips = $derived(parse ? answerChips(parse, answers) : []);
</script>

{#snippet account()}
  {#if phase === 'carrying' && carryFailed.length > 0}
    <div class="flex w-full flex-col gap-3 rounded-xl border border-border bg-background p-4" role="alert">
      <p class="text-sm">{carryFailureMessage(carryFailed)}</p>
      <div class="flex flex-wrap justify-end gap-2">
        <Button variant="primary" onclick={() => void carry(carryFailed)}>Try again</Button>
        <Button variant="outline" onclick={() => void goToFeed()}>Go to my feed</Button>
      </div>
    </div>
  {:else if phase === 'carrying'}
    <p class="text-sm text-muted-foreground" aria-live="polite">Saving your CV, profile and alert…</p>
  {:else if isAuthenticated()}
    <Button variant="cta" class="gap-1.5" onclick={() => void carry()}>
      Save and go to my feed
      <ArrowRight class="size-4" aria-hidden="true" />
    </Button>
  {:else}
    <Button variant="cta" class="gap-1.5" onclick={() => void signUp()}>
      Save my matches
      <ArrowRight class="size-4" aria-hidden="true" />
    </Button>
  {/if}
{/snippet}

<svelte:head>
  <title>Find jobs that fit your CV · freehire</title>
</svelte:head>

<div class="mx-auto flex w-full max-w-3xl flex-col gap-6 px-4 py-10 sm:py-14">
  <header class="flex flex-col gap-2 text-center">
    <h1 class="text-2xl font-semibold tracking-tight text-balance sm:text-3xl">Find the jobs your CV fits</h1>
    <p class="text-sm text-muted-foreground text-balance sm:text-base">
      Drop your CV, answer three quick questions, and see matching open jobs. No account needed.
    </p>
  </header>

  <Card class="p-5 sm:p-6">
    {#if phase === 'drop'}
      <FlowStep
        icon={FileUp}
        title="Start with your CV"
        subtitle="We read it and show you what we found. Nothing is stored until you save it."
      >
        <input type="file" accept="application/pdf,.pdf" class="hidden" bind:this={fileInput} onchange={onPick} />
        <!-- A drop target over the button inside, which is fully keyboard-accessible alone. -->
        <!-- svelte-ignore a11y_no_static_element_interactions -->
        <div
          data-testid="onboarding-dropzone"
          ondragover={(e) => {
            e.preventDefault();
            dragActive = true;
          }}
          ondragleave={(e) => {
            e.preventDefault();
            dragActive = false;
          }}
          ondrop={onDrop}
          class={[
            'flex flex-col items-center justify-center gap-3 rounded-xl border-2 border-dashed px-6 py-10 text-center transition-colors',
            dragActive ? 'border-brand bg-brand-muted' : 'border-border',
          ]}
        >
          <p class="text-sm font-medium">Drop your CV here, or</p>
          <Button variant="primary" onclick={() => fileInput?.click()}>Choose a PDF</Button>
          <p class="text-xs text-muted-foreground">PDF with selectable text, up to {RESUME_MAX_MB} MB.</p>
        </div>
        {#if isAuthenticated()}
          <button
            type="button"
            onclick={skip}
            class="self-start text-sm text-muted-foreground underline-offset-4 hover:text-foreground hover:underline"
          >
            Skip for now
          </button>
        {:else}
          <!-- Someone who already has an account came to sign in, not to be onboarded: their
               profile and CV are already stored, so the flow has nothing to add for them. -->
          <p class="text-sm text-muted-foreground">
            Already have an account?
            <!-- eslint-disable svelte/no-navigation-without-resolve -- signinUrl() wraps resolve('/signin'); the rule can't see through the appended query -->
            <a
              href={signinUrl({ returnTo: returnTo ?? resolve('/'), mode: 'login' })}
              class="font-medium text-foreground underline-offset-4 hover:underline">Sign in</a
            ><!-- eslint-enable svelte/no-navigation-without-resolve -->
          </p>
        {/if}
        {#if dropError}
          <div class="flex flex-col gap-2 text-sm" role="alert">
            <p class="text-destructive">{dropError}</p>
            {#if limited && !isAuthenticated()}
              <p class="text-muted-foreground">Signed in, there is no daily limit — sign in and drop it again.</p>
              <Button variant="outline" size="sm" class="self-start" href={signinUrl({ returnTo: here })}>
                Sign in or create an account
              </Button>
            {/if}
          </div>
        {/if}
      </FlowStep>
    {:else}
      <FlowStep icon={UserRound} title={cvTitle} subtitle={cvLine} state={parse ? 'done' : 'working'}>
        {#if !parse}
          <p class="text-sm text-muted-foreground">Reading {fileName}… this takes up to half a minute.</p>
        {:else}
          <!-- The work history on one line (who, and for how long), the skills apart below. -->
          {#if parse.companies.length > 0 || parse.total_years > 0}
            <div class="flex flex-wrap items-center gap-x-2.5 gap-y-1 text-sm text-muted-foreground">
              {#if parse.companies.length > 0}
                <CompanyLogoStack names={parse.companies} />
                <span>{companiesLine(parse.companies)}</span>
              {/if}
              {#if parse.total_years > 0}
                {#if parse.companies.length > 0}<span aria-hidden="true">·</span>{/if}
                <span class="font-medium text-foreground"
                  >{parse.total_years} {parse.total_years === 1 ? 'year' : 'years'} of experience</span
                >
              {/if}
            </div>
          {/if}
          {#if parse.skills.length > 0}
            <div class="flex flex-wrap gap-1.5" aria-label="Skills">
              {#each parse.skills.slice(0, 8) as skill (skill)}
                <Chip>{skill}</Chip>
              {/each}
            </div>
          {/if}
        {/if}
      </FlowStep>

      {#if showQuestions && parse}
        {#if phase === 'questions' && question}
          <FlowStep
            icon={MessageCircleQuestionMark}
            title="What are you looking for?"
            subtitle="Tap an answer, then we search."
          >
            <OnboardingQuestions {parse} {answers} current={question} {roleCounts} onAnswer={answer} onOpen={reopen} />
          </FlowStep>
        {:else}
          <FlowStep
            icon={MessageCircleQuestionMark}
            title="What you're looking for"
            subtitle={chips.join(' · ') || 'No preferences — the whole catalogue.'}
            state="done"
          >
            {#if searchError && phase === 'questions'}
              <p class="text-sm text-destructive" role="alert">{searchError}</p>
              <div class="flex flex-wrap justify-end gap-2">
                <Button variant="outline" size="sm" onclick={() => void search()}>Try again</Button>
                <Button variant="ghost" size="sm" onclick={() => reopen('where')}>Change answers</Button>
              </div>
            {/if}
          </FlowStep>
        {/if}
      {/if}

      {#if showSearch && parse}
        <FlowStep icon={Search} title="Searching" state={searching ? 'working' : 'done'}>
          <ul class="flex flex-col gap-1.5 text-sm" aria-live="polite">
            <li class={scaleRead ? 'text-foreground' : 'text-muted-foreground'}>{catalogLine}</li>
            <li class={searching && !widened ? 'text-muted-foreground' : 'text-foreground'}>
              {#if skillCount > 0}
                Matching your {skillCount} skills against jobs posted in the last {FIRST_WINDOW_DAYS} days
              {:else}
                Matching jobs to your answers, posted in the last {FIRST_WINDOW_DAYS} days
              {/if}
            </li>
            {#if widened}
              <li class={searching ? 'text-muted-foreground' : 'text-foreground'}>
                Nothing there yet — widening to the last {days} days
              </li>
            {/if}
          </ul>
        </FlowStep>
      {/if}

      {#if showResults && parse}
        <OnboardingResults
          {matches}
          {total}
          {days}
          {widened}
          ranked={skillCount > 0}
          {chips}
          report={parse.report}
          query={matchQuery(parse, answers, days)}
          onEditAnswers={phase === 'carrying' ? undefined : () => reopen('where')}
        />
      {:else if phase === 'carrying'}
        <FlowStep icon={UserRound} title="Saving to your account" state={carryFailed.length ? 'active' : 'working'}>
          {@render account()}
        </FlowStep>
      {/if}

      <!-- One closing row under one rule: the actions on the right, starting over beside saving. -->
      {#if phase !== 'carrying' || showResults}
        <div class="flex flex-wrap items-center justify-end gap-3 border-t border-dashed border-border pt-4">
          {#if showResults && phase === 'results'}
            <p class="mr-auto text-xs text-muted-foreground text-pretty">
              {isAuthenticated()
                ? 'Saving replaces your stored CV and search profile.'
                : 'Free — your CV and answers come with you.'}
            </p>
          {/if}
          {#if phase !== 'carrying'}
            <Button variant="ghost" size="sm" onclick={() => void startOver()}>
              <RotateCcw class="size-3.5" aria-hidden="true" /> Start over
            </Button>
          {/if}
          {#if showResults}
            {@render account()}
          {/if}
        </div>
      {/if}
    {/if}
  </Card>
</div>
