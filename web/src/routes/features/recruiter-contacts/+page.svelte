<script lang="ts">
  import { page } from '$app/state';
  import Seo from '#lib/components/Seo.svelte';
  import RecruiterContactsLandingView from '#lib/components/RecruiterContactsLandingView.svelte';
  import { faqPageJsonLd, jsonLdScript } from '#lib/seo.js';
  import { RECRUITER_CONTACTS_FAQ } from '#lib/recruiterContactsFaq.js';
  import type { PageData } from './$types';

  const { data }: { data: PageData } = $props();

  const canonical = $derived(`${page.url.origin}/features/recruiter-contacts`);
  // The FAQ block and this payload render from the same array, so they can never disagree.
  const jsonLd = $derived(jsonLdScript([faqPageJsonLd(RECRUITER_CONTACTS_FAQ)]));
</script>

<Seo
  title="Recruiter contacts — write to the person who hires | freehire"
  description="On any job page, see who recruits at that company and their work email, and draft a short personal note from your CV. A lookup that finds nobody is not counted. Part of Pro."
  {canonical}
/>

<svelte:head>
  <!-- eslint-disable-next-line svelte/no-at-html-tags -- non-executable JSON-LD built by jsonLdScript, which escapes `<`; raw injection is the only way to emit a structured-data <script> -->
  {@html jsonLd}
</svelte:head>

<div class="mx-auto w-full max-w-6xl px-4 py-6">
  <RecruiterContactsLandingView price={data.price} />
</div>
