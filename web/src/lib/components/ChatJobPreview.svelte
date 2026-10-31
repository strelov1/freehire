<script lang="ts">
  import { ArrowUpRight } from '@lucide/svelte';
  import { resolve } from '$app/paths';
  import { api } from '#lib/api.js';
  import type { Job } from '#lib/generated/contracts.js';
  import BuddyCompanyLogo from './BuddyCompanyLogo.svelte';

  // A job page a chat message links to, previewed under the message: the company's mark,
  // the title, company and place. Read from the public job endpoint; a slug that does not
  // resolve draws nothing, and the link stays in the message text either way.
  let { slug }: { slug: string } = $props();

  let job = $state<Job | null>(null);

  $effect(() => {
    const current = slug;
    let cancelled = false;
    job = null;
    api.getJob(current).then(
      (j) => { if (!cancelled) job = j; },
      () => {},
    );
    return () => { cancelled = true; };
  });
</script>

{#if job}
  <a href={resolve('/jobs/[slug]', { slug: job.public_slug })} class="group mt-2 flex items-center gap-2.5 rounded-lg bg-card px-2.5 py-2 text-foreground ring-1 ring-border hover:ring-foreground/30">
    <BuddyCompanyLogo name={job.company} size="md" />
    <span class="flex min-w-0 flex-1 flex-col">
      <span class="truncate text-sm font-semibold group-hover:underline group-hover:underline-offset-2">{job.title}</span>
      <span class="truncate text-xs text-muted-foreground">{[job.company, job.location].filter(Boolean).join(' · ')}</span>
    </span>
    <ArrowUpRight class="size-4 shrink-0 text-muted-foreground" aria-hidden="true" />
  </a>
{/if}
