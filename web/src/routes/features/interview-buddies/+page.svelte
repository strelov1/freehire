<script lang="ts">
  import { page } from '$app/state';
  import BuddiesLandingView from '#lib/components/BuddiesLandingView.svelte';
  import Seo from '#lib/components/Seo.svelte';
  import { BUDDIES_FAQ } from '#lib/buddiesFaq.js';
  import { faqPageJsonLd, jsonLdScript } from '#lib/seo.js';

  const canonical = $derived(`${page.url.origin}/features/interview-buddies`);
  // The FAQ block and this payload render from the same BUDDIES_FAQ array, so the
  // structured data can never disagree with what the page shows.
  const jsonLd = $derived(jsonLdScript([faqPageJsonLd(BUDDIES_FAQ)]));
</script>

<Seo
  title="Interview buddies — find people interviewing where you are | freehire"
  description="An anonymous, free peer network for job seekers: see who is applying to the same companies and how far they got, and message the person who just had the interview you're preparing for. No recruiters, no crawlers."
  {canonical}
/>

<svelte:head>
  <!-- eslint-disable-next-line svelte/no-at-html-tags -- non-executable JSON-LD built by jsonLdScript, which escapes `<`; raw injection is the only way to emit a structured-data <script> -->
  {@html jsonLd}
</svelte:head>

<div class="mx-auto w-full max-w-6xl px-4 py-6">
  <BuddiesLandingView />
</div>
