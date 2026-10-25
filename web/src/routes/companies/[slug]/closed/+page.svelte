<script lang="ts">
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import CompanyHeader from '#lib/components/CompanyHeader.svelte';
  import CompanyTabs from '#lib/components/CompanyTabs.svelte';
  import JobRow from '#lib/components/JobRow.svelte';
  import Seo from '#lib/components/Seo.svelte';
  import { pageHref, pageWithinWindow } from '#lib/pagination.js';
  import { Breadcrumbs } from '#lib/ui/index.js';
  import type { PageData } from './$types';

  let { data }: { data: PageData } = $props();

  const companyHref = $derived(resolve('/companies/[slug]', { slug: data.slug }));
  const name = $derived(data.company.name);
  const pathname = $derived(page.url.pathname);
  const noParams = new URLSearchParams();
  const breadcrumbItems = $derived([
    { name: 'Companies', href: '/companies' },
    { name, href: companyHref },
    { name: 'Closed' },
  ]);

  const closedOn = (iso: string) =>
    new Date(iso).toLocaleDateString('en-US', { year: 'numeric', month: 'short', day: 'numeric' });
</script>

<!-- noindex: one page per company per offset of postings nobody can apply to is not
     something to hand a crawler. The postings' own pages stay indexable. -->
<Seo
  title={`${name} — closed positions · freehire`}
  description={`Positions ${name} posted that are no longer accepting applications.`}
  robots="noindex, follow"
/>

<div class="mx-auto w-full max-w-6xl px-4 py-6">
  <Breadcrumbs items={breadcrumbItems} class="mb-4" />

  {#key data.slug}
    <CompanyHeader company={data.company} slug={data.slug} />

    <CompanyTabs slug={data.slug} active="closed">
      <div class="mt-6">
        <!-- Names the list between the company's h1 and the job cards' h3s. -->
        <h2 class="sr-only">Closed positions</h2>
        {#if !data.closed}
          <p class="py-16 text-center text-sm text-muted-foreground">Couldn't load closed positions.</p>
        {:else if data.closed.jobs.length === 0}
          <p class="py-16 text-center text-sm text-muted-foreground">
            {name} has no closed positions on freehire.
          </p>
        {:else}
          <!-- Same width and rhythm as the Jobs tab's list; the closing date rides in the
               card's own actions row rather than dangling under its border. -->
          <ul class="flex flex-col gap-3">
            {#each data.closed.jobs as job (job.public_slug)}
              <li>
                <JobRow {job}>
                  {#snippet footer()}
                    {#if job.closed_at}
                      <span class="whitespace-nowrap text-xs text-muted-foreground">Closed {closedOn(job.closed_at)}</span>
                    {/if}
                  {/snippet}
                </JobRow>
              </li>
            {/each}
          </ul>

          <nav class="mt-6 flex justify-between text-sm" aria-label="Closed positions pages">
            {#if data.pageNumber > 1}
              <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- pathname comes from page.url, already resolved (see Pagination.svelte) -->
              <a class="underline hover:text-foreground" href={pageHref(pathname, noParams, data.pageNumber - 1)}>← Newer</a>
            {:else}
              <span></span>
            {/if}
            <!-- The API stops at the feed window; past MAX_PAGE the next page would 404. -->
            {#if data.closed.hasMore && pageWithinWindow(data.pageNumber + 1)}
              <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- as above -->
              <a class="underline hover:text-foreground" href={pageHref(pathname, noParams, data.pageNumber + 1)}>Older →</a>
            {/if}
          </nav>
        {/if}
      </div>
    </CompanyTabs>
  {/key}
</div>
