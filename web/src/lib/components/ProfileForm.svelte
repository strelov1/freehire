<script lang="ts">
  import { ArrowUp, Check, X } from '@lucide/svelte';
  import { api, ApiError, RESUME_MAX_MB } from '$lib/api';
  import { cvUploadReason, track } from '$lib/analytics';
  import {
    CATEGORY_OPTIONS,
    COUNTRY_OPTIONS,
    REGION_OPTIONS,
    searchCities,
    WORK_MODE_OPTIONS,
    type FacetOption,
  } from '$lib/facets';
  import { categoryLabel } from '$lib/labels';
  import { profileStore } from '$lib/profile.svelte';
  import { buildLocationPreferences } from '$lib/profileLocation';
  import { withSkills } from '$lib/profileSkills';
  import { loadSkillDistribution } from '$lib/skillDictionary';
  import type { LocationPreferences, UserProfile } from '$lib/types';
  import { Button } from '$lib/ui';
  import HeadshotField from './HeadshotField.svelte';
  import RemoteSearchSelect from './facets/RemoteSearchSelect.svelte';
  import SearchSelect from './facets/SearchSelect.svelte';
  import { TabStrip, tabStripId } from '$lib/ui';

  // Mirror of the server's specialization cap (searchprofile.maxSpecializations).
  const MAX_SPECIALIZATIONS = 5;

  // Inline editor for the single profile. `profile` seeds the fields (null = first-time
  // set-up); `hasCv` drives the CV block's uploaded/empty state. `onSaved` fires after a
  // successful save (the page re-fetches coverage); `onCvUploaded` fires after a résumé
  // upload stores a new CV server-side (the page re-fetches the ATS report / has_cv).
  let {
    profile,
    hasCv,
    onSaved,
    onCvUploaded,
  }: {
    profile: UserProfile | null;
    hasCv: boolean;
    onSaved?: () => void;
    onCvUploaded?: () => void;
  } = $props();

  const editing = $derived(profile !== null);

  // Seed the fields once from the profile prop. The parent keys this component on the
  // profile identity, so a different profile remounts it — capturing the initial value
  // is intended.
  // svelte-ignore state_referenced_locally
  let specializations = $state.raw<string[]>(profile ? [...profile.specializations] : []);
  // svelte-ignore state_referenced_locally
  let skills = $state.raw<string[]>(profile ? [...profile.skills] : []);
  // Skills to avoid — seeded into the jobs filter's skills EXCLUDE set by "Apply my
  // profile". Optional (an empty set is valid), kept disjoint from `skills` (a skill can't
  // be both wanted and avoided — the server enforces this too, dropping any overlap).
  // svelte-ignore state_referenced_locally
  let excludedSkills = $state.raw<string[]>(profile ? [...(profile.excluded_skills ?? [])] : []);
  let formError = $state<string | null>(null);
  let busy = $state(false);
  // Which form section is shown — the profile is long, so split it into two tabs sharing
  // one Save (both tabs feed the same PUT). The list drives TabStrip, and typing it `as const`
  // ties `formTab` to it: an id that is not a section stops being expressible. The first
  // tab's label drops "Skills" once a profile exists — Skills and Skills to avoid live on
  // their own top-level tab then (see /my/profile's Skills tab); they stay here only for
  // first-time set-up, before that tab is reachable.
  const formTabs = $derived(
    [
      { id: 'main', label: editing ? 'Role' : 'Skills & role' },
      { id: 'location', label: 'Location & work' },
    ] as const,
  );
  const formPanelId = 'profile-form-panel';
  let formTab = $state<'main' | 'location'>('main');

  // Location & work preferences — the optional "where & how I want to work" block. Seeded
  // once from the profile (the parent keys this component on profile identity, so a switch
  // remounts and re-seeds). Held as flat fields; buildLocation() reassembles the block on
  // save and the server collapses an all-empty block to "no preferences".
  // svelte-ignore state_referenced_locally
  const loc0 = profile?.location_preferences ?? null;
  let workModes = $state.raw<string[]>(loc0?.work_modes ?? []);
  let remoteRegions = $state.raw<string[]>(loc0?.remote.regions ?? []);
  let remoteCountries = $state.raw<string[]>(loc0?.remote.countries ?? []);
  // Where the user IS. Seeded from what they stated, falling back to what their CV was
  // read to say — so someone who has uploaded a CV confirms a fact rather than retyping
  // it. The derivation only ever fills an UNSTATED field: a saved base always wins, and
  // an ambiguous derivation (more than one country) offers nothing rather than guessing.
  // svelte-ignore state_referenced_locally
  const derived0 = profile?.derived_location ?? null;
  const derivedCountry = (derived0?.countries.length === 1 ? derived0.countries[0] : '') ?? '';
  const derivedCity = (derived0?.cities.length === 1 ? derived0.cities[0] : '') ?? '';
  let baseCountry = $state<string>(loc0?.base.country ?? derivedCountry);
  let baseCity = $state<string>(loc0?.base.city ?? derivedCity);
  let relocOpen = $state<boolean>(loc0?.relocation.open ?? false);
  let relocRegions = $state.raw<string[]>(loc0?.relocation.regions ?? []);
  let relocCountries = $state.raw<string[]>(loc0?.relocation.countries ?? []);
  let relocCities = $state.raw<string[]>(loc0?.relocation.cities ?? []);

  // Work format gates the two "where would you take work" sub-forms: remote reach shows
  // only when Remote is accepted, relocation only for On-site/Hybrid. Hidden fields linger
  // in state (re-selecting the format restores the draft) but are not saved —
  // buildLocation() reads the same gates.
  //
  // The physical BASE is deliberately not among them. Where someone lives is a fact about
  // them, not a preference conditional on the arrangements they accept, and it matters
  // most for a remote worker: it governs their right to work, their taxation and their
  // overlap with a team. It used to be gated here and DISCARDED on save for anyone who
  // accepted only remote work, which is why two hard-constraint checks that read it were
  // silently inert for most profiles.
  const wantsRemote = $derived(workModes.includes('remote'));
  const wantsPhysical = $derived(workModes.includes('onsite') || workModes.includes('hybrid'));

  // Résumé upload → skill extraction. The server also stores the CV (used by the CV
  // readiness tab); the returned slugs merge (union) into the skills field without
  // wiping manual entries. They persist to the profile only on Save.
  let resumeBusy = $state(false);
  let resumeError = $state<string | null>(null);
  let resumeNote = $state<string | null>(null);
  let fileInput = $state<HTMLInputElement | null>(null);
  let dragActive = $state(false);

  // The universe of skills (canonical tokens with job counts) for the typeahead, from
  // the facet-distribution endpoint — the same source the filter panel uses.
  let skillDist = $state.raw<FacetOption[]>([]);
  // See RemoteSearchSelect's `ready` prop: without it, a dictionary fetch slower
  // than the picker's 250ms debounce leaves the popular first page stuck empty.
  let skillDistReady = $state(false);

  const canSubmit = $derived(specializations.length > 0 && skills.length > 0);

  // The skills typeahead: filter the loaded distribution locally (dictionary-only, so
  // only known skills are addable). With no query, show just the popular top few; typing
  // widens to the full match list — so the field is not a wall of skills on open. `avoid`
  // hides tokens already picked in the other skills control, keeping wanted and excluded
  // disjoint.
  function searchSkillsExcept(query: string, avoid: string[]): Promise<FacetOption[]> {
    const q = query.trim().toLowerCase();
    const pool = skillDist.filter((o) => !avoid.includes(o.value));
    const matches = q ? pool.filter((o) => o.label.toLowerCase().includes(q)) : pool;
    return Promise.resolve(matches.slice(0, q ? 50 : 8));
  }

  const searchSkills = (query: string) => searchSkillsExcept(query, excludedSkills);
  const searchExcludedSkills = (query: string) => searchSkillsExcept(query, skills);

  $effect(() => {
    void loadSkillDistribution().then((dist) => {
      skillDist = dist;
      skillDistReady = true;
    });
  });

  // Derive skills + specialization from a résumé PDF. Before a profile exists, both merge
  // into local fields the same way (a profile needs both up front, so Save persists them
  // together). Once a profile exists, Skills has moved to its own autosaving tab, so the
  // extraction writes both fields straight to the profile in one PUT instead: not skills
  // alone, because a specialization it also resolved would otherwise sit in this
  // component's local (unsaved) state right as a skills-only write's reseed discards it —
  // this component is keyed on the profile's own `updated_at` (see +page.svelte), so any
  // write to the row remounts it fresh from the server, wiping whatever wasn't included in
  // that write. The upload also stores the CV server-side, so notify the parent to refresh
  // the CV-readiness state.
  async function analyzeResume(file: File) {
    resumeBusy = true;
    resumeError = null;
    resumeNote = null;
    try {
      const cv = await api.extractResumeProfile(file);
      track('cv_upload', { ok: true, origin: 'profile' });
      onCvUploaded?.();

      // Merge every specialization the CV resolved, respecting the cap; track how many
      // were added vs. left out by the cap so the note is accurate. Always local first —
      // `nextSpecializations` is what an editing-mode write below persists alongside skills.
      let addedSpecs = 0;
      let cappedSpecs = 0; // resolved but the cap left no room
      const nextSpecializations = [...specializations];
      for (const cat of cv.categories) {
        if (nextSpecializations.includes(cat)) continue;
        if (nextSpecializations.length < MAX_SPECIALIZATIONS) {
          nextSpecializations.push(cat);
          addedSpecs++;
        } else {
          cappedSpecs++;
        }
      }
      specializations = nextSpecializations;

      let addedSkills: number;
      if (editing) {
        const current = profileStore.profile;
        const before = current?.skills ?? [];
        addedSkills = current ? withSkills(current, cv.skills).skills.length - before.length : 0;
        if (addedSkills > 0 || addedSpecs > 0) {
          await profileStore.mergeResumeExtraction(cv.skills, nextSpecializations);
        }
      } else {
        const beforeSkills = skills.length;
        skills = [...new Set([...skills, ...cv.skills])];
        addedSkills = skills.length - beforeSkills;
      }

      const parts: string[] = [];
      if (addedSkills > 0) parts.push(`${addedSkills} skill${addedSkills === 1 ? '' : 's'}`);
      if (addedSpecs > 0) parts.push(`${addedSpecs} specialization${addedSpecs === 1 ? '' : 's'}`);
      if (parts.length) {
        resumeNote =
          editing && addedSkills > 0
            ? `Added ${parts.join(' and ')} from your CV — see the Skills tab.`
            : `Added ${parts.join(' and ')} from your CV.`;
      } else if (cappedSpecs > 0)
        resumeNote = `Reached the ${MAX_SPECIALIZATIONS}-specialization limit — nothing more added.`;
      else if (cv.skills.length === 0 && cv.categories.length === 0) resumeNote = 'No known skills found in the CV.';
      else resumeNote = 'Everything from your CV was already listed.';
    } catch (err) {
      resumeError = err instanceof ApiError ? err.message : 'Could not read the CV. Please try again.';
      // The reason goes out as a bounded code, never the message itself: the copy is
      // written for the user and rewording it must not split the metric in two.
      track('cv_upload', {
        ok: false,
        origin: 'profile',
        reason: err instanceof ApiError ? cvUploadReason(err.message) : 'other',
      });
    } finally {
      resumeBusy = false;
    }
  }

  function onResumeFile(e: Event) {
    const target = e.currentTarget as HTMLInputElement;
    const file = target.files?.[0];
    target.value = ''; // clear so re-selecting the same file fires change again
    if (file) void analyzeResume(file);
  }

  function isPdf(file: File): boolean {
    return file.type === 'application/pdf' || file.name.toLowerCase().endsWith('.pdf');
  }

  // The file input's `accept` filters the picker, but a drop bypasses it — so the drop
  // path validates the type itself before touching the extractor.
  function onDrop(e: DragEvent) {
    e.preventDefault();
    dragActive = false;
    const file = e.dataTransfer?.files?.[0];
    if (!file) return;
    if (!isPdf(file)) {
      resumeError = 'Please drop a PDF file.';
      return;
    }
    void analyzeResume(file);
  }

  // Multi-select with a cap: toggling off is always allowed; toggling on is refused past
  // MAX_SPECIALIZATIONS (with a hint) so the form matches the server's limit.
  function toggleSpecialization(value: string) {
    if (specializations.includes(value)) {
      specializations = specializations.filter((s) => s !== value);
      formError = null;
      return;
    }
    if (specializations.length >= MAX_SPECIALIZATIONS) {
      formError = `You can pick at most ${MAX_SPECIALIZATIONS} specializations.`;
      return;
    }
    specializations = [...specializations, value];
    formError = null;
  }

  function toggleSkill(value: string) {
    skills = skills.includes(value) ? skills.filter((s) => s !== value) : [...skills, value];
  }

  function toggleExcludedSkill(value: string) {
    excludedSkills = excludedSkills.includes(value)
      ? excludedSkills.filter((s) => s !== value)
      : [...excludedSkills, value];
  }

  // Toggle a value in a multi-select list (work modes, regions, countries), returning a new
  // array so $state.raw readers re-run.
  function toggleIn(list: string[], value: string): string[] {
    return list.includes(value) ? list.filter((v) => v !== value) : [...list, value];
  }

  // Two-state toggle-pill styling for the small fixed sets (work format, regions).
  function pillCls(active: boolean): string {
    return active
      ? 'rounded-full bg-brand px-3 py-1 text-sm font-medium text-brand-foreground'
      : 'rounded-full border border-border px-3 py-1 text-sm transition-colors hover:border-brand/60';
  }

  // Reassemble the flat fields into the saved location block. The rules — which sub-forms
  // the work format gates, and why `base` is NOT among them — live in profileLocation.ts so
  // they are unit-testable; this component only supplies the fields.
  function buildLocation(): LocationPreferences | null {
    return buildLocationPreferences({
      workModes,
      remoteRegions,
      remoteCountries,
      baseCountry,
      baseCity,
      relocOpen,
      relocRegions,
      relocCountries,
      relocCities,
    });
  }

  async function submit(e: SubmitEvent) {
    e.preventDefault();
    if (!canSubmit || busy) return;
    busy = true;
    formError = null;
    try {
      await profileStore.save(specializations, skills, excludedSkills, buildLocation());
      onSaved?.();
    } catch (err) {
      formError =
        err instanceof ApiError ? err.message : 'Could not save the profile. Please try again.';
    } finally {
      busy = false;
    }
  }
