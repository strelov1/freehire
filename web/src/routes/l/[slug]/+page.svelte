<script lang="ts">
  import Seo from '$lib/components/Seo.svelte';
  import JobRow from '$lib/components/JobRow.svelte';
  import States from '$lib/components/States.svelte';
  import type { PageData } from './$types';

  let { data }: { data: PageData } = $props();
  const list = $derived(data.list);
</script>

<!-- Share-by-link, not meant for search discovery — still gets a rich link-preview
     card when posted to chat/social, via Seo's og:/twitter: tags. -->
<Seo
  title={`${list.name} — freehire`}
  description={list.description || 'A curated job list on freehire.'}
  robots="noindex"
/>

<div class="mx-auto w-full max-w-4xl px-4 py-6">
  <div class="mb-4 flex flex-col gap-1">
    <h1 class="text-2xl font-semibold tracking-tight">{list.name}</h1>
    {#if list.description}
      <p class="text-sm text-muted-foreground">{list.description}</p>
    {/if}
    <p class="text-sm text-muted-foreground">
      {list.jobs.length.toLocaleString()} {list.jobs.length === 1 ? 'job' : 'jobs'}
    </p>
  </div>

  {#if list.jobs.length === 0}
    <States state="empty" message="This list has no jobs yet." />
  {:else}
    <ul class="flex flex-col divide-y divide-border rounded-lg border border-border">
      {#each list.jobs as job (job.public_slug)}
        <JobRow {job} />
      {/each}
    </ul>
  {/if}
</div>
