<script lang="ts">
  // Education, Languages, and Certifications — all candidate-owned, editable, and
  // saved through the same owned-overlay PUT /me/resume/contacts as
  // CvSummaryCard/CandidateContactsEditor (see internal/resume/owned.go). Grouped here
  // because that's where a candidate looks for them, next to Education.
  //
  // A PUT replaces the whole owned block, so every save goes through
  // resumeStore.saveContacts, which queues the block's writers and builds each save from
  // the block the one before it saved.
  //
  // Everything autosaves (TextAutosave), each section with its own status line. The one
  // deliberate exception is a NEW education entry: it is typed into its own row and
  // joins the list only on "Add", so half an entry is never saved (#3879).
  import { untrack } from 'svelte';
  import { Award, GraduationCap, Languages as LanguagesIcon, Plus, X } from '@lucide/svelte';
  import { resumeStore } from '#lib/resume.svelte.js';
  import { SaveStatus } from '#lib/saveStatus.svelte.js';
  import { TextAutosave } from '#lib/textAutosave.js';
  import type { ResumeEducation, ResumeStructured } from '#lib/types.js';
  import { Button } from '#lib/ui/index.js';
  import PeriodDateInput from '#lib/components/PeriodDateInput.svelte';
  import SaveStatusLine from '../SaveStatus.svelte';
  import { messages } from './EducationCard.messages';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { t } from '#lib/i18n/t.js';

  let { structured }: { structured: ResumeStructured | null } = $props();

  const s = $derived(t(messages, locale()));
  // The languages/certifications textareas take their names from the visible headings.
  const uid = $props.id();
  const languagesHeadingId = `${uid}-languages`;
  const certificationsHeadingId = `${uid}-certifications`;

  // An entry with nothing in it is not an entry; this is the shape that gets saved and
  // compared, so blank rows never count as a change.
  const cleanEducation = (rows: ResumeEducation[]): ResumeEducation[] =>
    rows
      .map((e) => ({ degree: (e.degree ?? '').trim(), institution: (e.institution ?? '').trim(), year: e.year }))
      .filter((e) => e.degree || e.institution || e.year);
  const lines = (text: string) =>
    text
      .split('\n')
      .map((l) => l.trim())
      .filter(Boolean);

  const savedEducation = () => cleanEducation(structured?.education ?? []);
  const savedLanguages = () => structured?.languages ?? [];
  const savedCertifications = () => structured?.certifications ?? [];

  let educationRows = $state<ResumeEducation[]>(savedEducation().map((e) => ({ ...e })));
  let languagesText = $state(savedLanguages().join('\n'));
  let certificationsText = $state(savedCertifications().join('\n'));
  let newEntry = $state<ResumeEducation>({ degree: '', institution: '', year: undefined });

  const educationStatus = new SaveStatus();
  const languagesStatus = new SaveStatus();
  const certificationsStatus = new SaveStatus();

  // Owned even when a list ends up empty — otherwise clearing every row is
  // indistinguishable, server-side, from never having edited it, and the CV's own list
  // reappears on the next read (internal/resume/owned.go).
  const education = new TextAutosave<ResumeEducation[]>({
    status: educationStatus,
    read: () => cleanEducation(educationRows),
    saved: savedEducation,
    save: (value) => resumeStore.saveContacts({ education: value, education_set: true }),
  });
  const languages = new TextAutosave<string[]>({
    status: languagesStatus,
    read: () => lines(languagesText),
    saved: savedLanguages,
    save: (value) => resumeStore.saveContacts({ languages: value, languages_set: true }),
  });
  const certifications = new TextAutosave<string[]>({
    status: certificationsStatus,
    read: () => lines(certificationsText),
    saved: savedCertifications,
    save: (value) => resumeStore.saveContacts({ certifications: value, certifications_set: true }),
  });

  // Follow the saved lists when they change elsewhere (an Undo, a fresh CV parse), each
  // only while it holds nothing unsent. The boxes are read untracked: a keystroke must
  // never re-run this (see CandidateContactsEditor for why that eats the keystroke).
  const same = (a: unknown, b: unknown) => JSON.stringify(a) === JSON.stringify(b);
  $effect(() => {
    const saved = savedEducation();
    if (education.holding) return;
    if (!same(saved, untrack(() => cleanEducation(educationRows)))) educationRows = saved.map((e) => ({ ...e }));
  });
  $effect(() => {
    const saved = savedLanguages();
    if (languages.holding) return;
    if (!same(saved, untrack(() => lines(languagesText)))) languagesText = saved.join('\n');
  });
  $effect(() => {
    const saved = savedCertifications();
    if (certifications.holding) return;
    if (!same(saved, untrack(() => lines(certificationsText)))) certificationsText = saved.join('\n');
  });

  function removeEducationRow(i: number) {
    educationRows = educationRows.filter((_, idx) => idx !== i);
    education.edited();
    education.commit();
  }

  const newEntryFilled = $derived(
    !!(newEntry.degree?.trim() || newEntry.institution?.trim() || newEntry.year),
  );

  function addEducation() {
    if (!newEntryFilled) return;
    educationRows = [...educationRows, { ...newEntry }];
    newEntry = { degree: '', institution: '', year: undefined };
    education.edited();
    education.commit();
  }

  // Edits made right before leaving the page still land, quietly.
  $effect(() => () => {
    education.commit(true);
    languages.commit(true);
    certifications.commit(true);
  });
