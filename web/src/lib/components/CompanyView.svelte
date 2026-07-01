<script lang="ts">
  import type { Slice } from '$lib/api';
  import type { Company, Job } from '$lib/types';
  import JobsView from './JobsView.svelte';
  import CompanyLogo from './CompanyLogo.svelte';
  import CompanyFollowButton from './CompanyFollowButton.svelte';

  // The company and its first page of search results are server-rendered (route
  // `load`), so the header and rows are in the initial HTML. The job list reuses
  // the same filterable, counted search view as /jobs, pinned to this company:
  // `company_slug` is fixed (not a selectable facet) and the Source facet is
  // hidden, since a single company's postings share one source.
  let { company, initial, slug }: { company: Company; initial: Slice<Job>; slug: string } = $props();
</script>

<div class="flex flex-wrap items-center gap-3">
  <CompanyLogo name={company.name} size="size-8" />
  <h1 class="text-xl font-semibold tracking-tight">{company.name}</h1>
  <div class="ml-auto">
    <CompanyFollowButton {slug} companyName={company.name} />
  </div>
</div>

<div class="mt-6">
  <JobsView {initial} scope={{ company_slug: slug }} excludeFacets={['source']} />
</div>
