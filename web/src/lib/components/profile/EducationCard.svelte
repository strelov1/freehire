<script lang="ts">
  // Education, Languages, and Certifications — all candidate-owned, editable, and
  // saved through the same owned-overlay PUT /me/resume/contacts as
  // CvSummaryCard/CandidateContactsEditor (see internal/resume/owned.go). Grouped here
  // because that's where a candidate looks for them, next to Education.
  //
  // A PUT replaces the whole owned block, so saving here spreads the current `contacts`
  // object first and only overrides this card's own fields — exactly as the other two
  // editors do for theirs.
  import { Award, GraduationCap, Languages as LanguagesIcon, Pencil, Plus, X } from '@lucide/svelte';
  import { api } from '$lib/api';
  import type { CandidateContacts, ResumeEducation, ResumeStructured } from '$lib/types';
  import { Button, Chip } from '$lib/ui';
  import PeriodDateInput from '$lib/components/PeriodDateInput.svelte';
  import { formatPeriodDate } from '$lib/periodDate';

  let {
    structured,
    contacts = null,
    onSaved,
  }: {
    structured: ResumeStructured | null;
    contacts?: CandidateContacts | null;
    onSaved?: () => void;
  } = $props();

  const education = $derived(structured?.education ?? []);
  const languages = $derived(structured?.languages ?? []);
  const certifications = $derived(structured?.certifications ?? []);

  let editingEducation = $state(false);
  let educationDraft = $state<ResumeEducation[]>([]);
  let educationBusy = $state(false);
  let educationError = $state<string | null>(null);

  function startEditEducation() {
    educationDraft = education.map((e) => ({ ...e }));
    educationError = null;
    editingEducation = true;
  }

  function cancelEditEducation() {
    editingEducation = false;
    educationError = null;
  }

  function addEducationRow() {
    educationDraft = [...educationDraft, { degree: '', institution: '', year: undefined }];
  }

  function removeEducationRow(i: number) {
    educationDraft = educationDraft.filter((_, idx) => idx !== i);
  }

  async function saveEducation() {
    educationBusy = true;
    educationError = null;
    try {
      const cleaned = educationDraft
        .map((e) => ({
          degree: (e.degree ?? '').trim(),
          institution: (e.institution ?? '').trim(),
          year: e.year,
        }))
        .filter((e) => e.degree || e.institution || e.year);
      // Owned even when `cleaned` is empty — otherwise clearing every row here is
      // indistinguishable, server-side, from never having edited Education at all, and the
      // CV's own education reappears on the next read (internal/resume/owned.go).
      await api.putResumeContacts({ ...contacts, education: cleaned, education_set: true });
      editingEducation = false;
      onSaved?.();
    } catch (e) {
      educationError = e instanceof Error ? e.message : 'Could not save.';
    } finally {
      educationBusy = false;
    }
  }

  let editingLanguages = $state(false);
  let languagesText = $state('');
  let languagesBusy = $state(false);
  let languagesError = $state<string | null>(null);

  function startEditLanguages() {
    languagesText = languages.join('\n');
    languagesError = null;
    editingLanguages = true;
  }

  function cancelEditLanguages() {
    editingLanguages = false;
    languagesError = null;
  }

  async function saveLanguages() {
    languagesBusy = true;
    languagesError = null;
    try {
      await api.putResumeContacts({
        ...contacts,
        languages: languagesText
          .split('\n')
          .map((l) => l.trim())
          .filter(Boolean),
        // Owned even when the list above ends up empty — see saveEducation's comment.
        languages_set: true,
      });
      editingLanguages = false;
      onSaved?.();
    } catch (e) {
      languagesError = e instanceof Error ? e.message : 'Could not save.';
    } finally {
      languagesBusy = false;
    }
  }

  let editingCertifications = $state(false);
  let certificationsText = $state('');
  let certificationsBusy = $state(false);
  let certificationsError = $state<string | null>(null);

  function startEditCertifications() {
    certificationsText = certifications.join('\n');
    certificationsError = null;
    editingCertifications = true;
  }

  function cancelEditCertifications() {
    editingCertifications = false;
    certificationsError = null;
  }

  async function saveCertifications() {
    certificationsBusy = true;
    certificationsError = null;
    try {
      await api.putResumeContacts({
        ...contacts,
        certifications: certificationsText
          .split('\n')
          .map((l) => l.trim())
          .filter(Boolean),
        // Owned even when the list above ends up empty — see saveEducation's comment.
        certifications_set: true,
      });
      editingCertifications = false;
      onSaved?.();
    } catch (e) {
      certificationsError = e instanceof Error ? e.message : 'Could not save.';
    } finally {
      certificationsBusy = false;
    }
  }
</script>

