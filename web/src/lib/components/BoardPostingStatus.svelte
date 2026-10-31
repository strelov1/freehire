<script lang="ts">
  import type { MyJob } from '#lib/types.js';
  import { deadlineRemainingLabel } from '#lib/board.js';
  let { job }: { job: MyJob['job'] } = $props();
</script>

{#if job?.closed_at || job?.application_deadline}
  <span class="flex flex-wrap gap-x-2 gap-y-1 text-xs">
    {#if job?.closed_at}
      <span class="font-medium text-destructive">Vacancy closed</span>
    {/if}
    {#if job?.application_deadline}
      <span class="text-muted-foreground">Apply by <time datetime={job.application_deadline}>{job.application_deadline}</time></span>
      {#if !job.closed_at}
        <span class="text-muted-foreground">{deadlineRemainingLabel(job.application_deadline)}</span>
      {/if}
    {/if}
  </span>
{/if}
