<script lang="ts">
  import { page } from '$app/state';
  import HomeLandingView from '$lib/components/HomeLandingView.svelte';
  import Seo from '$lib/components/Seo.svelte';
  import { jsonLdScript, siteOrganizationJsonLd, websiteJsonLd } from '$lib/seo';
  import type { PageData } from './$types';

  let { data }: { data: PageData } = $props();

  const origin = $derived(page.url.origin);
  // What the site is (WebSite plus the search action, which names the feed's URL and
  // our own query parameter) and who publishes it (Organization). These describe the
  // site as a whole, so they belong on the URL search engines treat as the site — the
  // homepage. /about carries the same pair plus its FAQPage, because the visible FAQ
  // lives there; a repeated site-level entity is expected, an absent one is not.
  const jsonLd = $derived(jsonLdScript([websiteJsonLd(origin), siteOrganizationJsonLd(origin)]));
</script>

<Seo
  title="freehire — the open-source search engine for tech jobs"
  description="Search 3M+ tech jobs indexed straight from company career boards — deduplicated and tagged by stack, seniority and location. Free, open source, no walls."
  canonical="{origin}/"
/>

<svelte:head>
  <!-- eslint-disable-next-line svelte/no-at-html-tags -- non-executable JSON-LD built by jsonLdScript, which escapes `<`; raw injection is the only way to emit a structured-data <script> -->
  {@html jsonLd}
</svelte:head>

<!-- The same container every landing route here brings (/about, /cli,
     /features/*): `main` is a bare `flex-1` with no width or padding of its own, so a
     page that skips this one runs its copy from window edge to window edge. `py-6` is
     dropped: the hero measures itself against the viewport and adds its own vertical
     room, and a wrapper's padding on top of that pushes the fold. -->
<div class="mx-auto w-full max-w-6xl px-4">
  <HomeLandingView counts={data.counts} scale={data.scale} />
</div>
