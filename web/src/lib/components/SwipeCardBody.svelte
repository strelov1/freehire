<script lang="ts">
  import type { Snippet } from 'svelte';
  import { resolve } from '$app/paths';
  import { cardTags, formatSalary } from '#lib/enrichment.js';
  import { companyLogoUrl } from '#lib/logo.js';
  import type { Job } from '#lib/types.js';
  import { Badge, EntityLogo } from '#lib/ui/index.js';
  import JobMatch from './JobMatch.svelte';

  // What a swipe card shows, inside its frame: the employer, title, salary, tags, synopsis,
  // match, description and the link out. SwipeCard owns the drag and the frame.
  // `match`, when given, renders under the title in place of the JobMatch coverage bar;
  // `badge` renders at the end of the company row.
  let { job, match, badge }: { job: Job; match?: Snippet; badge?: Snippet } = $props();

  const salary = $derived(job.enrichment ? formatSalary(job.enrichment) : null);
  const tags = $derived(cardTags(job));
  const skills = $derived(job.skills?.slice(0, 6) ?? []);
  // The model-written one-line synopsis (only on enriched jobs); shown as the card's
  // lead, with the full description kept below as the fallback / detail.
  const summary = $derived(job.enrichment?.summary ?? '');
</script>

<!-- Scroll container holds the whole card. touch-action: pan-y lets the
     browser own vertical scrolling while onPointerMove's axis-lock keeps
     horizontal drags as swipes; overscroll-contain stops the scroll from
     chaining to the page behind the overlay. -->
<div class="swipe-scroll flex min-h-0 flex-1 touch-pan-y flex-col gap-3 overflow-y-auto overscroll-contain p-5">
  <div class="flex items-center gap-3">
    <EntityLogo
      name={job.company || 'Unknown company'}
      src={companyLogoUrl(job.company) ?? undefined}
      shape="square"
      size="xs"
    />
    <div class="min-w-0">
      <div class="truncate font-semibold">{job.company || 'Unknown company'}</div>
      {#if job.location}
        <div class="truncate text-sm text-muted-foreground">{job.location}</div>
      {/if}
    </div>
    {@render badge?.()}
  </div>

  <!-- The wrapper is load-bearing. line-clamp sets display:-webkit-box, and a
       -webkit-box collapses to height 0 when it's a direct child of this flex
       column (a legacy box reports no intrinsic block size to the modern flex
       parent, so the title vanishes). Nesting it in a plain block restores
       normal sizing while keeping the 3-line clamp. -->
  <div>
    <h2 class="line-clamp-3 text-2xl font-bold tracking-tight">{job.title}</h2>
  </div>

  {@render match?.()}

  {#if salary}
    <div class="text-lg font-bold tracking-tight text-brand-strong">
      {salary}
    </div>
  {/if}

  <div class="flex flex-wrap gap-1.5">
    {#each tags as tag (tag)}
      <Badge variant="secondary">{tag}</Badge>
    {/each}
    {#each skills as skill (skill)}
      <Badge variant="brand">{skill}</Badge>
    {/each}
  </div>

  {#if summary}
    <!-- Model-written synopsis: the card's lead. Short (≤400 chars), plain text. -->
    <p class="text-sm leading-relaxed text-foreground">{summary}</p>
  {/if}

  <!-- Personal profile-match block (reuses the detail-page component). Keyed
       by slug via the parent's {#key} (the deck or the match feed's swipe mode), so a
       fresh instance fetches per card. No match analysis handed in: a swipe card is rapid triage with no
       call-to-action row of its own, and reading one per card would spend a
       request on the cached verdict of a job almost none of which has ever been
       analysed. The deterministic coverage bar is what this card is for. -->
  {#if !match}
    <JobMatch {job} matchAnalysis={null} />
  {/if}

  {#if job.description}
    <!-- Description is server-sanitized HTML (see internal/sources), safe to
         render. Now scrolls in full with the rest of the card. -->
    <div class="job-teaser text-sm leading-relaxed text-muted-foreground">
      <!-- eslint-disable-next-line svelte/no-at-html-tags -- server-sanitized; the rule flags every {@html} regardless -->
      {@html job.description}
    </div>
  {/if}

  <!-- Opens in a new tab so the deck (and your place in it) is preserved —
       return by closing/switching back to this tab. -->
  <a
    href={resolve('/jobs/[slug]', { slug: job.public_slug })}
    target="_blank"
    rel="noopener"
    class="pt-1 text-left text-sm font-medium text-foreground underline-offset-4 hover:underline"
  >
    Open full details →
  </a>
</div>

<style>
  /* Scraped HTML: long URLs or nbsp-glued words must wrap, not force a scroll. */
  .job-teaser {
    overflow-wrap: break-word;
  }

  /* Flush the teaser's top with the card's gap — drop the first block's margin. */
  .job-teaser > :global(:first-child) {
    margin-top: 0;
  }

  /* Headings would be oversized in a teaser — render them as plain emphasis. */
  .job-teaser :global(h1),
  .job-teaser :global(h2),
  .job-teaser :global(h3),
  .job-teaser :global(h4) {
    margin: 0.5rem 0 0.25rem;
    font-weight: 600;
  }

  .job-teaser :global(p) {
    margin: 0.5rem 0;
  }

  .job-teaser :global(ul),
  .job-teaser :global(ol) {
    margin: 0.5rem 0;
    padding-left: 1.25rem;
  }

  .job-teaser :global(li) {
    display: list-item;
    list-style: disc outside;
    margin: 0.25rem 0;
  }

  /* Some boards wrap each <li> in a <p>; collapse it so the bullet sits inline. */
  .job-teaser :global(li) > :global(p) {
    margin: 0;
  }

  .job-teaser :global(b),
  .job-teaser :global(strong) {
    font-weight: 600;
  }
</style>
