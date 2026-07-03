<script lang="ts">
  import { ArrowUp, Check, X } from '@lucide/svelte';
  import { ApiError, extractResumeSkills, facetCounts } from '$lib/api';
  import { CATEGORY_OPTIONS, categoryLabel, type FacetOption } from '$lib/facets';
  import { profileStore } from '$lib/profile.svelte';
  import type { UserProfile } from '$lib/types';
  import { Button } from '$lib/ui';
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
  // only known skills are addable) and cap the visible matches.
  function searchSkills(query: string): Promise<FacetOption[]> {
    const q = query.trim().toLowerCase();
    const matches = q ? skillDist.filter((o) => o.label.toLowerCase().includes(q)) : skillDist;
    return Promise.resolve(matches.slice(0, 50));
  }

  async function loadSkills() {
    try {
      const counts = await facetCounts(new URLSearchParams());
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

  // Extract skills from a résumé PDF and merge them into the skills field as a
  // deduplicated union, preserving anything already entered. The upload also stores the
  // CV server-side, so notify the parent to refresh the CV-readiness state.
  async function analyzeResume(file: File) {
    resumeBusy = true;
    resumeError = null;
    resumeNote = null;
    try {
      const extracted = await extractResumeSkills(file);
      onCvUploaded?.();
      if (extracted.length === 0) {
        resumeNote = 'No known skills found in the CV.';
        return;
      }
      const before = skills.length;
      skills = [...new Set([...skills, ...extracted])];
      const added = skills.length - before;
      resumeNote =
        added > 0
          ? `Added ${added} skill${added === 1 ? '' : 's'} from your CV.`
          : 'All CV skills were already listed.';
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

  async function submit(e: SubmitEvent) {
    e.preventDefault();
    if (!canSubmit || busy) return;
    busy = true;
    formError = null;
    try {
      await profileStore.save(specializations, skills);
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

  {#if formError}
    <p class="text-sm text-destructive">{formError}</p>
  {/if}

  <div class="flex items-center gap-2 border-t border-border pt-4">
    <Button variant="primary" type="submit" disabled={!canSubmit || busy}>
      {busy ? 'Saving…' : editing ? 'Save changes' : 'Create profile'}
    </Button>
  </div>
</form>
