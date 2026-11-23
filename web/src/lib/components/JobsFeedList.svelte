<script lang="ts">
  import { resolve } from '$app/paths';
  import { Button } from '#lib/ui/index.js';
  import States from './States.svelte';
  import type { Job } from '#lib/types.js';
  import JobRow from './JobRow.svelte';
  import type { JobRowProps } from './JobRow.props';

  // The feed's rows on one surface, or why a page whose search matched shows none.
  let {
    jobs,
    clientMatches,
    minMatch,
    serverMatch,
    holdingOver,
    onHide,
    hideCompany,
  }: {
    jobs: Job[];
    clientMatches: ReadonlyMap<string, JobRowProps['clientMatch']>;
    minMatch: number | null;
    serverMatch: boolean;
    holdingOver: boolean;
    onHide: (slug: string) => void;
    hideCompany: boolean;
  } = $props();
</script>

{#if jobs.length === 0}
  <!-- The search DID match on this page — the client-side post-filters removed
       every row. Saying "no matching jobs" here would blame the search for the
       reader's own settings, and the two causes are worth telling apart because
       one of them is a slider they can move. The paginator below still renders,
       so this is somewhere to leave rather than a dead end. -->
  <States
    state="empty"
    message={minMatch === null || serverMatch
      ? 'Every job on this page is one you hid.'
      : 'Every job on this page is hidden or below your match threshold.'}
  />
{:else}
  <!-- Busy while held-over rows stand in for a reload underneath them (see
       `holdover`): the rows are real but about to be replaced. -->
  <!-- One list surface with hairline dividers rather than a stack of separately framed
       cards: twenty bordered boxes read as a wall. -->
  <div
    class="flex flex-col divide-y divide-border/70 overflow-hidden rounded-xl border border-border/70 bg-card"
    aria-busy={holdingOver}
  >
    {#each jobs as job (job.public_slug)}
      <JobRow {job} clientMatch={clientMatches.get(job.public_slug)} {onHide} {hideCompany} flat>
        {#snippet footer()}
          {#if !job.closed_at}
            <Button
              variant="ghost"
              size="sm"
              href={`${resolve('/tools/job-fit')}?job=${encodeURIComponent(job.public_slug)}`}
              aria-label="Review CV for {job.title}">Review CV</Button
            >
          {/if}
        {/snippet}
      </JobRow>
    {/each}
  </div>
{/if}
