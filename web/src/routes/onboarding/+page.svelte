<script lang="ts">
  // The onboarding page: the post-registration CV/profile wizard. Reached either by
  // an explicit "Sign up" link (carrying ?returnTo=) or, for a signed-in user with no
  // CV yet, the root layout's redirect effect (reappearing on a later visit if the
  // account still has no CV — no separate "completed" flag). GATING (whether to be on
  // this route at all) lives in the layout; this page assumes it's here for a reason
  // and just runs the steps that apply.
  //
  // An anonymous visitor has no steps to run here at all: the effect below bounces
  // them to /signin (carrying this page's own URL, returnTo and all, as ITS returnTo)
  // before any wizard markup renders, and /signin sends them right back once they have
  // an account. Distinct from the anonymous /jobs feed-preference capture this
  // replaced: this one writes to the server profile (PUT /api/v1/me/profile), not a
  // local filter query, and requires an account to do anything here.
  import { goto } from '$app/navigation';
  import { page } from '$app/state';
  import { ArrowLeft, ArrowRight, ChevronDown, FileUp, LoaderCircle, Search, X } from '@lucide/svelte';
  import { SvelteSet } from 'svelte/reactivity';
  import { api, ApiError, RESUME_MAX_MB } from '$lib/api';
  import { cvUploadReason, track } from '$lib/analytics';
  import { isAuthenticated } from '$lib/auth.svelte';
  import { FACETS, type FacetOption } from '$lib/facets';
  import { CATEGORY_GROUPS } from '$lib/filterSections';
  import { loadSkillDistribution } from '$lib/skillDictionary';
  import { onboardingGate } from '$lib/onboardingGate.svelte';
  import { profileStore } from '$lib/profile.svelte';
  import { resumeStore } from '$lib/resume.svelte';
  import { safeRedirect } from '$lib/safeRedirect';
  import type { DerivedLocation, LocationPreferences } from '$lib/types';
  import { mergeFacets } from '$lib/onboardingImport';
  import { focusTrap } from '$lib/actions/focusTrap';
  import { pillClass, pillTitle } from '$lib/components/facets/pill';
  import { signinUrl } from '$lib/signin';
  import RemoteSearchSelect from '$lib/components/facets/RemoteSearchSelect.svelte';
  import LocationPreferencesFields from '$lib/components/profile/LocationPreferencesFields.svelte';

  const seniorityOptions = FACETS.find((f) => f.param === 'seniority')?.options ?? [];

  // Where finishing or skipping the rest of the wizard sends the visitor — the page
  // they were on when they landed here, not always home. Validated the same way
  // every other returnTo in the app is (safeRedirect: same-origin relative path
  // only), since it arrives as a query param a visitor's browser holds.
  const returnTo = $derived(safeRedirect(page.url.searchParams.get('returnTo')) ?? '/');

  type StepKind = 'cv' | 'confirm' | 'skills' | 'location';
  const STEP_KINDS: StepKind[] = ['cv', 'confirm', 'skills', 'location'];
  const TOTAL_STEPS = STEP_KINDS.length;
  let step = $state(1);
  const currentKind = $derived(STEP_KINDS[Math.min(step, TOTAL_STEPS) - 1]);

  $effect(() => {
    if (isAuthenticated()) {
      void profileStore.ensureLoaded();
      return;
    }
    // `returnTo` for /signin is THIS page's own URL — so a successful sign-in/register
    // lands back here and the wizard picks up from the CV step. `cancelTo` is separate
    // and deliberately NOT this page's URL: it's the same place this page's own
    // `returnTo` points to (the visitor's original destination), so /signin's close
    // button skips both the credential form AND the rest of the wizard, instead of
    // bouncing back here and right back to /signin in a loop.
    const here = page.url.pathname + page.url.search;
    // eslint-disable-next-line svelte/no-navigation-without-resolve -- signinUrl() wraps resolve('/signin'); the rule can't see through the appended query
    void goto(signinUrl({ returnTo: here, cancelTo: returnTo }));
  });

  // Staged locally until commit (see finish()) — specializations/skills/seniorities/
  // location. Seeded once from the saved profile (if any), so a returning user who filled
  // these in on a prior visit but skipped the CV step does not lose them. Waits for
  // profileStore.loaded (re-running, since both are reactively read, whenever either
  // changes) rather than seeding on mount unconditionally — the load may still be in
  // flight the instant this page renders, or may not even have started yet (a visitor
  // arriving signed out never gets this far — see the redirect effect above).
  let specializations = $state.raw<string[]>([]);
  let skills = $state.raw<string[]>([]);
  let seniorities = $state.raw<string[]>([]);
  let location = $state<LocationPreferences | null>(null);

  let seeded = $state(false);
  $effect(() => {
    if (seeded || !profileStore.loaded) return;
    const p = profileStore.profile;
    specializations = p ? [...p.specializations] : [];
    skills = p ? [...p.skills] : [];
    seniorities = p ? [...p.seniorities] : [];
    location = p?.location_preferences ?? null;
    seeded = true;
  });

  // ---- Step "cv": CV upload ----

  let cvState = $state<'idle' | 'parsing' | 'error'>('idle');
  let cvError = $state<string | null>(null);
  let cvNote = $state<string | null>(null);
  let cvInput = $state<HTMLInputElement>();
  let cvGen = 0;

  // Defense in depth: a session expiring mid-visit can flip isAuthenticated() back to
  // false while this step is still on screen — extractResumeProfile would just 401.
  // The top-level `{#if !isAuthenticated()}` guard sends the whole wizard away (and
  // the redirect effect on to /signin) the next time it re-runs; this just avoids
  // opening the file picker in the meantime.
  function pickCv() {
    if (!isAuthenticated()) return;
    cvInput?.click();
  }

  async function onCvFile(e: Event) {
    const input = e.target as HTMLInputElement;
    const file = input.files?.[0];
    input.value = ''; // allow re-picking the same file after an error
    if (!file) return;
    const gen = ++cvGen;
    cvState = 'parsing';
    cvError = null;
    cvNote = null;
    try {
      const cv = await api.extractResumeProfile(file);
      track('cv_upload', { ok: true, origin: 'onboarding_gate' });
      // Marks the CV present so a later visit does not redirect back here — does NOT
      // navigate away itself: the user stays on this page to review/confirm the
      // extracted fields on the next steps (see the layout's redirect effect, which only
      // fires on a fresh navigation, not on this in-place state change).
      resumeStore.noteUpload();
      if (gen !== cvGen) return; // superseded by another pick or a page reset
      const merged = mergeFacets({ specializations, seniorities, skills }, cv);
      ({ specializations, seniorities, skills } = merged);
      cvState = 'idle';
      cvNote = merged.resolved
        ? 'Filled in what we found — review on the next step.'
        : 'Couldn’t read details from that CV — pick below.';
    } catch (err) {
      track('cv_upload', {
        ok: false,
        origin: 'onboarding_gate',
        reason: err instanceof ApiError ? cvUploadReason(err.message) : 'other',
      });
      if (gen !== cvGen) return;
      cvState = 'error';
      cvError = err instanceof ApiError ? err.message : 'Could not read the CV. Please try again.';
    }
  }

  // ---- Step "cv", second entry point: import from a public LinkedIn profile ----
  //
  // For the user who keeps their history on LinkedIn and has no PDF to hand. It fills the
  // same four fields a CV does, from the profile headline and address, through the same
  // dictionaries — see api.importLinkedInProfile.
  //
  // What it does NOT fill is work history, and that is a property of the source, not a gap
  // here: LinkedIn withholds every job title and position description from an anonymous
  // reader. The step says so out loud (see the markup), because "Import from LinkedIn" that
  // quietly imports no jobs reads as a bug rather than as a limit.

  let liUrl = $state('');
  let liState = $state<'idle' | 'loading' | 'error'>('idle');
  let liError = $state<string | null>(null);
  let liNote = $state<string | null>(null);
  let liGen = 0;

  // Where the import's reading of the profile's address goes. It is handed to the location
  // step as a DERIVED location, not written into the staged preferences: that component
  // already seeds an unstated base from a derivation and lets a stated one win, which is
  // exactly the precedence this needs — and an address we read off a page is something we
  // worked out about the user, not something they told us.
  let importedLocation = $state<DerivedLocation | null>(null);

  async function importLinkedIn() {
    if (!isAuthenticated()) {
      step = 1;
      return;
    }
    const url = liUrl.trim();
    if (!url || liState === 'loading') return;

    const gen = ++liGen;
    liState = 'loading';
    liError = null;
    liNote = null;
    try {
      const li = await api.importLinkedInProfile(url);
      track('linkedin_import', { ok: true, origin: 'onboarding_gate' });
      if (gen !== liGen) return; // superseded by another import or a page reset
      // Literally the same fold the CV path runs — a profile is one more source of evidence
      // about the user, not a different kind of thing, so it must not merge by different
      // rules.
      const merged = mergeFacets({ specializations, seniorities, skills }, li);
      ({ specializations, seniorities, skills } = merged);
      if (li.location) importedLocation = li.location;
      liState = 'idle';
      liNote = merged.resolved
        ? 'Filled in what we found — review on the next step.'
        : 'Couldn’t read details from that profile — pick below.';
    } catch (err) {
      track('linkedin_import', { ok: false, origin: 'onboarding_gate' });
      if (gen !== liGen) return;
      liState = 'error';
      liError = err instanceof ApiError ? err.message : 'Could not read that profile. Please try again.';
    }
  }

  // ---- Step "confirm": specialization / skills / level ----

  function toggleSpecialization(value: string) {
    specializations = specializations.includes(value)
      ? specializations.filter((v) => v !== value)
      : [...specializations, value];
  }

  // Specialization: the same grouped-sections-with-counts picker the job filter's
  // CategoryPane renders, rebuilt locally rather than reusing that component — it's
  // wired to a FacetStore (URL/localStorage-synced filter state) this page has no use
  // for, the same reason RoleCard.svelte (the profile's own Roles editor) doesn't reuse
  // it either. Counts come from the live, unfiltered category distribution — best-
  // effort: a failed/unavailable fetch (search down) just shows the picker without
  // counts, same as CategoryPane already tolerates a null `counts` prop.
  let specQuery = $state('');
  const specCollapsed = new SvelteSet<string>();
  let categoryDist = $state.raw<Record<string, number> | null>(null);
  $effect(() => {
    api.facetCounts(new URLSearchParams(), { facets: ['category'] })
      .then((res) => {
        categoryDist = res.facets?.category ?? null;
      })
      .catch(() => {});
  });
  const specGroups = $derived.by(() => {
    const q = specQuery.trim().toLowerCase();
    return CATEGORY_GROUPS.map((g) => ({
      ...g,
      options: q ? g.options.filter((o) => o.label.toLowerCase().includes(q)) : g.options,
    })).filter((g) => g.options.length > 0);
  });

  function toggleSeniority(value: string) {
    seniorities = seniorities.includes(value) ? seniorities.filter((v) => v !== value) : [...seniorities, value];
  }

  function toggleSkill(value: string) {
    skills = skills.includes(value) ? skills.filter((v) => v !== value) : [...skills, value];
  }

  // Skills typeahead, dictionary-backed like SkillsPicker — but skills only, no "avoid"
  // half, which is out of scope for this step.
  let skillDist = $state.raw<FacetOption[]>([]);
  let skillDistReady = $state(false);
  $effect(() => {
    void loadSkillDistribution().then((dist) => {
      skillDist = dist;
      skillDistReady = true;
    });
  });

  function searchSkills(query: string): Promise<FacetOption[]> {
    const q = query.trim().toLowerCase();
    const matches = q ? skillDist.filter((o) => o.label.toLowerCase().includes(q)) : skillDist;
    return Promise.resolve(matches.slice(0, q ? 50 : 8));
  }

  // ---- Step "location" ----

  function onLocationChange(next: LocationPreferences | null) {
    location = next;
  }

  // ---- Commit ----

  let saving = $state(false);

  // True when Level and/or Location have a pick that `finish()` is about to silently
  // drop — the save endpoint requires a non-empty specialization AND skill set for the
  // profile to exist at all, so a Level/Location-only save has nowhere to land. Shown
  // as a heads-up near the footer rather than blocking Finish/Skip (everything here
  // stays skippable) — the alternative was the picks vanishing with no explanation.
  const willLoseOrphanedPicks = $derived(
    (seniorities.length > 0 || location !== null) && (specializations.length === 0 || skills.length === 0),
  );

  // Commits once, whenever the page is left from the cv/confirm/location steps —
  // completion, skip-to-end, or an explicit dismissal all funnel through here. Skipped
  // entirely unless BOTH specializations and skills ended up with at least one value
  // (the save endpoint rejects either being empty), leaving any existing profile
  // untouched. Best-effort: a failed save must not trap the user on this page — they can
  // still edit and save from /my/profile. Marks the visit dismissed and navigates to
  // returnTo either way, so the layout's redirect effect does not immediately send them
  // right back.
  async function finish() {
    onboardingGate.dismiss();
    if (specializations.length > 0 && skills.length > 0) {
      saving = true;
      try {
        await profileStore.save(
          specializations,
          skills,
          seniorities,
          profileStore.profile?.excluded_skills ?? [],
          location,
        );
      } catch {
        // best-effort — see doc comment above.
      } finally {
        saving = false;
      }
    }
    // eslint-disable-next-line svelte/no-navigation-without-resolve -- returnTo is a validated same-origin path (safeRedirect), not a typed route
    void goto(returnTo);
  }

  // Both the header "Skip" link and the footer primary button advance the page the same
  // way — nothing on any step is required, so there is no separate validation path
  // between them. On the last step, either one commits and leaves.
  function advance() {
    if (step < TOTAL_STEPS) step += 1;
    else void finish();
  }

  function back() {
    if (step > 1) step -= 1;
  }

  function onKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') void finish();
  }
