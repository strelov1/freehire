<script lang="ts">
  import { Award, Briefcase, FolderKanban, GraduationCap, Languages, Tags, User } from '@lucide/svelte';
  import Seo from '$lib/components/Seo.svelte';
  import type { Experience } from '$lib/generated/contracts';
  import { companyLogoUrl } from '$lib/logo';
  import { EntityLogo } from '$lib/ui';
  import type { PageData } from './$types';

  let { data }: { data: PageData } = $props();
  const profile = $derived(data.profile);
  const cv = $derived(profile.cv);

  // The skill currently hovered in the sidebar, so work-history entries that don't carry
  // it in their own stack fade out — a quick "where did they use this" scan.
  let hoveredSkill = $state<string | null>(null);

  // Setting hoveredSkill triggers a scrollIntoView (below) that carries the sidebar
  // itself — not yet `sticky`-docked at scroll position 0 — out from under a stationary
  // cursor. Chromium recomputes hover state against the moved layout mid-scroll, so a
  // synthetic mouseenter/mouseleave can land on a *different* chip than the one the
  // pointer is actually resting over, or on empty space. While that self-inflicted
  // scroll is in flight, mouse-driven hover changes are ignored outright — keyboard
  // focus (unaffected by scroll-induced reflow) still updates immediately.
  let autoScrolling = false;
  let settleTimer: ReturnType<typeof setTimeout> | undefined;

  function setHovered(skill: string) {
    if (autoScrolling) return;
    hoveredSkill = skill;
  }

  function clearHovered() {
    if (autoScrolling) return;
    hoveredSkill = null;
  }

  // Case-insensitive, either-direction match: a skill chip comes from the canonical
  // dictionary ("Node.js"), a job's stack is free text off the résumé parser and can
  // differ in casing or specificity, so a strict `===` would miss real matches.
  function jobHasSkill(job: Experience, skill: string): boolean {
    const needle = skill.toLowerCase();
    return (job.stack ?? []).some((tech) => {
      const hay = tech.toLowerCase();
      return hay === needle || hay.includes(needle) || needle.includes(hay);
    });
  }

  function fadeClass(job: Experience): string {
    return hoveredSkill && !jobHasSkill(job, hoveredSkill) ? 'opacity-50 blur-sm' : '';
  }

  function chipClass(skill: string): string {
    return skill === hoveredSkill
      ? 'bg-brand text-brand-foreground'
      : 'bg-brand-muted text-brand-strong';
  }

  // One <li> ref per work-history entry, indexed like `experience` — a hover needs to
  // scroll straight to the first match, which can otherwise sit off-screen above or
  // below the visible list.
  let jobEls: (HTMLLIElement | null)[] = [];

  const experience = $derived(cv.experience ?? []);
  const education = $derived(cv.education ?? []);
  const languages = $derived(cv.languages ?? []);
  const certifications = $derived(cv.certifications ?? []);
  const projects = $derived(cv.projects ?? []);
  // Top-level facets (specializations + canonical skills from the profile), distinct
  // from cv.skills — the free-text skills the résumé parser found. Only the facets are
  // shown here: they're the curated list, the résumé-parsed one is redundant with it.
  // Deduped: specializations and skills are separate vocabularies that CAN genuinely
  // overlap (e.g. "devops" exists in both), and the {#each} below keys on the literal
  // string — a duplicate key throws during Svelte 5 hydration rather than just warning,
  // silently breaking the whole page's interactivity for that visitor.
  const skillChips = $derived([
    ...new Set([...(profile.specializations ?? []), ...(profile.skills ?? [])]),
  ]);

  // A hover jumps straight to the first matching entry — otherwise the blur alone gives
  // no clue whether the match is above or below the current scroll position.
  $effect(() => {
    const skill = hoveredSkill;
    if (!skill) return;
    const el = jobEls[experience.findIndex((job) => jobHasSkill(job, skill))];
    if (!el) return;
    autoScrolling = true;
    clearTimeout(settleTimer);
    settleTimer = setTimeout(() => (autoScrolling = false), 800);
    el.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
  });

  // full_name is present only in "public" mode — the backend omits the key entirely for
  // "anonymous" (see talentNetworkProfileResponse's doc comment), so there is nothing to
  // accidentally render here; no placeholder like "Anonymous Candidate" is needed.
  const heading = $derived(profile.full_name || 'Talent Network profile');

  // Avatar initials, derived only when full_name is present (public mode) — anonymous
  // mode never reaches this because the template falls back to the generic icon.
  const initials = $derived(
    (profile.full_name ?? '')
      .trim()
      .split(/\s+/)
      .filter(Boolean)
      .slice(0, 2)
      .map((part) => part[0]?.toUpperCase() ?? '')
      .join(''),
  );

  const pageTitle = $derived(`${heading} — freehire Talent Network`);
  const description = $derived(
    cv.headline || 'A candidate profile shared via freehire’s Talent Network.',
  );

  // A work/education entry's date range, printed as the CV wrote it ("2021 — Present").
  function dateRange(start?: string, end?: string): string {
    return [start, end].filter(Boolean).join(' — ');
  }
