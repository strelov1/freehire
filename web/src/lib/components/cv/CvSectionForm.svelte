<script lang="ts">
  import { Plus, Trash2 } from '@lucide/svelte';
  import { Button, Input } from '#lib/ui/index.js';
  import type { Document } from '#lib/generated/contracts.js';
  import {
    blankExperience,
    blankEducation,
    blankSkillGroup,
    blankLanguage,
    blankProject,
    blankCertification,
  } from '#lib/cv.js';
  import StringListEditor from './StringListEditor.svelte';
  import PeriodDateInput from '#lib/components/PeriodDateInput.svelte';

  // The controlled section form for one CV: binds directly to a caller-owned Document (and CV
  // title) and adds/removes rows per section. It does no data-fetching and no saving — the host
  // owns load/persist. The server sanitizes on save (bounds, caps, drops empty rows), so this
  // form does not police emptiness. Each section iterates `as entry` and binds to the entry's
  // fields — Svelte proxies each element of the bound array, so edits flow back without
  // index-based access. Used in the tailoring workspace's Editor tab against a document the page
  // owns, so the centre preview re-renders live from the same object.
  let {
    doc = $bindable(),
    title = $bindable(),
  }: { doc: Document; title: string } = $props();
  const uid = $props.id();

  // Section row helpers: append a blank row / drop one by index. `doc` is reassigned (a bound
  // prop), so the structural change flows back to the host's state.
  const push =
    <T,>(key: keyof Document, make: () => T) =>
    () => {
      doc = { ...doc, [key]: [...((doc[key] as T[] | undefined) ?? []), make()] };
    };
  const removeAt = (key: keyof Document, i: number) => {
    doc = { ...doc, [key]: ((doc[key] as unknown[] | undefined) ?? []).filter((_, idx) => idx !== i) };
  };
</script>

