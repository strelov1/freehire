<script lang="ts">
  import { page } from '$app/state';
  import ForCompaniesView from '$lib/components/ForCompaniesView.svelte';
  import Seo from '$lib/components/Seo.svelte';
  import { FOR_COMPANIES_FAQ } from '$lib/forCompaniesFaq';
  import { breadcrumbJsonLd, faqPageJsonLd, jsonLdScript } from '$lib/seo';

  const origin = $derived(page.url.origin);
  const canonical = $derived(`${origin}/for-companies`);
  // The FAQ block and this payload render from the same FOR_COMPANIES_FAQ array, so
  // the structured data can never disagree with what the page shows.
  const jsonLd = $derived(
    jsonLdScript([
      faqPageJsonLd(FOR_COMPANIES_FAQ),
      breadcrumbJsonLd([
        { name: 'freehire', url: `${origin}/` },
        { name: 'For companies', url: canonical },
      ]),
    ])
  );
</script>

<Seo
  title="List your job board — freehire for companies"
  description="Get your company's whole ATS board indexed by freehire, the free, open-source IT job aggregator. Add a supported board with one line, or contribute an adapter — we crawl it regularly and close roles you take down."
  {canonical}
/>

<svelte:head>
  <!-- eslint-disable-next-line svelte/no-at-html-tags -- non-executable JSON-LD built by jsonLdScript, which escapes `<`; raw injection is the only way to emit a structured-data <script> -->
  {@html jsonLd}
</svelte:head>

<div class="mx-auto w-full max-w-6xl px-4 py-6">
  <ForCompaniesView />
</div>