</script>

<Seo title={pageTitle} {description} />
<svelte:head>
  <!-- A candidate's private shareable link, not meant for search discovery. -->
  <meta name="robots" content="noindex" />
</svelte:head>

<div class="mx-auto flex w-full max-w-5xl flex-col gap-6 px-4 py-8">
  <div class="flex flex-col gap-4">
    <div class="flex items-start gap-4">
      <div
        class="flex size-14 shrink-0 items-center justify-center rounded-full bg-secondary text-sm font-semibold text-muted-foreground"
      >
        {#if profile.full_name}
          {initials}
        {:else}
          <User class="size-6" />
        {/if}
      </div>
      <div class="flex min-w-0 flex-col gap-1">
        <h1 class="text-2xl font-semibold tracking-tight">{heading}</h1>
        {#if cv.headline}
          <p class="text-sm text-muted-foreground">{cv.headline}</p>
        {/if}
        <div class="mt-1 flex flex-wrap gap-x-4 gap-y-1 text-sm text-muted-foreground">
          {#if cv.location}
            <span>{cv.location}</span>
          {/if}
          {#if cv.total_years}
            <span>{cv.total_years} yrs experience</span>
          {/if}
        </div>
      </div>
    </div>

    {#if cv.summary}
      <p class="text-sm leading-relaxed">{cv.summary}</p>
    {/if}
  </div>

  <hr class="border-border" />

  <div class="flex flex-col gap-6 lg:flex-row lg:items-start">
    <!-- Skills move into a dedicated sidebar (rather than a wall of chips under the
         header) since a well-rounded senior profile can carry 50+ of them — inline they
         pushed the actual work history below the fold. -->
    {#if skillChips.length}
      <aside class="w-full shrink-0 lg:order-2 lg:w-72">
        <div class="flex flex-col gap-3 rounded-xl border border-border bg-card p-4 lg:sticky lg:top-6">
          <h2 class="flex items-center gap-2 text-sm font-semibold"><Tags class="size-4" />Skills</h2>
          <div class="flex flex-wrap gap-2" aria-label="Skills">
            {#each skillChips as skill (skill)}
              <span
                role="button"
                tabindex="0"
                onmouseenter={() => setHovered(skill)}
                onmouseleave={clearHovered}
                onfocus={() => (hoveredSkill = skill)}
                onblur={() => (hoveredSkill = null)}
                class="rounded-full px-3 py-1 text-xs font-medium transition-colors {chipClass(
                  skill,
                )}"
                >{skill}</span
              >
            {/each}
          </div>
        </div>
      </aside>
    {/if}

    <div class="flex min-w-0 flex-1 flex-col gap-6">
      {#if experience.length}
        <section class="flex flex-col gap-3">
          <h2 class="flex items-center gap-2 text-sm font-semibold"><Briefcase class="size-4" />Work history</h2>
          <ul class="flex flex-col gap-3">
            <!-- Keyed on index, not on job's fields: two entries can legitimately share
                 title|company|start (differing only in summary/location), and a composite
                 string key colliding throws during Svelte 5 hydration. This list is static
                 once loaded and never reorders, so an index key is safe here. -->
            {#each experience as job, i (i)}
              <li
                bind:this={jobEls[i]}
                class="flex gap-3 rounded-xl border border-border bg-card p-4 transition duration-150 {fadeClass(
                  job,
                )}"
              >
                {#if job.company}
                  <EntityLogo
                    name={job.company}
                    src={companyLogoUrl(job.company) ?? undefined}
                    shape="square"
                    size="md"
                    class="shrink-0"
                  />
                {:else}
                  <div class="size-10 shrink-0 rounded-lg bg-secondary"></div>
                {/if}
                <div class="flex min-w-0 flex-1 flex-col gap-1">
                  <div class="flex flex-wrap items-baseline justify-between gap-2">
                    <span class="text-sm font-semibold">{job.title || job.company}</span>
                    {#if dateRange(job.start, job.end)}
                      <span class="text-xs text-muted-foreground tabular-nums"
                        >{dateRange(job.start, job.end)}</span
                      >
                    {/if}
                  </div>
                  {#if job.title && job.company}
                    <span class="text-sm text-muted-foreground">{job.company}</span>
                  {/if}
                  {#if job.summary}
                    <p class="text-sm leading-relaxed">{job.summary}</p>
                  {/if}
                  {#if job.highlights?.length}
                    <ul class="mt-1 flex list-disc flex-col gap-0.5 pl-4 text-sm leading-relaxed">
                      {#each job.highlights as highlight (highlight)}
                        <li>{highlight}</li>
                      {/each}
                    </ul>
                  {/if}
                  {#if job.stack?.length}
                    <div class="mt-1 flex flex-wrap gap-1.5">
                      {#each job.stack as tech (tech)}
                        <span class="rounded-full border border-border bg-secondary px-2 py-0.5 text-xs"
                          >{tech}</span
                        >
                      {/each}
                    </div>
                  {/if}
                </div>
              </li>
            {/each}
          </ul>
        </section>
      {/if}

      {#if education.length}
        <section class="flex flex-col gap-3">
          <h2 class="flex items-center gap-2 text-sm font-semibold"><GraduationCap class="size-4" />Education</h2>
          <ul class="flex flex-col gap-2">
            <!-- Same collision risk and same fix as the experience list above: two entries
                 can share degree|institution|year, so key on index instead. -->
            {#each education as ed, i (i)}
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

      {#if projects.length}
        <section class="flex flex-col gap-3">
          <h2 class="flex items-center gap-2 text-sm font-semibold"><FolderKanban class="size-4" />Projects</h2>
          <ul class="flex flex-col gap-2">
            {#each projects as project (project.name ?? project.link ?? '')}
              <li class="flex flex-col gap-1 rounded-xl border border-border bg-card p-4">
                {#if project.link}
                  <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- a project's own portfolio/repo link, not an internal route -->
                  <a href={project.link} target="_blank" rel="noopener noreferrer" class="text-sm font-semibold text-primary hover:underline">{project.name || project.link}</a>
                {:else if project.name}
                  <span class="text-sm font-semibold">{project.name}</span>
                {/if}
                {#if project.highlights?.length}
                  <ul class="flex list-disc flex-col gap-0.5 pl-4 text-sm leading-relaxed">
                    {#each project.highlights as highlight (highlight)}
                      <li>{highlight}</li>
                    {/each}
                  </ul>
                {/if}
              </li>
            {/each}
          </ul>
        </section>
      {/if}

      {#if languages.length}
        <section class="flex flex-col gap-2">
          <h2 class="flex items-center gap-2 text-sm font-semibold"><Languages class="size-4" />Languages</h2>
          <div class="flex flex-wrap gap-2">
            {#each languages as lang (lang)}
              <span class="rounded-full border border-border bg-secondary px-3 py-1 text-xs">{lang}</span>
            {/each}
          </div>
        </section>
      {/if}

      {#if certifications.length}
        <section class="flex flex-col gap-2">
          <h2 class="flex items-center gap-2 text-sm font-semibold"><Award class="size-4" />Certifications</h2>
          <div class="flex flex-wrap gap-2">
            {#each certifications as cert (cert)}
              <span class="rounded-full border border-border bg-secondary px-3 py-1 text-xs">{cert}</span>
            {/each}
          </div>
        </section>
      {/if}
    </div>
  </div>

  {#if !experience.length && !education.length && !skillChips.length && !cv.summary && !certifications.length && !languages.length && !projects.length}
    <p class="text-sm text-muted-foreground">This candidate hasn't added CV details yet.</p>
  {/if}
</div>
