<script lang="ts">
  import { page } from '$app/state';
  import Seo from '#lib/components/Seo.svelte';
  import TailorLandingView from '#lib/components/TailorLandingView.svelte';
  import { faqPageJsonLd, jsonLdScript } from '#lib/seo.js';
  import { TAILOR_FAQ } from '#lib/tailorFaq.js';
  import type { PageData } from './$types';

  const { data }: { data: PageData } = $props();

  const canonical = $derived(`${page.url.origin}/features/tailor`);
  // The FAQ block and this payload render from the same TAILOR_FAQ array, so the
  // structured data can never disagree with what the page shows.
  const jsonLd = $derived(jsonLdScript([faqPageJsonLd(TAILOR_FAQ)]));
</script>

<Seo
  title="CV tailoring — your CV rewritten for each job, nothing made up | freehire"
  description="freehire rewrites your CV for one vacancy at a time: it brings your relevant work forward in the job's own words, asks you about anything your history doesn't support, and shows the job match rising. ATS-readable PDF. Part of Pro."
  {canonical}
/>

<svelte:head>
  <!-- eslint-disable-next-line svelte/no-at-html-tags -- non-executable JSON-LD built by jsonLdScript, which escapes `<`; raw injection is the only way to emit a structured-data <script> -->
  {@html jsonLd}
</svelte:head>

<div class="mx-auto w-full max-w-6xl px-4 py-6">
  <TailorLandingView price={data.price} />
</div>
