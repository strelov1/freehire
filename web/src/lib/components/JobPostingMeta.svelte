<script lang="ts">
  import { Check, Clock, Eye, RefreshCw } from '@lucide/svelte';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import type { Job } from '#lib/types.js';
  import { formatDateOrAgo, formatDateTime } from '#lib/utils.js';

  // The job page's posting line: when it was posted and updated, the deadline, and how many
  // looked and applied. `className` is the caller's display and layout classes.
  let { job, className }: { job: Job; className: string } = $props();

  // Both read as an age for their first day ("20 minutes ago") and as a date after it —
  // the same label the feed's card already gives a posting, so a reader arriving from
  // the list meets the answer in the form they just left.
  const posted = $derived(formatDateOrAgo(job.posted_at, locale(), 'short'));
  // When the posting's own content last changed. `jobs.updated_at` is deliberately left
  // unstamped by the liveness refresh (internal/platform/db/queries/jobs.sql), so the column
  // means "the words moved", not "the crawler came back" — which is the only reading that
  // earns a line beside the posting date.
  const updated = $derived(formatDateOrAgo(job.updated_at, locale(), 'short'));
  // Engagement counters (distinct signed-in viewers / applicants), served on the
  // job. A zero metric is omitted so the line never reads as a dead "0 views".
  const views = $derived(job.view_count ?? 0);
  const applies = $derived(job.applied_count ?? 0);
</script>

<!-- How this posting is doing: when it went up, when its words last moved, and how many
     people have opened it or told us they applied. Facts ABOUT the posting rather than
     about the role, so they ride the provenance line with the company and the badges,
     rather than the sidebar, where a reader comparing "posted 20 minutes ago" against "3
     views" had to hold one thought across two places — past the match score and the salary
     to reach either.
     Every one of them is an icon and a number, which is what lets four of them share a line
     the words would have filled on their own. The icons carry the word they replaced in an
     `sr-only` span, so the line reads the same to a screen reader as it did when it was
     spelled out.
     "Updated" is dropped when it renders to the same label as the posting date: a job
     written once and never touched would otherwise say the same thing twice. Comparing
     the LABELS rather than the timestamps is the point — two edits an hour apart on one
     day are a real difference while both read as ages, and stop being one the day they
     both collapse into that date.
     The exact clock time rides the `title` on both dates, since a reader who has read "2
     hours ago" off a server-rendered page may want to know how old the page is.
     JobView renders it twice, one visible at a time (passing the display class): to the
     right of the provenance line from lg, and under the title below it, where that line
     already spends a phone's width on the company name and its badges. -->
{#if posted || views > 0 || applies > 0 || job.application_deadline}
  <div class={`items-center gap-x-3 text-xs text-muted-foreground ${className}`}>
    {#if posted}
      <span
        class="inline-flex items-center gap-1 whitespace-nowrap"
        title={`Posted ${formatDateTime(job.posted_at, locale())}`}
      >
        <Clock class="size-3.5 shrink-0" aria-hidden="true" />
        <span class="sr-only">Posted</span>
        <time datetime={job.posted_at}>{posted}</time>
      </span>
      {#if updated && updated !== posted}
        <span
          class="inline-flex items-center gap-1 whitespace-nowrap"
          title={`Updated ${formatDateTime(job.updated_at, locale())}`}
        >
          <RefreshCw class="size-3.5 shrink-0" aria-hidden="true" />
          <span class="sr-only">Updated</span>
          <time datetime={job.updated_at}>{updated}</time>
        </span>
      {/if}
    {/if}
    {#if job.application_deadline}
      <span class="inline-flex items-center gap-1 whitespace-nowrap">
        Apply by <time datetime={job.application_deadline}>{job.application_deadline}</time>
      </span>
    {/if}
    <!-- A zero counter is omitted rather than drawn: "0 views" on a posting nobody has
         opened yet measures our own traffic, not the job. -->
    {#if views > 0}
      <span class="inline-flex items-center gap-1 whitespace-nowrap">
        <Eye class="size-3.5 shrink-0" aria-hidden="true" />
        {views}
        {views === 1 ? 'view' : 'views'}
      </span>
    {/if}
    {#if applies > 0}
      <span class="inline-flex items-center gap-1 whitespace-nowrap">
        <Check class="size-3.5 shrink-0" aria-hidden="true" />
        {applies} applied
      </span>
    {/if}
  </div>
{/if}
