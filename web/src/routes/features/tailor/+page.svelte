<script lang="ts">
  import { page } from '$app/state';
  import Seo from '$lib/components/Seo.svelte';
  import TailorLandingView from '$lib/components/TailorLandingView.svelte';
  import { faqPageJsonLd, jsonLdScript } from '$lib/seo';
  import { TAILOR_FAQ } from '$lib/tailorFaq';

  const canonical = $derived(`${page.url.origin}/features/tailor`);
  // The FAQ block and this payload render from the same TAILOR_FAQ array, so the
  // structured data can never disagree with what the page shows.
  const jsonLd = $derived(jsonLdScript([faqPageJsonLd(TAILOR_FAQ)]));
</script>

<Seo
  title="CV tailoring — rewrite your CV for one job, inventing nothing | freehire"
  description="freehire reframes your CV toward a single vacancy: it reads the match analysis, pulls forward the experience you already have but buried, asks about anything your history doesn't support, and exports an ATS-readable PDF. Drive it in the browser or from the CLI."
  {canonical}
/>

<svelte:head>
  <!-- eslint-disable-next-line svelte/no-at-html-tags -- non-executable JSON-LD built by jsonLdScript, which escapes `<`; raw injection is the only way to emit a structured-data <script> -->
  {@html jsonLd}
</svelte:head>

<div class="mx-auto w-full max-w-6xl px-4 py-6">
  <TailorLandingView />
</div>
