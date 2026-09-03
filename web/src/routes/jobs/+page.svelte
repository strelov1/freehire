<script lang="ts">
  import { page } from '$app/state';
  import JobsView from '$lib/components/JobsView.svelte';
  import Seo from '$lib/components/Seo.svelte';
  import { jsonLdScript, siteOrganizationJsonLd } from '$lib/seo';
  import type { PageData } from './$types';

  let { data }: { data: PageData } = $props();

  const origin = $derived(page.url.origin);
  // Each paginated page is canonical to itself: pointing them all at `/jobs` would
  // declare the deeper rows duplicates of the first twenty, which is how the rows
  // the page links to stop being indexed.
  const canonical = $derived(
    data.pageNumber > 1 ? `${origin}/jobs?page=${data.pageNumber}` : `${origin}/jobs`,
  );
  // Page 2 onward says so, or every page competes for one SERP entry under one title.
  const title = $derived(
    data.pageNumber > 1
      ? `Tech jobs — page ${data.pageNumber} · freehire`
      : 'Tech jobs — search 3M+ openings · freehire',
  );
  // Who publishes this. The site-level WebSite + SearchAction pair stays on the
  // homepage, which is the URL search engines treat as the site; the feed names only
  // the publisher, so the entity is stated on the page a searcher actually lands on
  // without the feed claiming to BE the site.
  const jsonLd = $derived(jsonLdScript([siteOrganizationJsonLd(origin)]));
</script>

<Seo
  {title}
  description="Search 3M+ tech jobs indexed straight from company career boards — deduplicated and tagged by stack, seniority and location. Free, open source, no walls."
  {canonical}
/>

<svelte:head>
  <!-- eslint-disable-next-line svelte/no-at-html-tags -- non-executable JSON-LD built by jsonLdScript, which escapes `<`; raw injection is the only way to emit a structured-data <script> -->
  {@html jsonLd}
</svelte:head>

<div class="mx-auto w-full max-w-6xl px-4 py-6">
  <!-- Primary heading kept for search engines and assistive tech, but visually
       hidden: the page's purpose is clear from the feed and onboarding banner, so
       the on-screen title was redundant. sr-only takes no layout space. -->
  <h1 class="sr-only">Tech jobs</h1>
  <h2 class="sr-only">Job listings</h2>
  <JobsView
    initial={data.initial}
    initialParams={data.filterParams}
    currentPage={data.pageNumber}
  />
</div>
