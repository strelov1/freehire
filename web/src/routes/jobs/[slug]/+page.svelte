<script lang="ts">
  import { page } from '$app/state';
  import JobRelated from '$lib/components/JobRelated.svelte';
  import JobView from '$lib/components/JobView.svelte';
  import Seo from '$lib/components/Seo.svelte';
  import {
    breadcrumbJsonLd,
    jobPageTitle,
    jobPostingJsonLd,
    jsonLdScript,
    metaDescription,
  } from '$lib/seo';
  import type { PageData } from './$types';

  let { data }: { data: PageData } = $props();

  const origin = $derived(page.url.origin);
  const canonical = $derived(`${origin}/jobs/${data.job.public_slug}`);
  // The per-job OG preview lives beside the canonical URL; og:image must be absolute.
  const ogImage = $derived(`${canonical}/og.png`);
  const description = $derived(metaDescription(data.job.description));
  const jsonLd = $derived(
    jsonLdScript([
      jobPostingJsonLd(data.job, origin),
      breadcrumbJsonLd([
        { name: 'freehire', url: `${origin}/` },
        { name: 'Jobs', url: `${origin}/jobs` },
        { name: data.job.title, url: canonical },
      ]),
    ])
  );
</script>

<Seo title={jobPageTitle(data.job)} {description} {canonical} image={ogImage} />

<svelte:head>
  <!-- JobPosting structured data — eligible for Google Jobs. -->
  <!-- eslint-disable-next-line svelte/no-at-html-tags -- non-executable JSON-LD built by jsonLdScript, which escapes `<`; raw injection is the only way to emit a structured-data <script> -->
  {@html jsonLd}
</svelte:head>

<!-- Slightly wider mobile gutter than the site-wide px-4: the description is dense
     raw text with no card wrapper, so 16px reads tight against the edge; sm+ falls
     back to the shared px-4 rhythm. -->
<div class="mx-auto w-full max-w-6xl px-5 py-6 sm:px-4">
  <JobView job={data.job} />

  <JobRelated
    similar={data.similar}
    copies={data.copies}
    copiesTotal={data.copiesTotal}
    slug={data.job.public_slug}
  />
</div>
