<script lang="ts">
  import { ArrowUp, Check, X } from '@lucide/svelte';
  import { api, ApiError } from '$lib/api';
  import {
    CATEGORY_OPTIONS,
    categoryLabel,
    COUNTRY_OPTIONS,
    REGION_OPTIONS,
    WORK_MODE_OPTIONS,
    type FacetOption,
  } from '$lib/facets';
  import { profileStore } from '$lib/profile.svelte';
  import type { LocationPreferences, UserProfile } from '$lib/types';
  import { Button, Input } from '$lib/ui';
  import RemoteSearchSelect from './facets/RemoteSearchSelect.svelte';
  import SearchSelect from './facets/SearchSelect.svelte';

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
  let formError = $state<string | null>(null);
  let busy = $state(false);
  // Which form section is shown — the profile is long, so split it into two tabs sharing
  // one Save (both tabs feed the same PUT).
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
  let baseCountry = $state<string>(loc0?.base.country ?? '');
  let baseCity = $state<string>(loc0?.base.city ?? '');
  let relocOpen = $state<boolean>(loc0?.relocation.open ?? false);
  let relocRegions = $state.raw<string[]>(loc0?.relocation.regions ?? []);
  let relocCountries = $state.raw<string[]>(loc0?.relocation.countries ?? []);
  let relocCities = $state.raw<string[]>(loc0?.relocation.cities ?? []);
  let cityDraft = $state('');

  // Work format gates the geo sub-forms (progressive disclosure): remote reach shows only
  // when Remote is accepted; the physical base + relocation show only for On-site/Hybrid.
  // Hidden fields linger in state (re-selecting the format restores the draft) but are not
  // saved — buildLocation() reads the same gates.
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

  const canSubmit = $derived(specializations.length > 0 && skills.length > 0);

  // The skills typeahead: filter the loaded distribution locally (dictionary-only, so
  // only known skills are addable). With no query, show just the popular top few; typing
  // widens to the full match list — so the field is not a wall of skills on open.
  function searchSkills(query: string): Promise<FacetOption[]> {
    const q = query.trim().toLowerCase();
    const matches = q ? skillDist.filter((o) => o.label.toLowerCase().includes(q)) : skillDist;
    return Promise.resolve(matches.slice(0, q ? 50 : 8));
  }

  async function loadSkills() {
    try {
      const counts = await api.facetCounts(new URLSearchParams());
      const dist = counts.facets?.skills ?? {};
      skillDist = Object.entries(dist)
        .map(([value, count]) => ({ value, label: value, count }))
        .toSorted((a, b) => b.count - a.count || a.label.localeCompare(b.label));
    } catch {
      // best-effort: the typeahead just has nothing to suggest.
    }
  }

  $effect(() => {
    void loadSkills();
  });

  // Derive skills + specialization from a résumé PDF and merge them into the fields as a
  // deduplicated union, preserving anything already entered (the specialization respects
  // the cap). The upload also stores the CV server-side, so notify the parent to refresh
  // the CV-readiness state.
  async function analyzeResume(file: File) {
    resumeBusy = true;
    resumeError = null;
    resumeNote = null;
    try {
      const cv = await api.extractResumeProfile(file);
      onCvUploaded?.();

      const beforeSkills = skills.length;
      skills = [...new Set([...skills, ...cv.skills])];
      const addedSkills = skills.length - beforeSkills;

      // Merge every specialization the CV resolved, respecting the cap; track how many
      // were added vs. left out by the cap so the note is accurate.
      let addedSpecs = 0;
      let cappedSpecs = 0; // resolved but the cap left no room
      for (const cat of cv.categories) {
        if (specializations.includes(cat)) continue;
        if (specializations.length < MAX_SPECIALIZATIONS) {
          specializations = [...specializations, cat];
          addedSpecs++;
        } else {
          cappedSpecs++;
        }
      }

      const parts: string[] = [];
      if (addedSkills > 0) parts.push(`${addedSkills} skill${addedSkills === 1 ? '' : 's'}`);
      if (addedSpecs > 0) parts.push(`${addedSpecs} specialization${addedSpecs === 1 ? '' : 's'}`);
      if (parts.length) resumeNote = `Added ${parts.join(' and ')} from your CV.`;
      else if (cappedSpecs > 0)
        resumeNote = `Reached the ${MAX_SPECIALIZATIONS}-specialization limit — nothing more added.`;
      else if (cv.skills.length === 0 && cv.categories.length === 0) resumeNote = 'No known skills found in the CV.';
      else resumeNote = 'Everything from your CV was already listed.';
    } catch (err) {
      resumeError = err instanceof ApiError ? err.message : 'Could not read the CV. Please try again.';
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

  // Toggle a value in a multi-select list (work modes, regions, countries), returning a new
  // array so $state.raw readers re-run.
  function toggleIn(list: string[], value: string): string[] {
    return list.includes(value) ? list.filter((v) => v !== value) : [...list, value];
  }

  // Two-state toggle-pill styling for the small fixed sets (work format, regions).
  function pillCls(active: boolean): string {
    return active
      ? 'rounded-full bg-primary px-3 py-1 text-sm font-medium text-primary-foreground'
      : 'rounded-full border border-border px-3 py-1 text-sm transition-colors hover:border-primary/60';
  }

  // Add the drafted city (trimmed, case-insensitive dedup) to the relocation targets.
  function addCity() {
    const city = cityDraft.trim();
    cityDraft = '';
    if (!city || relocCities.some((c) => c.toLowerCase() === city.toLowerCase())) return;
    relocCities = [...relocCities, city];
  }

  // Reassemble the flat fields into the location block, or null when the user picked nothing
  // (the server also collapses an all-empty block, but sending null keeps intent explicit).
  // Relocation targets are gated on `relocOpen`: withdrawing "Open to relocation" drops them
  // from the saved block (they linger in local state so toggling back on restores the draft),
  // matching how the filter seed ignores targets when the user is not open to relocating.
  function buildLocation(): LocationPreferences | null {
    // Only persist fields the current work format reveals — a hidden sub-form's lingering
    // draft is not saved (mirrors how the UI gates them).
    const loc: LocationPreferences = {
      work_modes: workModes,
      remote: wantsRemote ? { regions: remoteRegions, countries: remoteCountries } : {},
      base: wantsPhysical ? { country: baseCountry || undefined, city: baseCity.trim() || undefined } : {},
      relocation:
        wantsPhysical && relocOpen
          ? { open: true, regions: relocRegions, countries: relocCountries, cities: relocCities }
          : { open: false },
    };
    return workModes.length === 0 ? null : loc;
  }

  async function submit(e: SubmitEvent) {
    e.preventDefault();
    if (!canSubmit || busy) return;
    busy = true;
    formError = null;
    try {
      await profileStore.save(specializations, skills, buildLocation());
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
  <div class="flex gap-5 border-b border-border">
    <button
      type="button"
      onclick={() => (formTab = 'main')}
      class="-mb-px border-b-2 px-1 pb-2.5 text-sm font-medium transition-colors {formTab === 'main'
        ? 'border-primary text-foreground'
        : 'border-transparent text-muted-foreground hover:text-foreground'}"
    >
      Skills &amp; role
    </button>
    <button
      type="button"
      onclick={() => (formTab = 'location')}
      class="-mb-px border-b-2 px-1 pb-2.5 text-sm font-medium transition-colors {formTab === 'location'
        ? 'border-primary text-foreground'
        : 'border-transparent text-muted-foreground hover:text-foreground'}"
    >
      Location &amp; work
    </button>
  </div>

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
        : 'px-6 py-8'} {dragActive ? 'border-primary bg-primary/5' : 'border-border hover:border-primary/60'}"
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
        <span class="flex size-11 items-center justify-center rounded-full bg-primary text-primary-foreground">
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
      Extracts your skills below · parsed on the server; the file is stored to score its readiness.
    </span>
    {#if resumeError}
      <p class="text-sm text-destructive">{resumeError}</p>
    {:else if resumeNote}
      <p class="text-xs text-muted-foreground">{resumeNote}</p>
    {/if}
  </div>

  <!-- Skills -->
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
    />
  </div>

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
            class="inline-flex items-center gap-1 rounded-full bg-primary px-2.5 py-1 text-sm font-medium text-primary-foreground transition-opacity hover:opacity-90"
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

    <!-- Physical presence (base + relocation) — only for On-site / Hybrid. -->
    {#if wantsPhysical}
      <!-- Base location -->
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
          <Input bind:value={baseCity} aria-label="Base city" placeholder="City" class="w-full" />
        </div>
      </div>

      <!-- Relocation -->
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
          {#if relocCities.length > 0}
            <div class="flex flex-wrap gap-1.5">
              {#each relocCities as city (city)}
                <button
                  type="button"
                  onclick={() => (relocCities = relocCities.filter((c) => c !== city))}
                  class="inline-flex items-center gap-1 rounded-full bg-primary px-2.5 py-1 text-sm font-medium text-primary-foreground transition-opacity hover:opacity-90"
                >
                  {city}
                  <X class="size-3" />
                </button>
              {/each}
            </div>
          {/if}
          <Input
            bind:value={cityDraft}
            onkeydown={(e) => {
              if (e.key === 'Enter') {
                e.preventDefault();
                addCity();
              }
            }}
            aria-label="Add a relocation city"
            placeholder="Add a city, press Enter"
            class="w-full"
          />
        {/if}
      </div>
    {/if}
  </div>
  {/if}

  {#if formError}
    <p class="text-sm text-destructive">{formError}</p>
  {/if}

  <div class="flex items-center gap-3 border-t border-border pt-4">
    <Button variant="primary" type="submit" disabled={!canSubmit || busy}>
      {busy ? 'Saving…' : editing ? 'Save changes' : 'Create profile'}
    </Button>
    {#if !canSubmit}
      <span class="text-xs text-muted-foreground">
        Add a role and at least one skill in the “Skills &amp; role” tab to save.
      </span>
    {/if}
  </div>
</form>
