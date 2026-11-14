<script lang="ts">
  import type { Snippet } from 'svelte';
  import { resolve } from '$app/paths';
  import { freshnessBadges } from '#lib/freshness.js';
  import { supersedesReality } from '#lib/ghost.js';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { companyLogoUrl } from '#lib/logo.js';
  import type { Job } from '#lib/types.js';
  import { EntityLogo } from '#lib/ui/index.js';
  import AIInterviewBadge from './AIInterviewBadge.svelte';
  import BackerBadge from './BackerBadge.svelte';
  import DirectApplyBadge from './DirectApplyBadge.svelte';
  import RealityBadge from './RealityBadge.svelte';

  // The job page's provenance line: the company, the badges about the posting, and at the
  // end of the line `meta` (JobView's posting dates, desktop only).
  let { job, closed, meta }: { job: Job; closed: boolean; meta: Snippet } = $props();

  // "New" / "Be an early applicant" — see freshness.ts for why the posting date alone
  // does not earn either.
  const freshness = $derived(freshnessBadges(job.posted_at, job.reality, job.applied_count ?? 0, job.closed_at));
</script>

<!-- The company line and the badges about the posting. It WRAPS, at every width: the
     badges are one `shrink-0` group that drops to a line of its own when it does not fit
     beside the company name. It used to be one non-wrapping row below lg with the
     company name as the part that gave way, and measured at 360px a reality chip and its
     "posting date refreshed" note left "Grupo …" of the employer — the one fact on the
     line a reader needs whole. The name still carries `min-w-0` and truncates, but only
     when it alone is wider than the column.
     The badge group keeps `shrink-0` rather than the default `min-width: auto`, whose
     floor for text is the longest WORD: a squeezed chip does not stay one line, it wraps
     "Be an early / applicant" inside itself. `max-w-full` plus its own `flex-wrap` let
     the chips wrap BETWEEN each other instead when even a line of their own is too
     narrow for all of them. -->
<div class="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-2 lg:col-start-2 lg:row-start-1 lg:gap-x-3">
  <EntityLogo
    name={job.company || 'Unknown company'}
    src={companyLogoUrl(job.company) ?? undefined}
    shape="square"
    size="sm"
  />
  <p class="min-w-0 text-sm text-muted-foreground max-lg:truncate">
    {#if job.company_slug}
      <a href={resolve('/companies/[slug]', { slug: job.company_slug })} class="hover:text-foreground hover:underline">
        {job.company || 'Unknown company'}
      </a>
    {:else}
      {job.company || 'Unknown company'}
    {/if}
  </p>

  <div class="flex max-w-full shrink-0 flex-wrap items-center gap-x-2 gap-y-2 lg:gap-x-3">
    <!-- Who backed the employer. The page has room the feed card does not, so the
         badge carries the brand name too and links to that collection's roles. -->
    <BackerBadge collections={job.collections} withLabel />

    <!-- How long this has really been open, on the provenance line rather than under
         the title: it is a fact ABOUT the posting, like the company and the backer,
         and a chip on its own row read as a headline. The ghost checklist stays below
         the title — it is a disclosure with a criteria list inside, not a chip, and it
         supersedes this badge rather than joining it. -->
    <!-- Nor on a closed posting: "Open 35d · reposted 4×" describes one still taking
         applications and contradicts the closure notice under the title. -->
    {#if !closed && !supersedesReality(job.ghost, locale())}
      <RealityBadge reality={job.reality} detailed />
    {/if}

    <!-- How the employer screens, beside how the posting reads. Neutral by design:
         the badge names the practice and its report count, and lets the reader
         decide. -->
    {#if job.direct_apply}
      <DirectApplyBadge />
    {/if}
    <AIInterviewBadge count={job.ai_interview_reports} />

    <!-- Freshness rides the same provenance line: like the backer and the reality
         badge it describes the POSTING, not the role, so the title keeps a single
         voice and the reader still meets all of it in one glance. A closed posting
         earns nothing here — that rule lives in freshnessBadges, beside the reality
         gate, rather than as a second copy in this template. -->
    {#each freshness as badge (badge.label)}
      <!-- The tooltip rides a wrapper, not the Chip: the primitive takes only
           variant/class/children, so a `title` passed to it would be dropped
           silently and the badge would state a claim with its evidence gone. -->
      <span title={badge.tooltip} class="inline-flex">
        <!-- The same look as on every job card: plain text, no pill. -->
        <span class="text-xs font-semibold text-foreground">{badge.label}</span>
      </span>
    {/each}
  </div>

  <!-- `ml-auto` rather than a grid column: the badges above are a variable-width group,
       so the dates take whatever is left of the line and sit against its right edge. -->
  {@render meta()}
</div>
