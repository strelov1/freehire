<script lang="ts">
  import { ArrowUp, Check, Trash2 } from '@lucide/svelte';
  import { api, ApiError, RESUME_MAX_MB } from '$lib/api';
  import { cvUploadReason, track } from '$lib/analytics';
  import { CATEGORY_OPTIONS } from '$lib/facets';
  import { profileStore } from '$lib/profile.svelte';
  import { withSkills } from '$lib/profileSkills';
  import type { LocationPreferences, UserProfile } from '$lib/types';
  import { Button, ConfirmDialog } from '$lib/ui';
  import HeadshotField from './HeadshotField.svelte';
  import SearchSelect from './facets/SearchSelect.svelte';
  import LocationPreferencesFields from './profile/LocationPreferencesFields.svelte';
  import SkillsPicker from './profile/SkillsPicker.svelte';

  // Mirror of the server's specialization cap (searchprofile.maxSpecializations).
  const MAX_SPECIALIZATIONS = 5;

  // Inline editor for the single profile. `profile` seeds the fields (null = first-time
  // set-up); `hasCv` drives the CV block's uploaded/empty state. `onSaved` fires after a
  // successful save (the page re-fetches coverage); `onCvUploaded` fires after a résumé
  // upload stores a new CV server-side (the page re-fetches the ATS report / has_cv).
  //
  // Once a profile exists, Roles/Skills/Location each have their own view (autosaving
  // there, the same way this form always required a separate save click) — so `editing`
  // mode only ever shows the CV/photo block, leading the Profile view itself rather than
  // living in Settings.
  let {
    profile,
    hasCv,
    uploadedAt = null,
    onSaved,
    onCvUploaded,
    onCvDeleted,
  }: {
    profile: UserProfile | null;
    hasCv: boolean;
    /** When the stored CV was uploaded (ISO). Shown in the uploaded-state box instead of
     *  the bare "CV on file" a candidate had no way to act on — a date at least answers
     *  "is this current". */
    uploadedAt?: string | null;
    onSaved?: () => void;
    onCvUploaded?: () => void;
    /** Fired after the stored CV is deleted, so the parent can refresh `hasCv`/the
     *  résumé meta it reads elsewhere on the page. */
    onCvDeleted?: () => void;
  } = $props();

  const uploadedAtLabel = $derived(
    uploadedAt
      ? `Uploaded ${new Date(uploadedAt).toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: 'numeric' })}`
      : 'CV on file',
  );

  const editing = $derived(profile !== null);

  // Seed the fields once from the profile prop (null on first mount, i.e. always for
  // set-up — this whole block only renders pre-profile). Local, unsaved state until the
  // form's own Save.
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

  // Location & work preferences — the optional "where & how I want to work" block, held via
  // LocationPreferencesFields' own internal state and only reassembled here on change.
  // svelte-ignore state_referenced_locally
  let location = $state<LocationPreferences | null>(profile?.location_preferences ?? null);

  // Résumé upload → skill extraction. The server also stores the CV (used by the CV
  // readiness tab); the returned slugs merge (union) into the skills field without
  // wiping manual entries. They persist to the profile only on Save.
  let resumeBusy = $state(false);
  let resumeError = $state<string | null>(null);
  let resumeNote = $state<string | null>(null);
  let fileInput = $state<HTMLInputElement | null>(null);
  let dragActive = $state(false);

  const canSubmit = $derived(specializations.length > 0 && skills.length > 0);

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

  let confirmDeleteCvOpen = $state(false);
  let deletingCv = $state(false);

  async function deleteCv() {
    deletingCv = true;
    resumeError = null;
    resumeNote = null;
    try {
      await api.deleteResume();
      onCvDeleted?.();
    } catch (err) {
      resumeError = err instanceof ApiError ? err.message : 'Could not delete the CV. Please try again.';
    } finally {
      deletingCv = false;
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
    if (resumeBusy) return; // the box is a plain div, not a disabled button — enforce it here
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

  async function submit(e: SubmitEvent) {
    e.preventDefault();
    if (!canSubmit || busy) return;
    busy = true;
    formError = null;
    try {
      await profileStore.save(specializations, skills, excludedSkills, location);
      onSaved?.();
    } catch (err) {
      formError =
        err instanceof ApiError ? err.message : 'Could not save the profile. Please try again.';
    } finally {
      busy = false;
    }
  }
</script>

<div class="flex flex-col gap-6">
  <!-- Your photo: one headshot per member, printed by the CV templates that show one.
       Renders nothing when object storage is unconfigured. -->
  <HeadshotField />

  <!-- Your CV: the same dashed drop-zone box in both states — empty (drop/choose a PDF)
       or with one on file (uploaded indicator + Replace/Delete), so the box the candidate
       first uploaded to is still where they manage it, not a bare row that replaces it.
       A div, not a button, once a CV exists: Replace and Delete are their own buttons,
       which can't nest inside one. Drag-to-replace still works on the box either way.
       Uploading extracts skills into the fields below during set-up; once a profile
       exists, it merges straight into the profile (Roles/Skills have their own views by
       then). -->
  <div class="flex flex-col gap-1.5">
    <span class="text-sm font-medium">Your CV</span>
    <input
      type="file"
      accept="application/pdf,.pdf"
      class="hidden"
      bind:this={fileInput}
      onchange={onResumeFile}
    />
    <!-- A drop-target enhancement over controls (the buttons below) that are already
         fully keyboard-accessible on their own. -->
    <!-- svelte-ignore a11y_no_static_element_interactions -->
    <div
      ondragover={(e) => {
        e.preventDefault();
        if (!resumeBusy) dragActive = true;
      }}
      ondragleave={(e) => {
        e.preventDefault();
        dragActive = false;
      }}
      ondrop={onDrop}
      class="flex items-center justify-center gap-3 rounded-xl border-2 border-dashed px-6 py-8 text-center transition-colors {dragActive
        ? 'border-brand bg-brand/5'
        : 'border-border'}"
    >
      {#if hasCv}
        <span class="flex size-11 items-center justify-center rounded-full bg-brand text-brand-foreground">
          <Check class="size-5" />
        </span>
        <span class="flex flex-col items-start gap-1.5 text-left">
          <span class="text-sm font-semibold">{resumeBusy ? 'Analyzing…' : uploadedAtLabel}</span>
          <span class="flex items-center gap-2">
            <Button variant="outline" size="sm" disabled={resumeBusy} onclick={() => fileInput?.click()}>
              Replace
            </Button>
            <Button
              variant="ghost"
              size="sm"
              disabled={resumeBusy || deletingCv}
              onclick={() => (confirmDeleteCvOpen = true)}
              class="text-muted-foreground hover:bg-destructive/10 hover:text-destructive"
            >
              <Trash2 class="size-4" />
              Delete
            </Button>
          </span>
        </span>
      {:else}
        <button
          type="button"
          onclick={() => fileInput?.click()}
          disabled={resumeBusy}
          class="flex items-center gap-3 disabled:opacity-70"
        >
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
        </button>
      {/if}
    </div>
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

  {#if !editing}
    <!-- Set-up only, one flat scroll — no sub-tabs. The server requires at least one skill
         and one specialization to create a profile, so both are asked here; once a profile
         exists Roles/Skills/Location move to their own views, autosaving there instead of
         behind a Save button. -->
    <form onsubmit={submit} class="flex flex-col gap-6 border-t border-border pt-6">
      <SkillsPicker {skills} {excludedSkills} onToggleSkill={toggleSkill} onToggleExcluded={toggleExcludedSkill} />

      <div class="flex flex-col gap-2">
        <div class="flex items-baseline justify-between">
          <span class="text-sm font-medium">Roles</span>
          <span class="text-xs tabular-nums text-muted-foreground">
            {specializations.length}/{MAX_SPECIALIZATIONS}
          </span>
        </div>
        <SearchSelect
          options={CATEGORY_OPTIONS}
          include={specializations}
          placeholder="Search specializations"
          onToggle={toggleSpecialization}
          clearOnSelect
        />
      </div>

      <div class="flex flex-col gap-2">
        <span class="text-sm font-medium">Location & work</span>
        <LocationPreferencesFields
          value={location}
          derivedLocation={profile?.derived_location}
          onChange={(next) => (location = next)}
        />
      </div>

      {#if formError}
        <p class="text-sm text-destructive">{formError}</p>
      {/if}

      <div class="flex items-center gap-3 border-t border-border pt-4">
        <Button variant="primary" type="submit" disabled={!canSubmit || busy}>
          {busy ? 'Saving…' : 'Create profile'}
        </Button>
        {#if !canSubmit}
          <span class="text-xs text-muted-foreground">Add a role and at least one skill to save.</span>
        {/if}
      </div>
    </form>
  {/if}
</div>

<ConfirmDialog
  bind:open={confirmDeleteCvOpen}
  title="Delete your CV?"
  description="Removes the stored file and its parsed profile. Your contact info stays."
  confirmLabel="Delete"
  variant="destructive"
  onConfirm={deleteCv}
/>
