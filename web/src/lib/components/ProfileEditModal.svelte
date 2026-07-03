<script lang="ts">
  import { ArrowUp } from '@lucide/svelte';
  import { ApiError, extractResumeSkills, facetCounts } from '$lib/api';
  import { CATEGORY_OPTIONS, type FacetDef, type FacetStore } from '$lib/facets';
  import { emptyFilters, type FacetState } from '$lib/filters';
  import { profileStore } from '$lib/profile.svelte';
  import { StagedFilters } from '$lib/stagedFilters.svelte';
  import type { FacetCounts, UserProfile } from '$lib/types';
  import { Button } from '$lib/ui';
  import FacetSection from './facets/FacetSection.svelte';

  // Edit the single profile in a modal, reusing the job-search facet controls
  // (FacetSection) so specialization/skills selection stays identical to the jobs
  // filters. Seeded from the current profile; Save upserts via the store.
  let { profile, onClose }: { profile: UserProfile | null; onClose: () => void } = $props();

  // Mirror of the server's specialization cap (userprofile.maxSpecializations).
  const MAX_SPECIALIZATIONS = 5;

  // Profile-scoped facet defs: the same category/skills controls as job search, but
  // with the search-only exclude/match toggles off (a profile value is a plain choice,
  // not a filter mode).
  const CATEGORY_FACET: FacetDef = {
    param: 'category',
    label: 'Specializations',
    control: 'select',
    options: CATEGORY_OPTIONS,
    excludable: false,
    placeholder: 'Search specializations',
  };
  const SKILLS_FACET: FacetDef = {
    param: 'skills',
    label: 'Skills',
    control: 'select',
    dynamic: true,
    excludable: false,
    placeholder: 'Search skills',
  };

  // The staged store the controls edit, seeded from the profile. StagedFilters already
  // satisfies FacetStore, so the facet controls render against it unchanged.
  const staged = new StagedFilters();
  const seed = emptyFilters();
  const seedFacet = (values: string[]): FacetState => ({ values: [...values], exclude: false, matchAll: false });
  // The modal is recreated on every open (it renders under {#if editing}), so seeding
  // from the initial `profile` value is intended — it never goes stale.
  // svelte-ignore state_referenced_locally
  seed.facets['category'] = seedFacet(profile?.specializations ?? []);
  // svelte-ignore state_referenced_locally
  seed.facets['skills'] = seedFacet(profile?.skills ?? []);
  staged.seed(seed);

  // Set when a toggle is refused for hitting the specialization cap (drives the hint).
  let capHit = $state(false);

  // Cap the specialization count at the server limit by intercepting only category
  // toggles; everything else passes straight through to the staged store.
  const store: FacetStore = {
    facet: (p) => staged.facet(p),
    toggle: (p, v) => {
      const cat = staged.facet('category');
      if (p === 'category' && !cat.values.includes(v) && cat.values.length >= MAX_SPECIALIZATIONS) {
        capHit = true;
        return;
      }
      capHit = false;
      staged.toggle(p, v);
    },
    add: (p, v) => staged.add(p, v),
    remove: (p, v) => staged.remove(p, v),
    clearFacet: (p) => staged.clearFacet(p),
    setExclude: (p, e) => staged.setExclude(p, e),
    setMatchAll: (p, o) => staged.setMatchAll(p, o),
  };

  let counts = $state<FacetCounts | null>(null);
  let saveError = $state<string | null>(null);
  let busy = $state(false);

  const specializations = $derived(staged.facet('category').values);
  const skills = $derived(staged.facet('skills').values);
  const canSave = $derived(specializations.length > 0 && skills.length > 0);

  // The live skill distribution (with counts) that drives the dynamic skills control —
  // the same source and shape the jobs filter panel uses.
  $effect(() => {
    void loadCounts();
  });
  async function loadCounts() {
    try {
      counts = await facetCounts(new URLSearchParams());
    } catch {
      // best-effort: the skills control just has no live suggestions.
    }
  }

  // Résumé upload → skill extraction, merged (union) into the skills facet.
  let resumeBusy = $state(false);
  let resumeError = $state<string | null>(null);
  let resumeNote = $state<string | null>(null);
  let fileInput = $state<HTMLInputElement | null>(null);
  let dragActive = $state(false);
  let fileName = $state<string | null>(null);

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
      for (const s of extracted) staged.add('skills', s);
      const added = staged.facet('skills').values.length - before;
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

  async function save() {
    if (!canSave || busy) return;
    busy = true;
    saveError = null;
    try {
      await profileStore.save(specializations, skills);
      onClose();
    } catch (err) {
      saveError = err instanceof ApiError ? err.message : 'Could not save the profile. Please try again.';
      busy = false;
    }
  }
</script>

<svelte:window onkeydown={(e) => e.key === 'Escape' && onClose()} />

<div class="fixed inset-0 z-50 flex items-center justify-center p-4">
  <button type="button" aria-label="Close" class="absolute inset-0 bg-black/50" onclick={onClose}></button>

  <div
    role="dialog"
    aria-modal="true"
    aria-label="Edit profile"
    class="relative flex max-h-[90vh] w-full max-w-lg flex-col overflow-hidden rounded-lg border border-border bg-background shadow-lg"
  >
    <div class="border-b border-border px-6 py-4">
      <h2 class="text-base font-semibold tracking-tight">Edit profile</h2>
      <p class="mt-1 text-sm text-muted-foreground">
        Pick your specializations and skills — the same controls as the job filters.
      </p>
    </div>

    <div class="flex flex-col gap-4 overflow-y-auto px-6 py-4">
      <div class="flex flex-col gap-1">
        <FacetSection def={CATEGORY_FACET} {store} {counts} expand />
        <div class="flex items-center justify-between text-xs text-muted-foreground">
          <span>{specializations.length}/{MAX_SPECIALIZATIONS} selected</span>
          {#if capHit}
            <span class="text-destructive">At most {MAX_SPECIALIZATIONS} specializations.</span>
          {/if}
        </div>
      </div>

      <FacetSection def={SKILLS_FACET} {store} {counts} expand />

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
          class="flex flex-col items-center justify-center gap-2 rounded-xl border-2 border-dashed px-6 py-6 text-center transition-colors disabled:opacity-70 {dragActive
            ? 'border-primary bg-primary/5'
            : 'border-border hover:border-primary/60'}"
        >
          <span class="flex size-9 items-center justify-center rounded-full bg-primary text-primary-foreground">
            <ArrowUp class="size-4" />
          </span>
          <span class="text-sm font-semibold">{resumeBusy ? 'Analyzing…' : 'Drop PDF or choose a file'}</span>
          {#if fileName}<span class="text-xs text-primary">✓ {fileName}</span>{/if}
        </button>
        {#if resumeError}
          <p class="text-sm text-destructive">{resumeError}</p>
        {:else if resumeNote}
          <p class="text-xs text-muted-foreground">{resumeNote}</p>
        {/if}
      </div>

      {#if saveError}
        <p class="text-sm text-destructive">{saveError}</p>
      {/if}
    </div>

    <div class="flex items-center justify-end gap-2 border-t border-border px-6 py-4">
      <Button variant="ghost" onclick={onClose}>Cancel</Button>
      <Button variant="primary" onclick={save} disabled={!canSave || busy}>
        {busy ? 'Saving…' : 'Save'}
      </Button>
    </div>
  </div>
</div>