</script>

<svelte:window onkeydown={onKeydown} />

<!-- A multi-select pill group: each pill toggles independently. -->
{#snippet multiPills(selected: string[], options: FacetOption[], toggle: (value: string) => void)}
  <div class="flex flex-wrap gap-2">
    {#each options as o (o.value)}
      <button
        type="button"
        onclick={() => toggle(o.value)}
        aria-pressed={selected.includes(o.value)}
        class={[
          'rounded-full border px-3 py-1.5 text-sm font-medium transition-colors',
          selected.includes(o.value) ? 'border-brand bg-brand text-brand-foreground' : 'border-border bg-card hover:bg-accent',
        ]}
      >
        {o.label}
      </button>
    {/each}
  </div>
{/snippet}

<!-- A field label with a "Clear" X — same pattern as FacetSection's section header:
     shown only once something is selected, clears that field's whole selection at once
     (separate from removing one chip/pill at a time). -->
{#snippet fieldLabel(text: string, count: number, onClear: () => void)}
  <div class="mb-2 flex min-h-6 items-center justify-between gap-2">
    <span class="text-sm font-medium">{text}</span>
    {#if count > 0}
      <button
        type="button"
        onclick={onClear}
        title="Clear {text}"
        aria-label="Clear {text}"
        class="flex size-5 items-center justify-center rounded-full text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
      >
        <X class="size-3.5" />
      </button>
    {/if}
  </div>
{/snippet}

{#if !isAuthenticated()}
  <!-- Redirecting to /signin — see the effect above. Nothing to render meanwhile,
       since the wizard below assumes an account already exists. -->
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
          {#each { length: TOTAL_STEPS } as _, i (i)}
            <span class={['h-1 w-7 rounded-full transition-colors', i < step ? 'bg-brand' : 'bg-border']}></span>
          {/each}
        </div>
        <button
          type="button"
          onclick={advance}
          class="text-sm font-medium text-muted-foreground transition-colors hover:text-foreground"
        >
          Skip
        </button>
        <button
          type="button"
          onclick={() => void finish()}
          aria-label="Close"
          class="flex size-8 items-center justify-center rounded-lg border border-border text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
        >
          <X class="size-4" />
        </button>
      </div>

      <!-- body -->
      <div class="no-scrollbar mx-auto min-h-0 w-full max-w-lg flex-1 overflow-y-auto px-5 py-6">
        <div class="mb-1 inline-flex items-center gap-1.5 text-xs font-semibold uppercase tracking-wide text-brand-strong">
          Step {step} of {TOTAL_STEPS}
        </div>
        {#if currentKind === 'cv'}
          <h2 class="text-xl font-semibold tracking-tight">Upload your CV</h2>
          <p class="mt-1 text-sm text-muted-foreground">We'll use it to fill in your role, skills, and level — you can always skip this.</p>

          <input type="file" accept=".pdf,application/pdf" bind:this={cvInput} onchange={onCvFile} class="hidden" />
          <button
            type="button"
            onclick={pickCv}
            disabled={cvState === 'parsing'}
            class="mt-4 flex w-full items-center justify-center gap-2 rounded-xl border border-dashed border-border bg-card px-4 py-3 text-sm font-medium transition-colors hover:border-brand hover:bg-accent disabled:opacity-60"
          >
            {#if cvState === 'parsing'}
              <LoaderCircle class="size-4 animate-spin" aria-hidden="true" /> Reading your CV…
            {:else}
              <FileUp class="size-4" aria-hidden="true" /> Upload CV
            {/if}
          </button>
          {#if cvState === 'error'}
            <p class="mt-2 text-xs text-destructive">{cvError}</p>
          {:else if cvNote}
            <p class="mt-2 text-xs text-muted-foreground">{cvNote}</p>
          {:else}
            <p class="mt-2 text-xs text-muted-foreground">PDF with selectable text, up to {RESUME_MAX_MB} MB.</p>
          {/if}

          <!-- The second entry point. Co-equal with the dropzone, not a fallback under it:
               a user with no PDF should not have to work out that the greyed-out half of
               the step is the one meant for them. -->
          <div class="mt-5 flex items-center gap-3">
            <div class="h-px flex-1 bg-border"></div>
            <span class="text-xs font-medium uppercase tracking-wide text-muted-foreground">or</span>
            <div class="h-px flex-1 bg-border"></div>
          </div>

          <form class="mt-4 flex gap-2" onsubmit={(e) => { e.preventDefault(); void importLinkedIn(); }}>
            <input
              bind:value={liUrl}
              type="text"
              inputmode="url"
              autocomplete="url"
              placeholder="linkedin.com/in/your-name"
              aria-label="Your LinkedIn profile link"
              disabled={liState === 'loading'}
              class="min-w-0 flex-1 rounded-xl border border-input bg-card px-3 py-2.5 text-sm outline-none focus:ring-2 focus:ring-ring disabled:opacity-60"
            />
            <button
              type="submit"
              disabled={liState === 'loading' || liUrl.trim() === ''}
              class="inline-flex shrink-0 items-center justify-center gap-2 rounded-xl border border-border bg-card px-4 py-2.5 text-sm font-medium transition-colors hover:border-brand hover:bg-accent disabled:opacity-60"
            >
              {#if liState === 'loading'}
                <LoaderCircle class="size-4 animate-spin" aria-hidden="true" /> Reading…
              {:else}
                Import
              {/if}
            </button>
          </form>

          {#if liState === 'error'}
            <p class="mt-2 text-xs text-destructive">{liError}</p>
          {:else if liNote}
            <p class="mt-2 text-xs text-muted-foreground">{liNote}</p>
          {/if}

          <!-- Said before anyone tries it, not after it disappoints them. LinkedIn does not
               release work history to a reader who is not signed in, so this fills your role,
               skills, level and location and nothing else. The PDF route below is the honest
               way to get the rest in, and it lands on the dropzone above. -->
          <p class="mt-2 text-xs text-muted-foreground">
            LinkedIn only shares your headline and location publicly — not your work history.
            To bring that in, open your profile on LinkedIn, choose <span class="font-medium text-foreground">More → Save to PDF</span>, and upload the file above.
          </p>
        {:else if currentKind === 'confirm'}
          <h2 class="text-xl font-semibold tracking-tight">Confirm your details</h2>
          <p class="mt-1 text-sm text-muted-foreground">Everything's optional — pick as many as apply.</p>

          <div class="mt-5">
            {@render fieldLabel('Specialization', specializations.length, () => (specializations = []))}
          </div>
          <div class="mb-4 flex items-center gap-2 rounded-lg border border-input px-3 focus-within:ring-2 focus-within:ring-ring">
            <Search class="size-4 shrink-0 text-muted-foreground" />
            <input
              bind:value={specQuery}
              placeholder="Search specializations…"
              aria-label="Search specializations"
              class="h-9 w-full bg-transparent text-sm outline-none placeholder:text-muted-foreground"
            />
          </div>
          {#each specGroups as g (g.name)}
            {@const isCollapsed = specCollapsed.has(g.name) && !specQuery}
            <div class="border-t border-border first:border-t-0">
              <button
                type="button"
                class="flex w-full items-center gap-2 py-3"
                onclick={() => (specCollapsed.has(g.name) ? specCollapsed.delete(g.name) : specCollapsed.add(g.name))}
              >
                <ChevronDown class={['size-4 text-muted-foreground transition-transform', isCollapsed && '-rotate-90']} />
                <h3 class="text-sm font-semibold tracking-tight">{g.name}</h3>
              </button>
              {#if !isCollapsed}
                <div class="flex flex-wrap gap-2 pb-3">
                  {#each g.options as o (o.value)}
                    {@const included = specializations.includes(o.value)}
                    <button
                      type="button"
                      onclick={() => toggleSpecialization(o.value)}
                      title={pillTitle(included, false, false)}
                      class={pillClass(included, false, 'px-3 py-1.5 text-sm')}
                    >
                      {o.label}{#if categoryDist}<span class="ml-1 opacity-60 tabular-nums">{(categoryDist[o.value] ?? 0).toLocaleString()}</span>{/if}
                    </button>
                  {/each}
                </div>
              {/if}
            </div>
          {/each}

          <div class="mt-6">
            {@render fieldLabel('Level', seniorities.length, () => (seniorities = []))}
          </div>
          {@render multiPills(seniorities, seniorityOptions, toggleSeniority)}
        {:else if currentKind === 'skills'}
          <h2 class="text-xl font-semibold tracking-tight">What are your skills?</h2>
          <p class="mt-1 text-sm text-muted-foreground">Optional — search and add as many as apply.</p>

          <div class="mt-5">
            {@render fieldLabel('Skills', skills.length, () => (skills = []))}
          </div>
          <RemoteSearchSelect
            search={searchSkills}
            include={skills}
            placeholder="Search skills"
            onToggle={toggleSkill}
            fallbackLabel={(v) => v}
            clearOnSelect
            ready={skillDistReady}
            techIcons
          />
        {:else}
          <h2 class="text-xl font-semibold tracking-tight">Where are you based?</h2>
          <p class="mt-1 text-sm text-muted-foreground">Also optional — helps us match you to the right jobs.</p>

          <div class="mt-5">
            <LocationPreferencesFields
              value={location}
              derivedLocation={importedLocation ?? profileStore.profile?.derived_location}
              onChange={onLocationChange}
            />
          </div>
        {/if}
      </div>

      <!-- footer: back / continue|finish -->
      <div class="mx-auto w-full max-w-lg border-t border-border px-5 py-3">
        {#if willLoseOrphanedPicks && (currentKind === 'skills' || currentKind === 'location')}
          <p class="mb-2 text-xs text-muted-foreground">
            Add a specialization and a skill (on the Confirm step) to save your level/location picks — otherwise they won't be kept.
          </p>
        {/if}
        <div class="flex items-center gap-3">
          {#if step > 1}
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
            onclick={advance}
            disabled={saving}
            class="inline-flex h-11 items-center gap-1.5 rounded-lg bg-brand px-6 text-sm font-semibold text-brand-foreground transition-opacity hover:opacity-90 disabled:opacity-60"
          >
            {step < TOTAL_STEPS ? 'Continue' : 'Finish'} <ArrowRight class="size-4" />
          </button>
        </div>
      </div>
    </div>
  </div>
{/if}

<style>
  /* Step content scrolls without a visible scrollbar (same pattern as JobDrawer's tab
     rail) — the full-screen steps read as a page, not a scroll pane with a rail. */
  .no-scrollbar {
    scrollbar-width: none;
    -ms-overflow-style: none;
  }
  .no-scrollbar::-webkit-scrollbar {
    display: none;
  }
</style>