</script>

<div class="flex flex-col gap-6">
  <div class="flex flex-col gap-2">
    <div class="flex items-center justify-between gap-3">
      <h2 class="flex items-center gap-2 text-base font-semibold">
        <GraduationCap class="size-4.5" />{s.education.heading}
      </h2>
      <SaveStatusLine status={educationStatus} />
    </div>

    <div class="flex flex-col gap-2">
      <!-- Every saved entry is editable in place; an edit saves a second after typing
           stops or when focus leaves the list. -->
      <div
        class="flex flex-col gap-2"
        oninput={() => education.edited()}
        onchange={() => education.edited()}
        onfocusout={() => education.commit()}
      >
        {#each educationRows as row, i (row)}
          <div class="flex items-start gap-2 rounded-lg border border-border p-3">
            <div class="grid flex-1 gap-2 sm:grid-cols-2">
              <input
                class="rounded-md border border-border bg-background px-3 py-2 text-sm"
                bind:value={row.degree}
                placeholder={s.education.degreePlaceholder}
                aria-label="{s.education.degreePlaceholder} {i + 1}"
              />
              <input
                class="rounded-md border border-border bg-background px-3 py-2 text-sm"
                bind:value={row.institution}
                placeholder={s.education.institutionPlaceholder}
                aria-label="{s.education.institutionPlaceholder} {i + 1}"
              />
              <div class="sm:col-span-2">
                <PeriodDateInput
                  bind:value={row.year}
                  placeholder={s.education.yearPlaceholder}
                  label="{s.education.yearPlaceholder} {i + 1}"
                />
              </div>
            </div>
            <Button
              size="icon"
              variant="ghost"
              class="shrink-0 text-muted-foreground"
              onclick={() => removeEducationRow(i)}
              aria-label="{s.remove} {i + 1}"
            >
              <X class="size-4" />
            </Button>
          </div>
        {/each}
      </div>

      <!-- A new entry is not saved until "Add": half an entry (a degree with no school yet)
           is not something to store. -->
      <div class="flex flex-col gap-2 rounded-lg border border-dashed border-border p-3">
        <span class="text-xs text-muted-foreground">{s.education.newEntry}</span>
        <div class="grid gap-2 sm:grid-cols-2">
          <input
            class="rounded-md border border-border bg-background px-3 py-2 text-sm"
            bind:value={newEntry.degree}
            placeholder={s.education.degreePlaceholder}
            aria-label="{s.education.newEntry}: {s.education.degreePlaceholder}"
          />
          <input
            class="rounded-md border border-border bg-background px-3 py-2 text-sm"
            bind:value={newEntry.institution}
            placeholder={s.education.institutionPlaceholder}
            aria-label="{s.education.newEntry}: {s.education.institutionPlaceholder}"
          />
          <div class="sm:col-span-2">
            <PeriodDateInput
              bind:value={newEntry.year}
              placeholder={s.education.yearPlaceholder}
              label="{s.education.newEntry}: {s.education.yearPlaceholder}"
            />
          </div>
        </div>
        <Button size="sm" variant="secondary" class="self-start" disabled={!newEntryFilled} onclick={addEducation}>
          <Plus class="size-3.5" />{s.education.add}
        </Button>
      </div>
    </div>
  </div>

  <div class="flex flex-col gap-2" onfocusout={() => languages.commit()}>
    <div class="flex items-center justify-between gap-3">
      <h2 id={languagesHeadingId} class="flex items-center gap-2 text-base font-semibold">
        <LanguagesIcon class="size-4.5" />{s.languages.heading}
      </h2>
      <SaveStatusLine status={languagesStatus} />
    </div>
    <textarea
      bind:value={languagesText}
      oninput={() => languages.edited()}
      aria-labelledby={languagesHeadingId}
      rows="3"
      class="w-full rounded-md border border-border bg-background px-3 py-2 text-sm"
      placeholder={s.languages.placeholder}
    ></textarea>
  </div>

  <div class="flex flex-col gap-2" onfocusout={() => certifications.commit()}>
    <div class="flex items-center justify-between gap-3">
      <h2 id={certificationsHeadingId} class="flex items-center gap-2 text-base font-semibold">
        <Award class="size-4.5" />{s.certifications.heading}
      </h2>
      <SaveStatusLine status={certificationsStatus} />
    </div>
    <textarea
      bind:value={certificationsText}
      oninput={() => certifications.edited()}
      aria-labelledby={certificationsHeadingId}
      rows="3"
      class="w-full rounded-md border border-border bg-background px-3 py-2 text-sm"
      placeholder={s.certifications.placeholder}
    ></textarea>
  </div>
</div>
