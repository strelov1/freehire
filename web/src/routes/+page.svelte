<script lang="ts">
  import { page } from '$app/state';
  import HomeView from '$lib/components/HomeView.svelte';
  import Seo from '$lib/components/Seo.svelte';
  import { HOME_FAQ } from '$lib/homeFaq';
  import { faqPageJsonLd, jsonLdScript, siteOrganizationJsonLd, websiteJsonLd } from '$lib/seo';
  import type { PageData } from './$types';

  const { data }: { data: PageData } = $props();

  const origin = $derived(page.url.origin);
  const canonical = $derived(`${origin}/`);
  // One script carrying the site-level entities: what the site is (WebSite +
  // its search action), who publishes it (Organization), and the FAQ.
  const jsonLd = $derived(
    jsonLdScript([websiteJsonLd(origin), siteOrganizationJsonLd(origin), faqPageJsonLd(HOME_FAQ)])
  );
</script>

<Seo
  title="freehire — every tech job, in one clean feed"
  description="FreeHire aggregates tech jobs straight from company career boards, deduplicates them, and tags each with stack, seniority and location. Free and open source."
  {canonical}
/>

<svelte:head>
  <!-- eslint-disable-next-line svelte/no-at-html-tags -- non-executable JSON-LD built by jsonLdScript, which escapes `<`; raw injection is the only way to emit a structured-data <script> -->
  {@html jsonLd}
</svelte:head>

<div class="mx-auto w-full max-w-6xl px-4 py-6">
  <HomeView stats={data.stats} />
</div>
