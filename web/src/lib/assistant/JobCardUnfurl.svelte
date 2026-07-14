<script lang="ts">
  import { resolve } from '$app/paths';
  import JobRow from '$lib/components/JobRow.svelte';
  import { loadJob } from './jobCache';

  // A freehire job link the chat detected. `slug` hydrates the card; if the job
  // can't be fetched (404 / network / removed beyond reach) it degrades to a
  // plain link to the detail page — the message is never broken.
  let { slug }: { slug: string } = $props();
</script>

<div class="my-2">
  {#await loadJob(slug)}
    <!-- Loading placeholder shaped like a JobRow so the layout doesn't jump. -->
    <div class="animate-pulse rounded-xl border border-border bg-card p-4" aria-hidden="true">
      <div class="mb-3 h-4 w-1/3 rounded bg-muted"></div>
      <div class="mb-2 h-5 w-2/3 rounded bg-muted"></div>
      <div class="h-3 w-full rounded bg-muted"></div>
    </div>
  {:then job}
    <JobRow {job} dimViewed={false} newTab />
  {:catch}
    <a
      href={resolve('/jobs/[slug]', { slug })}
      target="_blank"
      rel="noopener"
      class="text-brand underline underline-offset-2"
    >
      View job ↗
    </a>
  {/await}
</div>
