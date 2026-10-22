<script lang="ts">
  import { page } from '$app/state';
  import JobsApiLandingView from '#lib/components/JobsApiLandingView.svelte';
  import Seo from '#lib/components/Seo.svelte';
  import { JOBS_API_FAQ } from '#lib/jobsApiFaq.js';
  import { breadcrumbJsonLd, faqPageJsonLd, jsonLdScript } from '#lib/seo.js';
  import type { PageData } from './$types';

  const { data }: { data: PageData } = $props();

  const origin = $derived(page.url.origin);
  const canonical = $derived(`${origin}/jobs-api`);
  const jsonLd = $derived(
    jsonLdScript([
      faqPageJsonLd(JOBS_API_FAQ),
      breadcrumbJsonLd([
        { name: 'freehire', url: `${origin}/` },
        { name: 'Jobs API', url: canonical },
      ]),
    ]),
  );
</script>

<Seo
  title="Jobs API for commercial use — freehire"
  description="Millions of open job postings read from employers’ own ATS, de-duplicated and structured, behind one API. Flat monthly price, no per-job metering, a plain licence to show them in your product."
  {canonical}
/>

<svelte:head>
  <!-- eslint-disable-next-line svelte/no-at-html-tags -- non-executable JSON-LD built by jsonLdScript, which escapes `<`; raw injection is the only way to emit a structured-data <script> -->
  {@html jsonLd}
</svelte:head>

<div class="mx-auto w-full max-w-6xl px-4 py-6">
  <JobsApiLandingView stats={data.stats} plans={data.plans} />
</div>