<div class="flex flex-col gap-6">
  <div class="flex flex-col gap-2">
    <div class="flex items-center justify-between">
      <h2 class="flex items-center gap-2 text-base font-semibold">
        <GraduationCap class="size-4.5" />Education
      </h2>
      {#if !editingEducation}
        <Button size="sm" variant="ghost" class="text-muted-foreground" onclick={startEditEducation}>
          <Pencil class="size-3.5" />Edit
        </Button>
      {/if}
    </div>

    {#if editingEducation}
      <div class="flex flex-col gap-2">
        {#each educationDraft as row, i (row)}
          <div class="flex flex-col gap-2 rounded-lg border border-border p-3">
            <div class="flex items-start gap-2">
              <div class="grid flex-1 gap-2 sm:grid-cols-2">
                <input
                  class="rounded-md border border-border bg-background px-3 py-2 text-sm"
                  bind:value={row.degree}
                  placeholder="Degree"
                />
                <input
                  class="rounded-md border border-border bg-background px-3 py-2 text-sm"
                  bind:value={row.institution}
                  placeholder="Institution"
                />
                <div class="sm:col-span-2">
                  <PeriodDateInput bind:value={row.year} placeholder="Year" />
                </div>
              </div>
              <Button
                size="icon"
                variant="ghost"
                class="shrink-0 text-muted-foreground"
                onclick={() => removeEducationRow(i)}
                aria-label="Remove"
              >
                <X class="size-4" />
              </Button>
            </div>
          </div>
        {/each}
        <Button size="sm" variant="secondary" class="self-start" onclick={addEducationRow}>
          <Plus class="size-3.5" />Add education
        </Button>
        <div class="flex flex-wrap items-center gap-2">
          <Button size="sm" variant="primary" disabled={educationBusy} onclick={saveEducation}>Save</Button>
          <Button
            size="sm"
            variant="ghost"
            class="text-muted-foreground"
            disabled={educationBusy}
            onclick={cancelEditEducation}
          >
            Cancel
          </Button>
        </div>
        {#if educationError}
          <p class="text-sm text-destructive">{educationError}</p>
        {/if}
      </div>
    {:else if education.length}
      <ul class="flex flex-col gap-1.5">
        {#each education as ed, i (i)}
          <li class="flex flex-wrap items-baseline justify-between gap-2 rounded-xl border border-border bg-card p-4 text-sm">
            <span>
              <span class="font-medium">{ed.degree || ed.institution}</span>
              {#if ed.degree && ed.institution}
                <span class="text-muted-foreground"> · {ed.institution}</span>
              {/if}
            </span>
            {#if ed.year}
              <span class="text-xs tabular-nums text-muted-foreground">{formatPeriodDate(ed.year)}</span>
            {/if}
          </li>
        {/each}
      </ul>
    {:else}
      <p class="text-sm text-muted-foreground">Nothing here yet — add your education.</p>
    {/if}
  </div>

  <div class="flex flex-col gap-2">
    <div class="flex items-center justify-between">
      <h2 class="flex items-center gap-2 text-base font-semibold">
        <LanguagesIcon class="size-4.5" />Languages
      </h2>
      {#if !editingLanguages}
        <Button size="sm" variant="ghost" class="text-muted-foreground" onclick={startEditLanguages}>
          <Pencil class="size-3.5" />Edit
        </Button>
      {/if}
    </div>

    {#if editingLanguages}
      <textarea
        bind:value={languagesText}
        rows="4"
        class="w-full rounded-md border border-border bg-background px-3 py-2 text-sm"
        placeholder="English&#10;Spanish"
      ></textarea>
      <div class="flex flex-wrap items-center gap-2">
        <Button size="sm" variant="primary" disabled={languagesBusy} onclick={saveLanguages}>Save</Button>
        <Button
          size="sm"
          variant="ghost"
          class="text-muted-foreground"
          disabled={languagesBusy}
          onclick={cancelEditLanguages}
        >
          Cancel
        </Button>
      </div>
      {#if languagesError}
        <p class="text-sm text-destructive">{languagesError}</p>
      {/if}
    {:else if languages.length}
      <div class="flex flex-wrap gap-1.5">
        {#each languages as lang, i (i)}
          <Chip>{lang}</Chip>
        {/each}
      </div>
    {:else}
      <p class="text-sm text-muted-foreground">Nothing here yet — add a language.</p>
    {/if}
  </div>

  <div class="flex flex-col gap-2">
    <div class="flex items-center justify-between">
      <h2 class="flex items-center gap-2 text-base font-semibold">
        <Award class="size-4.5" />Certifications
      </h2>
      {#if !editingCertifications}
        <Button size="sm" variant="ghost" class="text-muted-foreground" onclick={startEditCertifications}>
          <Pencil class="size-3.5" />Edit
        </Button>
      {/if}
    </div>

    {#if editingCertifications}
      <textarea
        bind:value={certificationsText}
        rows="4"
        class="w-full rounded-md border border-border bg-background px-3 py-2 text-sm"
        placeholder="AWS Certified Solutions Architect"
      ></textarea>
      <div class="flex flex-wrap items-center gap-2">
        <Button size="sm" variant="primary" disabled={certificationsBusy} onclick={saveCertifications}>
          Save
        </Button>
        <Button
          size="sm"
          variant="ghost"
          class="text-muted-foreground"
          disabled={certificationsBusy}
          onclick={cancelEditCertifications}
        >
          Cancel
        </Button>
      </div>
      {#if certificationsError}
        <p class="text-sm text-destructive">{certificationsError}</p>
      {/if}
    {:else if certifications.length}
      <div class="flex flex-wrap gap-1.5">
        {#each certifications as cert, i (i)}
          <Chip>{cert}</Chip>
        {/each}
      </div>
    {:else}
      <p class="text-sm text-muted-foreground">Nothing here yet — add a certification.</p>
    {/if}
  </div>
</div>
