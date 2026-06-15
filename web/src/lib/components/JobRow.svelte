<script lang="ts">
  import CompanyLogo from './CompanyLogo.svelte';
  import { cardTags, formatSalary } from '$lib/enrichment';
  import type { Job } from '$lib/types';
  import { Badge } from '$lib/ui';
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
  const salary = $derived(job.enrichment ? formatSalary(job.enrichment) : null);
  const skills = $derived(job.enrichment?.skills ?? []);
  // How recently it was posted is a key signal, so it leads the header.
  const posted = $derived(timeAgo(job.posted_at));

  const MAX_SKILLS = 5;
  const shownSkills = $derived(skills.slice(0, MAX_SKILLS));
  const extraSkills = $derived(skills.length - MAX_SKILLS);
</script>

<a
  href={`/jobs/${job.public_slug}`}
  class="block rounded-xl border border-border bg-card p-4 transition hover:bg-accent hover:opacity-100"
  class:opacity-60={isViewed}
>
  <div class="flex items-start justify-between gap-3">
    <div class="flex min-w-0 flex-wrap items-center gap-2">
      <span class="inline-flex items-center gap-1.5 font-semibold">
        <CompanyLogo name={job.company} />
        {job.company || 'Unknown company'}
      </span>
      {#each tags as tag (tag)}
        <Badge variant="secondary">{tag}</Badge>
      {/each}
    </div>
    {#if posted}
      <span class="shrink-0 text-xs text-muted-foreground">{posted}</span>
    {/if}
  </div>

  <h3 class="mt-2 line-clamp-2 text-lg font-semibold tracking-tight">{job.title}</h3>

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
      <span class="shrink-0 text-base font-bold tracking-tight">{salary}</span>
    {/if}
  </div>
</a>
