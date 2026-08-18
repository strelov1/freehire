<script lang="ts">
  import { page } from '$app/state';
  import Seo from '$lib/components/Seo.svelte';
  import AdvancedSearchLandingView from '$lib/components/AdvancedSearchLandingView.svelte';
  import { breadcrumbJsonLd, faqPageJsonLd, jsonLdScript } from '$lib/seo';
  import { ADVANCED_SEARCH_FAQ } from '$lib/advancedSearchFaq';

  const origin = $derived(page.url.origin);
  const canonical = $derived(`${origin}/features/advanced-search`);
  const jsonLd = $derived(
    jsonLdScript([
      faqPageJsonLd(ADVANCED_SEARCH_FAQ),
      breadcrumbJsonLd([
        { name: 'freehire', url: `${origin}/` },
        { name: 'Advanced search', url: canonical },
      ]),
    ])
  );
</script>

<Seo
  title="Advanced job search — filters, exclusions and saved alerts | freehire"
  description="Twenty filterable facets — role, skills, seniority, region, company, salary and more — each one includable or excludable, and saved to your profile as an alert. The same filters work from the API and the CLI."
  {canonical}
/>

<svelte:head>
  <!-- eslint-disable-next-line svelte/no-at-html-tags -- non-executable JSON-LD built by jsonLdScript, which escapes `<`; raw injection is the only way to emit a structured-data <script> -->
  {@html jsonLd}
</svelte:head>

<div class="mx-auto w-full max-w-6xl px-4 py-6">
  <AdvancedSearchLandingView />
</div>