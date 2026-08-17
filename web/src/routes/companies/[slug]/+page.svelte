<script lang="ts">
  import { page } from '$app/state';
  import CompanyView from '$lib/components/CompanyView.svelte';
  import Seo from '$lib/components/Seo.svelte';
  import {
    breadcrumbJsonLd,
    companyMetaDescription,
    companyPageTitle,
    jsonLdScript,
    listingRobots,
    organizationJsonLd,
  } from '$lib/seo';
  import type { PageData } from './$types';

  let { data }: { data: PageData } = $props();

  const origin = $derived(page.url.origin);
  const base = $derived(`${origin}/companies/${data.slug}`);
  // Each paginated page is canonical to itself; pointing them all at page one
  // would declare the later postings duplicates of the first twenty.
  const canonical = $derived(data.pageNumber > 1 ? `${base}?page=${data.pageNumber}` : base);
  const description = $derived(companyMetaDescription(data.company));
  // Page 2 onward says so, or every page of a large employer's postings competes
  // for one SERP entry under an identical title.
  const listingTitle = $derived(companyPageTitle(data.company.name, data.initial?.total));
  // A company can reach the sitemap with nothing this page can list; see
  // listingRobots for why the two counts disagree.
  const robots = $derived(listingRobots(data.initial?.total));
  const pageTitle = $derived(
    data.pageNumber > 1
      ? `${data.company.name} — page ${data.pageNumber} · freehire`
      : listingTitle,
  );
  const jsonLd = $derived(
    jsonLdScript([
      organizationJsonLd(data.company, origin),
      breadcrumbJsonLd([
        { name: 'freehire', url: `${origin}/` },
        { name: 'Companies', url: `${origin}/companies` },
        { name: data.company.name, url: base },
      ]),
    ])
  );
</script>

<Seo
  title={pageTitle}
  {description}
  {canonical}
  {robots}
  image={`${origin}/companies/${data.slug}/og.png`}
/>

<svelte:head>
  <!-- eslint-disable-next-line svelte/no-at-html-tags -- non-executable JSON-LD built by jsonLdScript, which escapes `<`; raw injection is the only way to emit a structured-data <script> -->
  {@html jsonLd}
</svelte:head>

<div class="mx-auto w-full max-w-6xl px-4 py-6">
  <!-- Remount on slug change so the seeded paginator/filters start fresh per company. -->
  {#key data.slug}
    <CompanyView
      company={data.company}
      initial={data.initial}
      slug={data.slug}
      currentPage={data.pageNumber}
      referralAvailable={data.referralAvailable}
    />
  {/key}
</div>
