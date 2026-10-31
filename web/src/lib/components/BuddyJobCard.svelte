<script lang="ts">
  import { ArrowUpRight } from '@lucide/svelte';
  import { resolve } from '$app/paths';
  import type { BuddyFeedItem } from '#lib/types.js';
  import BuddyCompanyLogo from './BuddyCompanyLogo.svelte';

  // The posting a feed event is about, drawn like a link preview: the company's mark, the
  // job title, the company and where the job is. It links to the job while the posting
  // still has a page; an event whose posting is gone links to the company instead.
  let { item }: { item: BuddyFeedItem } = $props();

  const where = $derived([item.company.name, item.job_location].filter(Boolean).join(' · '));
</script>

{#snippet body()}
  <BuddyCompanyLogo name={item.company.name} size="lg" />
  <span class="flex min-w-0 flex-1 flex-col">
    <span class="truncate font-semibold group-hover:underline group-hover:underline-offset-2">{item.job_title ?? item.company.name}</span>
    <span class="truncate text-sm text-muted-foreground">{where}</span>
  </span>
  <span class="flex shrink-0 items-center gap-1 text-sm text-muted-foreground group-hover:text-foreground">
    {item.job_slug ? 'View job' : 'Company'}<ArrowUpRight class="size-4" aria-hidden="true" />
  </span>
{/snippet}

{#if item.job_slug}
  <a href={resolve('/jobs/[slug]', { slug: item.job_slug })} class="group flex items-center gap-3 rounded-lg bg-muted px-3 py-2.5 transition-colors hover:bg-accent">
    {@render body()}
  </a>
{:else}
  <a href={resolve('/companies/[slug]', { slug: item.company.slug })} class="group flex items-center gap-3 rounded-lg bg-muted px-3 py-2.5 transition-colors hover:bg-accent">
    {@render body()}
  </a>
{/if}
