<script lang="ts">
  import { CircleSlash, ExternalLink } from '@lucide/svelte';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import type { Job } from '#lib/types.js';
  import { formatDate } from '#lib/utils.js';

  // The closure notice under a closed posting's title. `hasSimilar` says the page lists
  // similar open postings at `#similar-jobs` to point the reader at.
  let { job, hasSimilar }: { job: Job; hasSimilar: boolean } = $props();
</script>

<!-- Directly under the title, before anything that reads as a way in: a closed
     posting's first fact is that it is closed. It sat below the tab row once, under
     live Apply buttons that led to a dead page — the controls are withheld now (see
     `closed`), and the reader is pointed at the open postings the route lists below. -->
{#if job.closed_at}
  {@const closedOn = formatDate(job.closed_at, locale())}
  <div
    class="flex flex-wrap items-center justify-between gap-x-4 gap-y-2 rounded-lg border border-border bg-secondary px-4 py-3 text-sm"
  >
    <p class="inline-flex items-center gap-2 font-medium">
      <CircleSlash class="size-4 shrink-0 text-muted-foreground" aria-hidden="true" />
      <span>
        This position is no longer accepting applications{closedOn ? ` (closed ${closedOn})` : ''}.
      </span>
    </p>
    <!-- The posting's own page stays reachable: a reader who doubts the closure (or
         a crawl that read it wrong) has nowhere else to check, and the Apply buttons
         that used to carry this link are withheld. A quiet link, not a CTA, so it does
         not read as a way to apply; `nofollow` for the reason JobView's `applyCta` states. -->
    <div class="flex flex-wrap items-center gap-x-4 gap-y-1">
      {#if job.url && !job.direct_apply}
        <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- the posting's own URL on its employer's site; there is no route to resolve --><a
          href={job.url}
          target="_blank"
          rel="nofollow noopener noreferrer"
          class="inline-flex items-center gap-1.5 font-medium text-muted-foreground hover:text-foreground"
        >
          <ExternalLink class="size-4 shrink-0" aria-hidden="true" />
          View the original posting
        </a>
      {/if}
      {#if hasSimilar}
        <a href="#similar-jobs" class="font-medium text-brand-strong underline underline-offset-4">
          See similar open jobs →
        </a>
      {/if}
    </div>
  </div>
{/if}
