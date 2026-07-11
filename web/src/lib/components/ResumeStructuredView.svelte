<script lang="ts">
  import { Briefcase, GraduationCap, Languages, Link as LinkIcon, Mail, MapPin, Phone } from '@lucide/svelte';
  import type { ResumeStructured } from '$lib/types';

  // The read-only structured résumé the backend parsed from the uploaded CV (best-effort,
  // via the LLM). Every field is optional — the model omits what the CV does not state —
  // so each section renders only when it has content. This view never edits; it exists so
  // the user can see what the system understood from their CV.
  let { resume }: { resume: ResumeStructured } = $props();

  const experience = $derived(resume.experience ?? []);
  const education = $derived(resume.education ?? []);
  const languages = $derived(resume.languages ?? []);
  const links = $derived(resume.links ?? []);

  // A work/education entry's date range, printed as the CV wrote it ("2021 — Present"),
  // or just the one bound that exists. Empty when the CV stated no dates.
  function dateRange(start?: string, end?: string): string {
    return [start, end].filter(Boolean).join(' — ');
  }
</script>

<div class="flex flex-col gap-6">
  <div class="flex min-w-0 flex-col gap-1">
    <h2 class="text-sm font-semibold uppercase tracking-[0.14em] text-muted-foreground">Parsed from your CV</h2>
    {#if resume.full_name}
      <p class="text-lg font-semibold">{resume.full_name}</p>
    {/if}
    {#if resume.headline}
      <p class="text-sm text-muted-foreground">{resume.headline}</p>
    {/if}
    <div class="mt-1 flex flex-wrap gap-x-4 gap-y-1 text-sm text-muted-foreground">
      {#if resume.location}
        <span class="flex items-center gap-1"><MapPin class="size-3.5" />{resume.location}</span>
      {/if}
      {#if resume.email}
        <span class="flex items-center gap-1"><Mail class="size-3.5" />{resume.email}</span>
      {/if}
      {#if resume.phone}
        <span class="flex items-center gap-1"><Phone class="size-3.5" />{resume.phone}</span>
      {/if}
      {#if resume.total_years}
        <span>{resume.total_years} yrs experience</span>
      {/if}
    </div>
  </div>

  {#if resume.summary}
    <p class="text-sm leading-relaxed">{resume.summary}</p>
  {/if}

  {#if experience.length}
    <section class="flex flex-col gap-3">
      <h3 class="flex items-center gap-2 text-sm font-semibold"><Briefcase class="size-4" />Experience</h3>
      <ul class="flex flex-col gap-3">
        {#each experience as job (`${job.title ?? ''}|${job.company ?? ''}|${job.start ?? ''}`)}
          <li class="flex flex-col gap-1 rounded-xl border border-border bg-card p-4">
            <div class="flex flex-wrap items-baseline justify-between gap-2">
              <span class="text-sm font-semibold">{job.title || job.company}</span>
              {#if dateRange(job.start, job.end)}
                <span class="text-xs text-muted-foreground tabular-nums">{dateRange(job.start, job.end)}</span>
              {/if}
            </div>
            {#if job.title && job.company}
              <span class="text-sm text-muted-foreground">{job.company}</span>
            {/if}
            {#if job.summary}
              <p class="text-sm leading-relaxed">{job.summary}</p>
            {/if}
          </li>
        {/each}
      </ul>
    </section>
  {/if}

  {#if education.length}
    <section class="flex flex-col gap-3">
      <h3 class="flex items-center gap-2 text-sm font-semibold"><GraduationCap class="size-4" />Education</h3>
      <ul class="flex flex-col gap-2">
        {#each education as ed (`${ed.degree ?? ''}|${ed.institution ?? ''}|${ed.year ?? ''}`)}
          <li class="flex flex-wrap items-baseline justify-between gap-2 rounded-xl border border-border bg-card p-4">
            <div class="flex min-w-0 flex-col">
              <span class="text-sm font-semibold">{ed.degree || ed.institution}</span>
              {#if ed.degree && ed.institution}
                <span class="text-sm text-muted-foreground">{ed.institution}</span>
              {/if}
            </div>
            {#if ed.year}
              <span class="text-xs text-muted-foreground tabular-nums">{ed.year}</span>
            {/if}
          </li>
        {/each}
      </ul>
    </section>
  {/if}

  {#if languages.length}
    <section class="flex flex-col gap-2">
      <h3 class="flex items-center gap-2 text-sm font-semibold"><Languages class="size-4" />Languages</h3>
      <div class="flex flex-wrap gap-2">
        {#each languages as lang (lang)}
          <span class="rounded-full border border-border bg-secondary px-3 py-1 text-xs">{lang}</span>
        {/each}
      </div>
    </section>
  {/if}

  {#if links.length}
    <section class="flex flex-col gap-2">
      <h3 class="flex items-center gap-2 text-sm font-semibold"><LinkIcon class="size-4" />Links</h3>
      <ul class="flex flex-col gap-1">
        {#each links as link (link)}
          <li>
            <a href={link} target="_blank" rel="noopener noreferrer" class="text-sm text-primary hover:underline break-all">{link}</a>
          </li>
        {/each}
      </ul>
    </section>
  {/if}
</div>
