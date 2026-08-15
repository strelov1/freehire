<script lang="ts">
  import type { Slice } from '$lib/api';
  import type { Company, Job } from '$lib/types';
  import JobsView from './JobsView.svelte';
  import States from './States.svelte';
  import CompanyHeader from './CompanyHeader.svelte';
  import CompanyAbout from './CompanyAbout.svelte';
  import CompanyFacts from './CompanyFacts.svelte';
  import ReferralBlock from './ReferralBlock.svelte';

  // Both the company entity and its first page of jobs are server-rendered (route
  // `load`), so the header AND the job rows — with their /jobs/<slug> links — are in
  // the initial HTML for crawlers. The list reuses the same filterable, counted
  // search view as /jobs, pinned to this company: `company_slug` is fixed (not a
  // selectable facet) and the Source facet is hidden, since a single company's
  // postings share one source. `initial` is null when the search call failed; the
  // rest of the page still renders (see the route's load).
  let {
    company,
    initial,
    slug,
    referralAvailable = false,
  }: {
    company: Company;
    initial: Slice<Job> | null;
    slug: string;
    referralAvailable?: boolean;
  } = $props();
</script>

<CompanyHeader {company} {slug} />

{#if referralAvailable}
  <div class="mt-4">
    <ReferralBlock companySlug={slug} companyName={company.name} />
  </div>
{/if}

<!-- Company facts + About sit atop the jobs sidebar on desktop (passed into JobsView
     as `sidebarTop`); the sidebar is hidden on mobile, so mirror them here as cards
     under the header. Both render nothing when the company has no facts/description. -->
<div class="mt-4 flex flex-col gap-4 md:hidden">
  <CompanyFacts {company} />
  <CompanyAbout {company} />
</div>

<div class="mt-4">
  {#if initial}
    <JobsView {initial} scope={{ company_slug: slug }} excludeFacets={['source']}>
      {#snippet sidebarTop()}
        <CompanyFacts {company} />
        <CompanyAbout {company} />
      {/snippet}
    </JobsView>
  {:else}
    <States state="error" message="Couldn't load jobs for this company." />
  {/if}
</div>
