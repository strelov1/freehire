<script lang="ts">
  import { supersedesReality } from '#lib/ghost.js';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import type { Job, JobCard } from '#lib/types.js';
  import AIInterviewBadge from './AIInterviewBadge.svelte';
  import AppliedCompanyBadge from './AppliedCompanyBadge.svelte';
  import DirectApplyBadge from './DirectApplyBadge.svelte';
  import GhostBadge from './GhostBadge.svelte';
  import RealityBadge from './RealityBadge.svelte';

  // The signals at the head of a job card's badge row: on-site application, the reader's
  // earlier application to the employer, the posting's reality (or the ghost verdict that
  // supersedes it), AI-interview reports and freshness.
  let {
    job,
    directApply,
    appliedToCompany,
    ghost,
    reality,
    freshness,
  }: {
    job: Job | JobCard;
    directApply: boolean;
    appliedToCompany: boolean;
    ghost: Job['ghost'] | undefined;
    reality: Job['reality'] | undefined;
    freshness: { label: string; tooltip: string }[];
  } = $props();
</script>

<!-- evergreen_posting IS the reality verdict, so showing both chips states one
   fact twice, the second time louder. The ghost chip carries it inside its
   checklist; where ghost is silent, reality renders exactly as before. -->
{#if directApply}
  <DirectApplyBadge />
{/if}
<!-- A fact about the reader, not the posting: they applied to this employer before.
   Ahead of the posting's own signals because it changes what the card is for —
   a second application to the same company is a different decision. -->
{#if appliedToCompany}
  <AppliedCompanyBadge />
{/if}
{#if supersedesReality(ghost, locale())}
  <GhostBadge {ghost} />
{:else}
  <RealityBadge {reality} plain />
{/if}
<!-- A separate fact from the two above: those describe the POSTING's reality, this
   describes how the EMPLOYER screens. It is neutral and joins them rather than
   superseding either. Renders nothing when nobody has reported. -->
<AIInterviewBadge count={job.ai_interview_reports} />
<!-- Freshness before the facets: "New" is a fact about right now, the facets are
   stable attributes of the role. Both sit behind the reality/ghost chip, because
   a warning that a posting may not be real outranks a note that it is fresh.
   The badge's title is the claim's own basis (when it was posted), not a
   restatement of its label. -->
{#each freshness as badge (badge.label)}
  <!-- The tooltip rides a wrapper, not the Badge: the primitive takes only
     variant/class/children, so a `title` passed to it would be dropped
     silently and the badge would state a claim with its evidence gone. The
     wrapper (rather than a span inside) also makes the badge's own padding
     hoverable — the same shape the job's own page uses. -->
  <span title={badge.tooltip} class="inline-flex shrink-0">
    <!-- Plain text, not a pill and not the accent: it says the least on the row, so it
       must not be its loudest shape, and the brand colour is kept for primary actions. -->
    <span class="text-xs font-semibold text-foreground">{badge.label}</span>
  </span>
{/each}
