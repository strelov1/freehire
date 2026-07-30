<script lang="ts">
  import { page } from '$app/state';
  import RecruitersView from '$lib/components/RecruitersView.svelte';
  import Seo from '$lib/components/Seo.svelte';
  import { RECRUITERS_FAQ } from '$lib/recruitersFaq';
  import { breadcrumbJsonLd, faqPageJsonLd, jsonLdScript } from '$lib/seo';

  const origin = $derived(page.url.origin);
  const canonical = $derived(`${origin}/recruiters`);
  // The FAQ block and this payload render from the same RECRUITERS_FAQ array, so the
  // structured data can never disagree with what the page shows.
  const jsonLd = $derived(
    jsonLdScript([
      faqPageJsonLd(RECRUITERS_FAQ),
      breadcrumbJsonLd([
        { name: 'freehire', url: `${origin}/` },
        { name: 'For recruiters', url: canonical },
      ]),
    ])
  );
</script>

<Seo
  title="Post a job — freehire for recruiters"
  description="Submit a tech job to freehire, the free, open-source IT job aggregator. Moderator-reviewed postings join one clean, searchable feed of developer roles."
  {canonical}
/>

<svelte:head>
  <!-- eslint-disable-next-line svelte/no-at-html-tags -- non-executable JSON-LD built by jsonLdScript, which escapes `<`; raw injection is the only way to emit a structured-data <script> -->
  {@html jsonLd}
</svelte:head>

<div class="mx-auto w-full max-w-6xl px-4 py-6">
  <RecruitersView />
</div>
