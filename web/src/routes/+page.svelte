<script lang="ts">
  import { page } from '$app/state';
  import JobsView from '$lib/components/JobsView.svelte';
  import Seo from '$lib/components/Seo.svelte';
  import { jsonLdScript, siteOrganizationJsonLd, websiteJsonLd } from '$lib/seo';
  import type { PageData } from './$types';

  let { data }: { data: PageData } = $props();

  const origin = $derived(page.url.origin);
  const canonical = $derived(`${origin}/`);
  // What the site is (WebSite + the search action that names our own query
  // parameter) and who publishes it (Organization). These describe the site as a
  // whole, so they belong on the URL search engines treat as the site — the
  // homepage. /about carries the same pair plus its FAQPage, because the visible FAQ
  // lives there; a repeated site-level entity is expected, an absent one is not.
  const jsonLd = $derived(jsonLdScript([websiteJsonLd(origin), siteOrganizationJsonLd(origin)]));
</script>

<Seo
  title="freehire — the open-source search engine for tech jobs"
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
  <JobsView initial={data.initial} initialParams={data.filterParams} />
</div>
