<script lang="ts">
  import { resolve } from '$app/paths';
  import CompanyLogo from './CompanyLogo.svelte';
  import { cardTags, formatSalary } from '$lib/enrichment';
  import { metaDescription } from '$lib/seo';
  import type { Job } from '$lib/types';
  import { Badge } from '$lib/ui';
  import RealityBadge from './RealityBadge.svelte';
  import { timeAgo } from '$lib/utils';
  import { hasViewed } from '$lib/viewedJobs.svelte';

  // Single source of truth for how a job appears in any list (jobs list and
  // company detail). The whole card is a link to the job detail.
  //
  // `dimViewed` dims the card when the signed-in user has already viewed this
  // job, so the browse list shows what's been seen. The My Jobs surfaces (where
  // every card is viewed by definition) pass `dimViewed={false}` to opt out.
  let { job, dimViewed = true }: { job: Job; dimViewed?: boolean } = $props();

  const isViewed = $derived(dimViewed && hasViewed(job.public_slug));

  const tags = $derived(cardTags(job));
  // A one-line blurb under the title so a card conveys what the job is without
  // opening it. Prefer the clean model-written summary, but only tech jobs are
  // enriched — fall back to a plain-text snippet of the raw (HTML) posting so
  // non-tech jobs still show a description. metaDescription strips the tags.
  const blurb = $derived(job.enrichment?.summary || metaDescription(job.description ?? '', 220));
  const salary = $derived(job.enrichment ? formatSalary(job.enrichment) : null);
  // Top-level `skills` is the served (deterministic-dictionary) facet; the raw
  // `enrichment.skills` is kept in the JSONB and NOT served, so it's always absent
  // here — read the dictionary field so the card's skill chips actually populate.
  const skills = $derived(job.skills ?? []);
  // How recently it was posted is a key signal, so it leads the header.
  const posted = $derived(timeAgo(job.posted_at));

  const MAX_SKILLS = 5;
  const shownSkills = $derived(skills.slice(0, MAX_SKILLS));
  const extraSkills = $derived(skills.length - MAX_SKILLS);
</script>

<a
  href={resolve('/jobs/[slug]', { slug: job.public_slug })}
  class="block rounded-xl border border-border bg-card p-4 transition hover:border-foreground/15 hover:bg-accent hover:opacity-100"
  class:opacity-60={isViewed}
>
  <!-- Company + timestamp rail: a quiet eyebrow that yields the stage to the title.
       The name truncates to a single line, so a long company (e.g. "Veterinary
       Emergency Group (VEG)") keeps the logo centred and the card rhythm even
       instead of wrapping into a ragged multi-line header. -->
  <div class="flex items-center justify-between gap-3">
    <div class="flex min-w-0 items-center gap-2">
      <CompanyLogo name={job.company} size="size-7" />
      <span class="truncate text-sm font-medium text-muted-foreground">
        {job.company || 'Unknown company'}
      </span>
    </div>
    {#if posted}
      <span class="shrink-0 text-xs tabular-nums text-muted-foreground">{posted}</span>
    {/if}
  </div>

  <!-- The title is the card's hero — a size up from the body with tight leading, so
       the eye lands on the role first. -->
  <h3 class="mt-2.5 line-clamp-2 text-lg font-semibold leading-snug tracking-tight sm:text-[1.35rem]">
    {job.title}
  </h3>

  <!-- Signal row: reality chip + the region/employment facets, grouped under the
       title as quiet outline chips so they read as metadata, not decoration. -->
  {#if job.reality || tags.length > 0}
    <div class="mt-2 flex flex-wrap items-center gap-1.5">
      <RealityBadge reality={job.reality} />
      {#each tags as tag (tag)}
        <Badge variant="outline">{tag}</Badge>
      {/each}
    </div>
  {/if}

  {#if blurb}
    <p class="mt-2 line-clamp-2 text-sm text-muted-foreground">{blurb}</p>
  {/if}

  <div class="mt-3 flex items-end justify-between gap-3">
    <div class="flex min-w-0 flex-wrap items-center gap-1.5">
      {#each shownSkills as skill (skill)}
        <Badge variant="secondary">{skill}</Badge>
      {/each}
      {#if extraSkills > 0}
        <span class="text-xs text-muted-foreground">+{extraSkills} skills</span>
      {/if}
    </div>
    {#if salary}
      <span class="shrink-0 text-base font-bold tabular-nums tracking-tight">{salary}</span>
    {/if}
  </div>
</a>
