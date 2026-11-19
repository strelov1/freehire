<script lang="ts">
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { supersedesReality } from '#lib/ghost.js';
  import type { Job } from '#lib/types.js';
  import GhostChecklist from './GhostChecklist.svelte';
  import MentorBlock from './MentorBlock.svelte';
  import RecruiterContactsBlock from './RecruiterContactsBlock.svelte';
  import ReferralBlock from './ReferralBlock.svelte';

  // What sits under the job page's title besides its actions: the ghost signal, and the
  // ways to reach someone at the company (a referral, its recruiters, a mentor).
  let { job, onTailor }: { job: Job; onTailor: () => void } = $props();
</script>

<!-- The ghost row supersedes the reality chip (see JobRow). It states the signal
     once for the whole page: a gauge and the hedged wording here, the criteria and
     a link to the full explanation behind its own disclosure. Placed above the
     description, so somebody deciding whether this is worth an hour of unpaid work
     meets the hedge before the pitch rather than after they have already invested
     the reading. The caveat itself now lives on /features/ghost-jobs — the row
     carries the ceiling on the claim ("possibly", two of four), not the essay. -->
{#if supersedesReality(job.ghost, locale())}
  <GhostChecklist ghost={job.ghost} />
{/if}

{#if job.referral_available && job.company_slug}
  <ReferralBlock companySlug={job.company_slug} companyName={job.company} jobSlug={job.public_slug} {onTailor} />
{/if}

<!-- Writing to the people who recruit: offered only where the API says a lookup can run. -->
{#if job.recruiter_contacts_available && job.closed_at == null}
  <RecruiterContactsBlock jobSlug={job.public_slug} companyName={job.company} />
{/if}

<!-- Mentorship is a separate way to get help from someone at this company. -->
{#if job.company_slug}
  <MentorBlock companySlug={job.company_slug} companyName={job.company} />
{/if}
