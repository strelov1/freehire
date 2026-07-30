<script lang="ts">
  import { page } from '$app/state';
  import Seo from '$lib/components/Seo.svelte';
  import GhostLandingView from '$lib/components/GhostLandingView.svelte';
  import { faqPageJsonLd, jsonLdScript } from '$lib/seo';
  import { GHOST_FAQ } from '$lib/ghostFaq';

  const canonical = $derived(`${page.url.origin}/features/ghost-jobs`);
  // The FAQ block and this payload render from the same GHOST_FAQ array, so the
  // structured data can never disagree with what the page shows.
  const jsonLd = $derived(jsonLdScript([faqPageJsonLd(GHOST_FAQ)]));
</script>

<Seo
  title="Ghost jobs — which postings are not being filled, and why we think so | freehire"
  description="Between 18% and 27% of listings are jobs nobody is working to fill. freehire watches how a posting behaves and what happened to people who applied, then shows the facts behind every warning — never a claim about an employer's intent."
  {canonical}
/>

<svelte:head>
  <!-- eslint-disable-next-line svelte/no-at-html-tags -- non-executable JSON-LD built by jsonLdScript, which escapes `<`; raw injection is the only way to emit a structured-data <script> -->
  {@html jsonLd}
</svelte:head>

<div class="mx-auto w-full max-w-6xl px-4 py-6">
  <GhostLandingView />
</div>
