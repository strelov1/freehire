<script lang="ts">
  import { page } from '$app/state';
  import Seo from '$lib/components/Seo.svelte';
  import TrackingLandingView from '$lib/components/TrackingLandingView.svelte';
  import { faqPageJsonLd, jsonLdScript } from '$lib/seo';
  import { TRACKING_FAQ } from '$lib/trackingFaq';

  const canonical = $derived(`${page.url.origin}/features/tracking`);
  // The FAQ block and this payload render from the same TRACKING_FAQ array, so the
  // structured data can never disagree with what the page shows.
  const jsonLd = $derived(jsonLdScript([faqPageJsonLd(TRACKING_FAQ)]));
</script>

<Seo
  title="Job application tracking — one board, from Preparing to Offer | freehire"
  description="Track every application on one board — Preparing, Applied, Interview, Offer — with a day-counter when an employer goes quiet, notes on every card, and recruiter replies that attach and advance it themselves. Also a List, a Pipeline funnel, and a Calendar. Drive it from the browser or the CLI."
  {canonical}
/>

<svelte:head>
  <!-- eslint-disable-next-line svelte/no-at-html-tags -- non-executable JSON-LD built by jsonLdScript, which escapes `<`; raw injection is the only way to emit a structured-data <script> -->
  {@html jsonLd}
</svelte:head>

<div class="mx-auto w-full max-w-6xl px-4 py-6">
  <TrackingLandingView />
</div>
