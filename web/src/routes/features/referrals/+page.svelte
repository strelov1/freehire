<script lang="ts">
  import { page } from '$app/state';
  import ReferralsLandingView from '$lib/components/ReferralsLandingView.svelte';
  import Seo from '$lib/components/Seo.svelte';
  import { REFERRALS_FAQ } from '$lib/referralsFaq';
  import { faqPageJsonLd, jsonLdScript } from '$lib/seo';

  const canonical = $derived(`${page.url.origin}/features/referrals`);
  // The FAQ block and this payload render from the same REFERRALS_FAQ array, so the
  // structured data can never disagree with what the page shows.
  const jsonLd = $derived(jsonLdScript([faqPageJsonLd(REFERRALS_FAQ)]));
</script>

<Seo
  title="Get referred — warm intros over cold applies | freehire"
  description="Ask an employee inside the company to put your name forward — anonymously, for free. freehire's referral marketplace connects job seekers with verified insiders who can refer them, and lets employees offer to refer good people in."
  {canonical}
/>

<svelte:head>
  <!-- eslint-disable-next-line svelte/no-at-html-tags -- non-executable JSON-LD built by jsonLdScript, which escapes `<`; raw injection is the only way to emit a structured-data <script> -->
  {@html jsonLd}
</svelte:head>

<div class="mx-auto w-full max-w-6xl px-4 py-6">
  <ReferralsLandingView />
</div>
