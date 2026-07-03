<script lang="ts">
  import type { Slice } from '$lib/api';
  import type { Company, Job } from '$lib/types';
  import JobsView from './JobsView.svelte';
  import States from './States.svelte';
  import CompanyHeader from './CompanyHeader.svelte';

  // The company entity is server-rendered (route `load`), so the header is in the
  // initial HTML. The job list is *streamed*: `initial` is a promise the route
  // returns unresolved, so the header shows immediately and the list fills in
  // behind a skeleton rather than blocking the whole navigation on the (slower)
  // search. The list reuses the same filterable, counted search view as /jobs,
  // pinned to this company: `company_slug` is fixed (not a selectable facet) and
  // the Source facet is hidden, since a single company's postings share one source.
  let { company, initial, slug }: { company: Company; initial: Promise<Slice<Job>>; slug: string } =
    $props();
</script>

<CompanyHeader {company} {slug} />

<div class="mt-6">
  {#await initial}
    <States state="loading" />
  {:then slice}
    <JobsView initial={slice} scope={{ company_slug: slug }} excludeFacets={['source']} />
  {:catch}
    <States state="error" message="Couldn't load jobs for this company." />
  {/await}
</div>