<div class="space-y-8">
  <!-- Meta -->
  <section class="space-y-3">
    <label for="cv-title" class="block text-sm font-medium">CV title</label>
    <Input id="cv-title" bind:value={title} placeholder="e.g. Backend Engineer — general" class="w-full" />
  </section>

  <!-- Header -->
  <section class="space-y-3">
    <h2 class="text-lg font-semibold">Header</h2>
    <div class="grid gap-3 sm:grid-cols-2 [&>*]:w-full">
      <Input bind:value={doc.header.full_name} placeholder="Full name" aria-label="Full name" />
      <Input bind:value={doc.header.email} placeholder="Email" aria-label="Email" />
      <Input bind:value={doc.header.phone} placeholder="Phone" aria-label="Phone" />
      <Input bind:value={doc.header.location} placeholder="Location" aria-label="Location" />
    </div>
    <div>
      <p class="mb-1 text-sm text-muted-foreground">Links</p>
      <StringListEditor bind:items={doc.header.links as string[]} placeholder="https://…" addLabel="Add link" label="Link" />
    </div>
  </section>

  <!-- Summary (the tagline shown under the name) -->
  <section class="space-y-2">
    <h2 id="{uid}-summary-heading" class="text-lg font-semibold">Summary</h2>
    <textarea
      bind:value={doc.summary}
      aria-labelledby="{uid}-summary-heading"
      rows="3"
      placeholder="One or two lines shown under your name."
      class="w-full rounded-lg border border-input bg-transparent p-3 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50"
    ></textarea>
  </section>

  <!-- Experience -->
  <section class="space-y-4">
    <div class="flex items-center justify-between">
      <h2 class="text-lg font-semibold">Experience</h2>
      <Button variant="outline" size="sm" onclick={push('experience', blankExperience)}>
        <Plus class="mr-1 h-4 w-4" /> Add role
      </Button>
    </div>
    {#each doc.experience ?? [] as entry, i (entry)}
      <div class="space-y-3 rounded-lg border border-border p-4">
        <div class="flex items-start justify-between gap-2">
          <div class="grid flex-1 gap-3 sm:grid-cols-2 [&>*]:w-full">
            <Input bind:value={entry.role} placeholder="Role" aria-label="Role {i + 1}: role" />
            <Input bind:value={entry.company} placeholder="Company" aria-label="Role {i + 1}: company" />
            <Input bind:value={entry.location} placeholder="Location" aria-label="Role {i + 1}: location" />
            <div class="grid grid-cols-2 gap-3 [&>*]:w-full">
              <PeriodDateInput bind:value={entry.start} placeholder="Start" label="Role {i + 1}: start" />
              <PeriodDateInput bind:value={entry.end} placeholder="End" label="Role {i + 1}: end" />
            </div>
          </div>
          <Button variant="ghost" size="icon" aria-label="Remove role" onclick={() => removeAt('experience', i)}>
            <Trash2 class="h-4 w-4" />
          </Button>
        </div>
        <Input bind:value={entry.summary} placeholder="One-line company/role context (optional)" aria-label="Role {i + 1}: context" class="w-full" />
        <div>
          <p class="mb-1 text-sm text-muted-foreground">Bullets</p>
          <StringListEditor bind:items={entry.bullets as string[]} placeholder="Achievement or responsibility" addLabel="Add bullet" label="Role {i + 1} bullet" />
        </div>
        <div>
          <p class="mb-1 text-sm text-muted-foreground">Stack</p>
          <StringListEditor bind:items={entry.stack as string[]} placeholder="e.g. Go" addLabel="Add technology" label="Role {i + 1} technology" />
        </div>
      </div>
    {/each}
  </section>

  <!-- Education -->
  <section class="space-y-4">
    <div class="flex items-center justify-between">
      <h2 class="text-lg font-semibold">Education</h2>
      <Button variant="outline" size="sm" onclick={push('education', blankEducation)}>
        <Plus class="mr-1 h-4 w-4" /> Add education
      </Button>
    </div>
    {#each doc.education ?? [] as entry, i (entry)}
      <div class="flex items-start justify-between gap-2 rounded-lg border border-border p-4">
        <div class="grid flex-1 gap-3 sm:grid-cols-2 [&>*]:w-full">
          <Input bind:value={entry.institution} placeholder="Institution" aria-label="Education {i + 1}: institution" />
          <Input bind:value={entry.degree} placeholder="Degree" aria-label="Education {i + 1}: degree" />
          <Input bind:value={entry.field} placeholder="Field" aria-label="Education {i + 1}: field of study" />
          <div class="grid grid-cols-2 gap-3 [&>*]:w-full">
            <PeriodDateInput bind:value={entry.start} placeholder="Start" label="Education {i + 1}: start" />
            <PeriodDateInput bind:value={entry.end} placeholder="End" label="Education {i + 1}: end" />
          </div>
        </div>
        <Button variant="ghost" size="icon" aria-label="Remove education" onclick={() => removeAt('education', i)}>
          <Trash2 class="h-4 w-4" />
        </Button>
      </div>
    {/each}
  </section>

  <!-- Skills -->
  <section class="space-y-4">
    <div class="flex items-center justify-between">
      <h2 class="text-lg font-semibold">Skills</h2>
      <Button variant="outline" size="sm" onclick={push('skills', blankSkillGroup)}>
        <Plus class="mr-1 h-4 w-4" /> Add group
      </Button>
    </div>
    {#each doc.skills ?? [] as entry, i (entry)}
      <div class="space-y-3 rounded-lg border border-border p-4">
        <div class="flex items-center justify-between gap-2">
          <Input bind:value={entry.group} placeholder="Group (e.g. Languages)" aria-label="Skill group {i + 1}: name" class="flex-1" />
          <Button variant="ghost" size="icon" aria-label="Remove group" onclick={() => removeAt('skills', i)}>
            <Trash2 class="h-4 w-4" />
          </Button>
        </div>
        <StringListEditor bind:items={entry.items as string[]} placeholder="e.g. Go" addLabel="Add skill" label="Skill group {i + 1} skill" />
      </div>
    {/each}
  </section>

  <!-- Languages -->
  <section class="space-y-4">
    <div class="flex items-center justify-between">
      <h2 class="text-lg font-semibold">Languages</h2>
      <Button variant="outline" size="sm" onclick={push('languages', blankLanguage)}>
        <Plus class="mr-1 h-4 w-4" /> Add language
      </Button>
    </div>
    {#each doc.languages ?? [] as entry, i (entry)}
      <div class="flex items-center gap-2">
        <Input bind:value={entry.name} placeholder="Language" aria-label="Language {i + 1}: name" class="flex-1" />
        <Input bind:value={entry.level} placeholder="Level (e.g. C1)" aria-label="Language {i + 1}: level" class="flex-1" />
        <Button variant="ghost" size="icon" aria-label="Remove language" onclick={() => removeAt('languages', i)}>
          <Trash2 class="h-4 w-4" />
        </Button>
      </div>
    {/each}
  </section>

  <!-- Projects -->
  <section class="space-y-4">
    <div class="flex items-center justify-between">
      <h2 class="text-lg font-semibold">Projects</h2>
      <Button variant="outline" size="sm" onclick={push('projects', blankProject)}>
        <Plus class="mr-1 h-4 w-4" /> Add project
      </Button>
    </div>
    {#each doc.projects ?? [] as entry, i (entry)}
      <div class="space-y-3 rounded-lg border border-border p-4">
        <div class="flex items-center gap-2">
          <Input bind:value={entry.name} placeholder="Project name" aria-label="Project {i + 1}: name" class="flex-1" />
          <Input bind:value={entry.link} placeholder="Link" aria-label="Project {i + 1}: link" class="flex-1" />
          <Button variant="ghost" size="icon" aria-label="Remove project" onclick={() => removeAt('projects', i)}>
            <Trash2 class="h-4 w-4" />
          </Button>
        </div>
        <StringListEditor bind:items={entry.bullets as string[]} placeholder="What it does / your role" addLabel="Add bullet" label="Project {i + 1} bullet" />
      </div>
    {/each}
  </section>

  <!-- Certifications -->
  <section class="space-y-4">
    <div class="flex items-center justify-between">
      <h2 class="text-lg font-semibold">Certifications</h2>
      <Button variant="outline" size="sm" onclick={push('certifications', blankCertification)}>
        <Plus class="mr-1 h-4 w-4" /> Add certification
      </Button>
    </div>
    {#each doc.certifications ?? [] as entry, i (entry)}
      <div class="flex items-center gap-2">
        <Input bind:value={entry.name} placeholder="Name" aria-label="Certification {i + 1}: name" class="flex-1" />
        <Input bind:value={entry.issuer} placeholder="Issuer" aria-label="Certification {i + 1}: issuer" class="flex-1" />
        <PeriodDateInput bind:value={entry.year} placeholder="Year" label="Certification {i + 1}: year" />
        <Button variant="ghost" size="icon" aria-label="Remove certification" onclick={() => removeAt('certifications', i)}>
          <Trash2 class="h-4 w-4" />
        </Button>
      </div>
    {/each}
  </section>
</div>