</script>

<form onsubmit={submit} class="flex flex-col gap-6 rounded-xl border border-border bg-card p-5 sm:p-6">
  <!-- Sub-tabs: the professional self vs. where/how you want to work. One Save below covers
       both (a single PUT). -->
  <TabStrip
    tabs={formTabs}
    active={formTab}
    onSelect={(id) => (formTab = id)}
    label="Profile form sections"
    panelId={formPanelId}
  />

  <!-- Region-pill group, reused by remote reach and relocation targets. Declared at the form
       top level so it is available inside either tab. -->
  {#snippet regionPills(selected: string[], onToggle: (v: string) => void)}
    <div class="flex flex-wrap gap-1.5">
      {#each REGION_OPTIONS as opt (opt.value)}
        <button type="button" onclick={() => onToggle(opt.value)} class={pillCls(selected.includes(opt.value))}>
          {opt.label}
        </button>
      {/each}
    </div>
  {/snippet}

  <!-- A geographic reach: region pills + a top-N country search. Shared by remote reach and
       relocation targets so the two stay identical. -->
  {#snippet geoReach(
    regions: string[],
    onRegion: (v: string) => void,
    countries: string[],
    onCountry: (v: string) => void,
  )}
    {@render regionPills(regions, onRegion)}
    <SearchSelect
      options={COUNTRY_OPTIONS}
      include={countries}
      placeholder="Add specific countries"
      onToggle={onCountry}
      cap={8}
      clearOnSelect
    />
  {/snippet}

  <!-- One panel around both sections: they are mutually exclusive, so a single element can
       carry the tabpanel role and stay pointed at whichever tab is active. `gap-6` repeats the
       form's own spacing, which the section's blocks used to inherit as direct form children. -->
  <div
    id={formPanelId}
    role="tabpanel"
    aria-labelledby={tabStripId(formPanelId, formTab)}
    class="flex flex-col gap-6"
  >
  {#if formTab === 'main'}
  <!-- Your CV: uploaded state or an empty drop-zone. Uploading extracts skills into the
       field below and stores the CV for the readiness tab. -->
  <div class="flex flex-col gap-1.5">
    <span class="text-sm font-medium">Your CV</span>
    <input
      type="file"
      accept="application/pdf,.pdf"
      class="hidden"
      bind:this={fileInput}
      onchange={onResumeFile}
    />
    <button
      type="button"
      onclick={() => fileInput?.click()}
      ondragover={(e) => {
        e.preventDefault();
        dragActive = true;
      }}
      ondragleave={(e) => {
        e.preventDefault();
        dragActive = false;
      }}
      ondrop={onDrop}
      disabled={resumeBusy}
      class="flex items-center justify-center gap-3 rounded-xl border-2 border-dashed text-center transition-colors disabled:opacity-70 {hasCv
        ? 'px-4 py-3'
        : 'px-6 py-8'} {dragActive ? 'border-brand bg-brand/5' : 'border-border hover:border-brand/60'}"
    >
      {#if hasCv}
        <Check class="size-4 shrink-0 text-primary" />
        <span class="text-sm">
          <span class="font-medium">{resumeBusy ? 'Analyzing…' : 'CV uploaded'}</span>
          {#if !resumeBusy}
            <span class="text-muted-foreground">· drop a new PDF to update</span>
          {/if}
        </span>
      {:else}
        <span class="flex size-11 items-center justify-center rounded-full bg-brand text-brand-foreground">
          <ArrowUp class="size-5" />
        </span>
        <span class="flex flex-col gap-0.5 text-left">
          <span class="text-sm font-semibold">{resumeBusy ? 'Analyzing…' : 'Drop PDF here'}</span>
          {#if !resumeBusy}
            <span class="text-xs text-muted-foreground">
              or <span class="text-primary underline">choose from disk</span>
            </span>
          {/if}
        </span>
      {/if}
    </button>
    <span class="text-xs text-muted-foreground">
      PDF with selectable text, up to {RESUME_MAX_MB} MB ·
      {editing ? 'adds new skills to your profile' : 'extracts your skills below'}; the file is
      stored to score its readiness.
    </span>
    {#if resumeError}
      <p class="text-sm text-destructive">{resumeError}</p>
    {:else if resumeNote}
      <p class="text-xs text-muted-foreground">{resumeNote}</p>
    {/if}
  </div>

  <!-- Your photo: one headshot per member, printed by the CV templates that show one.
       Renders nothing when object storage is unconfigured. -->
  <HeadshotField />

  {#if !editing}
  <!-- Skills — set-up only. Once a profile exists, Skills and Skills to avoid move to
       their own top-level tab (autosaving, no Save button); the server also requires at
       least one skill to create a profile in the first place, so they stay here until
       there is a profile — and a Skills tab — to move to. -->
  <div class="flex flex-col gap-2">
    <div class="flex items-baseline justify-between">
      <span class="text-sm font-medium">Skills</span>
      <span class="text-xs tabular-nums text-muted-foreground">{skills.length}</span>
    </div>
    <RemoteSearchSelect
      search={searchSkills}
      include={skills}
      placeholder="Search skills"
      onToggle={toggleSkill}
      fallbackLabel={(v) => v}
      clearOnSelect
      ready={skillDistReady}
    />
  </div>

  <!-- Skills to avoid — optional; seeded into the filter's skills exclude set by "Apply my
       profile". Same dictionary source as Skills, kept disjoint from it. Passed as the
       control's `exclude` set so the chips render in the destructive (red, struck-through)
       style, matching how an excluded facet value looks everywhere else. -->
  <div class="flex flex-col gap-2">
    <div class="flex items-baseline justify-between">
      <span class="text-sm font-medium">Skills to avoid</span>
      <span class="text-xs tabular-nums text-muted-foreground">{excludedSkills.length}</span>
    </div>
    <RemoteSearchSelect
      search={searchExcludedSkills}
      include={[]}
      exclude={excludedSkills}
      placeholder="Search skills to exclude"
      onToggle={toggleExcludedSkill}
      fallbackLabel={(v) => v}
      clearOnSelect
      ready={skillDistReady}
    />
    <span class="text-xs text-muted-foreground">
      Filtered out when you apply your profile to the job filters.
    </span>
  </div>
  {/if}

  <!-- Role / specializations -->
  <div class="flex flex-col gap-2">
    <div class="flex items-baseline justify-between">
      <span class="text-sm font-medium">Role</span>
      <span class="text-xs tabular-nums text-muted-foreground">
        {specializations.length}/{MAX_SPECIALIZATIONS}
      </span>
    </div>
    {#if specializations.length > 0}
      <div class="flex flex-wrap gap-1.5">
        {#each specializations as value (value)}
          <button
            type="button"
            onclick={() => toggleSpecialization(value)}
            class="inline-flex items-center gap-1 rounded-full bg-brand px-2.5 py-1 text-sm font-medium text-brand-foreground transition-opacity hover:opacity-90"
          >
            {categoryLabel(value)}
            <X class="size-3" />
          </button>
        {/each}
      </div>
    {/if}
    <SearchSelect
      options={CATEGORY_OPTIONS}
      include={specializations}
      placeholder="Search specializations"
      onToggle={toggleSpecialization}
      clearOnSelect
    />
  </div>

  {:else}
  <!-- Location & work: optional preferences. Every part is optional; leaving it all empty
       stores "no preferences". -->
  <div class="flex flex-col gap-4">
    <span class="text-xs text-muted-foreground">All optional — used to tailor your job filters.</span>

    <!-- Work format -->
    <div class="flex flex-col gap-1.5">
      <span class="text-xs font-medium text-muted-foreground">Work format</span>
      <div class="flex flex-wrap gap-1.5">
        {#each WORK_MODE_OPTIONS as opt (opt.value)}
          <button type="button" onclick={() => (workModes = toggleIn(workModes, opt.value))} class={pillCls(workModes.includes(opt.value))}>
            {opt.label}
          </button>
        {/each}
      </div>
    </div>

    <!-- Where you are now. Asked of EVERY user, whatever work formats they accept: it is
         a fact about the person, not a preference, and it is what the visa-sponsorship and
         onsite-country checks compare a job against. Pre-filled from the CV when the user
         has stated nothing, so they confirm rather than retype. -->
    <div class="flex flex-col gap-1.5">
      <span class="text-xs font-medium text-muted-foreground">Where you're based</span>
      <div class="grid grid-cols-1 gap-2 sm:grid-cols-2">
        <select
          bind:value={baseCountry}
          aria-label="Base country"
          class="h-9 rounded-lg border border-input bg-transparent px-3 text-sm transition-colors focus-visible:border-ring focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50 dark:bg-input/30"
        >
          <option value="">Country…</option>
          {#each COUNTRY_OPTIONS as opt (opt.value)}
            <option value={opt.value}>{opt.label}</option>
          {/each}
        </select>
        <RemoteSearchSelect
          search={(q) => searchCities(q, baseCountry)}
          include={baseCity ? [baseCity] : []}
          onToggle={(v) => (baseCity = baseCity === v ? '' : v)}
          fallbackLabel={(v) => v}
          placeholder="City"
          clearOnSelect
        />
      </div>
    </div>

    {#if workModes.length === 0}
      <span class="text-xs text-muted-foreground">Pick a work format above to set where you can work.</span>
    {/if}

    <!-- Remote reach — only relevant once Remote is accepted. -->
    {#if wantsRemote}
      <div class="flex flex-col gap-1.5">
        <span class="text-xs font-medium text-muted-foreground">Remote — regions you can work for (empty = worldwide)</span>
        {@render geoReach(
          remoteRegions,
          (v) => (remoteRegions = toggleIn(remoteRegions, v)),
          remoteCountries,
          (v) => (remoteCountries = toggleIn(remoteCountries, v)),
        )}
      </div>
    {/if}

    <!-- Relocation — only meaningful for someone who would take physical work. -->
    {#if wantsPhysical}
      <div class="flex flex-col gap-2">
        <label class="flex items-center gap-2 text-sm">
          <input type="checkbox" bind:checked={relocOpen} class="size-4 rounded border-input" />
          Open to relocation
        </label>
        {#if relocOpen}
          <span class="text-xs font-medium text-muted-foreground">Where you'd relocate (empty = anywhere)</span>
          {@render geoReach(
            relocRegions,
            (v) => (relocRegions = toggleIn(relocRegions, v)),
            relocCountries,
            (v) => (relocCountries = toggleIn(relocCountries, v)),
          )}
          <RemoteSearchSelect
            search={(q) => searchCities(q)}
            include={relocCities}
            onToggle={(v) => (relocCities = toggleIn(relocCities, v))}
            fallbackLabel={(v) => v}
            placeholder="Add a city"
            clearOnSelect
          />
        {/if}
      </div>
    {/if}
  </div>
  {/if}
  </div>

  {#if formError}
    <p class="text-sm text-destructive">{formError}</p>
  {/if}

  <div class="flex items-center gap-3 border-t border-border pt-4">
    <Button variant="primary" type="submit" disabled={!canSubmit || busy}>
      {busy ? 'Saving…' : editing ? 'Save changes' : 'Create profile'}
    </Button>
    {#if !canSubmit}
      <span class="text-xs text-muted-foreground">
        {editing
          ? 'Add a role in the Role tab to save.'
          : 'Add a role and at least one skill in the “Skills & role” tab to save.'}
      </span>
    {/if}
  </div>
</form>
