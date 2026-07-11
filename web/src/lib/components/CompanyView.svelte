<script lang="ts">
  import type { Slice } from '$lib/api';
  import type { Company, Job } from '$lib/types';
  import JobsView from './JobsView.svelte';
  import States from './States.svelte';
  import CompanyHeader from './CompanyHeader.svelte';
  import CompanyAbout from './CompanyAbout.svelte';
  import CompanyFacts from './CompanyFacts.svelte';

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

<!-- Full company description, full-width under the header (CompanyHeader shows only the
     derived tagline). Renders nothing when there's no description. -->
<div class="mt-4">
  <CompanyAbout {company} />
</div>

<!-- Company facts sit atop the jobs sidebar on desktop (passed into JobsView as
     `sidebarTop`); the sidebar is hidden on mobile, so mirror them here as a card
     under the header. CompanyFacts renders nothing when there are no facts. -->
<div class="mt-4 md:hidden">
  <CompanyFacts {company} />
</div>

<div class="mt-4">
  {#await initial}
    <States state="loading" />
  {:then slice}
    <JobsView initial={slice} scope={{ company_slug: slug }} excludeFacets={['source']}>
      {#snippet sidebarTop()}
        <CompanyFacts {company} />
      {/snippet}
    </JobsView>
  {:catch}
    <States state="error" message="Couldn't load jobs for this company." />
  {/await}
</div>
