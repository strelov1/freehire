<script lang="ts">
  import { ArrowLeft, ArrowUp, X } from '@lucide/svelte';
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { ApiError, extractResumeSkills, facetCounts } from '$lib/api';
  import { CATEGORY_OPTIONS, categoryLabel, type FacetOption } from '$lib/facets';
  import { searchProfiles } from '$lib/searchProfiles.svelte';
  import type { SearchProfile } from '$lib/types';
  import { Button, Input } from '$lib/ui';
  import RemoteSearchSelect from './facets/RemoteSearchSelect.svelte';
  import SearchSelect from './facets/SearchSelect.svelte';

  // Mirror of the server's specialization cap (searchprofile.maxSpecializations).
  const MAX_SPECIALIZATIONS = 5;

  // Undefined = create; a profile = edit (its fields seed the form). The wrapping
  // route resolves the profile (or redirects) before rendering, so here it is a fact.
  let { profile }: { profile?: SearchProfile } = $props();

  const editing = $derived(profile !== undefined);

  // Seed the form once from the profile prop. The edit route keys on `profile.id`, so
  // a different profile remounts this component — the initial-value capture is intended.
  // svelte-ignore state_referenced_locally
  let name = $state(profile?.name ?? '');
  // svelte-ignore state_referenced_locally
  let specializations = $state.raw<string[]>(profile ? [...profile.specializations] : []);
  // svelte-ignore state_referenced_locally
  let skills = $state.raw<string[]>(profile ? [...profile.skills] : []);
  let formError = $state<string | null>(null);
  let busy = $state(false);

  // Résumé upload → skill extraction. The file/text is parsed server-side and discarded;
  // the returned slugs merge (union) into the skills field without wiping manual entries.
  let resumeBusy = $state(false);
  let resumeError = $state<string | null>(null);
  let resumeNote = $state<string | null>(null);
  let fileInput = $state<HTMLInputElement | null>(null);
  let dragActive = $state(false);
  let fileName = $state<string | null>(null);

  // The universe of skills (canonical tokens with job counts) for the typeahead, from
  // the facet-distribution endpoint — the same source the filter panel uses. Empty
  // params = the whole catalogue's skill distribution.
  let skillDist = $state.raw<FacetOption[]>([]);

  const canSubmit = $derived(
    name.trim() !== '' && specializations.length > 0 && skills.length > 0,
  );

  const backHref = resolve('/my/profiles');

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
      // best-effort: an edited profile's skills still render as chips (seeded via
      // fallbackLabel); the typeahead just has nothing to suggest.
    }
  }

  $effect(() => {
    void loadSkills();
  });

  // Extract skills from a résumé PDF and merge them into the skills field as a
  // deduplicated union, preserving anything already entered.
  async function analyzeResume(file: File) {
    resumeBusy = true;
    resumeError = null;
    resumeNote = null;
    fileName = file.name;
    try {
      const extracted = await extractResumeSkills(file);
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
      resumeError =
        err instanceof ApiError ? err.message : 'Could not read the CV. Please try again.';
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

  // The file input's `accept` filters the picker, but a drop bypasses it — so the
  // drop path validates the type itself before touching the extractor.
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

  function onDragOver(e: DragEvent) {
    e.preventDefault();
    dragActive = true;
  }

  function onDragLeave(e: DragEvent) {
    e.preventDefault();
    dragActive = false;
  }

  // Multi-select with a cap: toggling off is always allowed; toggling on is refused
  // past MAX_SPECIALIZATIONS (with a hint) so the form matches the server's limit.
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
      if (profile === undefined) {
        await searchProfiles.create(name.trim(), specializations, skills);
      } else {
        await searchProfiles.update(profile.id, {
          name: name.trim(),
          specializations,
          skills,
        });
      }
      await goto(backHref);
    } catch (err) {
      formError =
        err instanceof ApiError ? err.message : 'Could not save the profile. Please try again.';
      busy = false;
    }
  }
</script>

<div class="flex flex-col gap-6">
  <div class="flex flex-col gap-3">
    <a
      href={backHref}
      class="inline-flex w-fit items-center gap-1 text-sm text-muted-foreground transition-colors hover:text-foreground"
    >
      <ArrowLeft class="size-4" />
      Profiles
    </a>
    <div class="flex flex-col gap-1">
      <h1 class="text-2xl font-semibold tracking-tight">
        {editing ? 'Edit profile' : 'New profile'}
      </h1>
      <p class="text-sm text-muted-foreground">
        Describe what you do — one or more specializations and your skills — and reuse it to
        find relevant work.
      </p>
    </div>
  </div>

  <form onsubmit={submit} class="flex flex-col gap-6 rounded-xl border border-border bg-card p-5 sm:p-6">
    <label class="flex flex-col gap-1.5">
      <span class="text-sm font-medium">Name</span>
      <Input bind:value={name} placeholder="e.g. Go backend" maxlength={100} class="w-full" />
      <span class="text-xs text-muted-foreground">A short label to recognise this profile.</span>
    </label>

    <div class="flex flex-col gap-2">
      <div class="flex items-baseline justify-between">
        <span class="text-sm font-medium">Specializations</span>
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
        selected={specializations}
        placeholder="Search specializations"
        onToggle={toggleSpecialization}
        clearOnSelect
      />
    </div>

    <div class="flex flex-col gap-2">
      <div class="flex items-baseline justify-between">
        <span class="text-sm font-medium">Skills</span>
        <span class="text-xs tabular-nums text-muted-foreground">{skills.length}</span>
      </div>
      <RemoteSearchSelect
        search={searchSkills}
        selected={skills}
        placeholder="Search skills"
        onToggle={toggleSkill}
        fallbackLabel={(v) => v}
        clearOnSelect
      />

      <div class="flex flex-col gap-1.5">
        <span class="text-sm font-medium">Add skills from your CV</span>
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
          ondragover={onDragOver}
          ondragleave={onDragLeave}
          ondrop={onDrop}
          disabled={resumeBusy}
          class="flex flex-col items-center justify-center gap-3 rounded-xl border-2 border-dashed px-6 py-8 text-center transition-colors disabled:opacity-70 {dragActive
            ? 'border-primary bg-primary/5'
            : 'border-border hover:border-primary/60'}"
        >
          <span
            class="flex size-11 items-center justify-center rounded-full bg-primary text-primary-foreground"
          >
            <ArrowUp class="size-5" />
          </span>
          <span class="flex flex-col gap-0.5">
            <span class="text-sm font-semibold">{resumeBusy ? 'Analyzing…' : 'Drop PDF here'}</span>
            {#if !resumeBusy}
              <span class="text-xs text-muted-foreground">
                or <span class="text-primary underline">choose from disk</span>
              </span>
            {/if}
          </span>
          {#if fileName}
            <span class="text-xs text-primary">✓ {fileName}</span>
          {/if}
        </button>
        <span class="text-xs text-muted-foreground">
          Extracts your skills into the field above · parsed on the server and discarded.
        </span>
        {#if resumeError}
          <p class="text-sm text-destructive">{resumeError}</p>
        {:else if resumeNote}
          <p class="text-xs text-muted-foreground">{resumeNote}</p>
        {/if}
      </div>
    </div>

    {#if formError}
      <p class="text-sm text-destructive">{formError}</p>
    {/if}

    <div class="flex items-center gap-2 border-t border-border pt-4">
      <Button variant="primary" type="submit" disabled={!canSubmit || busy}>
        {busy ? 'Saving…' : editing ? 'Save changes' : 'Create profile'}
      </Button>
      <Button variant="ghost" href={backHref}>Cancel</Button>
    </div>
  </form>
</div>
