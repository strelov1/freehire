<script lang="ts">
  import { page } from '$app/state';
  import JobView from '$lib/components/JobView.svelte';
  import Seo from '$lib/components/Seo.svelte';
  import { jobPageTitle, jobPostingJsonLd, jsonLdScript, metaDescription } from '$lib/seo';
  import type { PageData } from './$types';

  let { data }: { data: PageData } = $props();

  const origin = $derived(page.url.origin);
  const canonical = $derived(`${origin}/jobs/${data.job.public_slug}`);
  // The per-job OG preview lives beside the canonical URL; og:image must be absolute.
  const ogImage = $derived(`${canonical}/og.png`);
  const description = $derived(metaDescription(data.job.description));
  const jsonLd = $derived(jsonLdScript(jobPostingJsonLd(data.job, origin)));
</script>

<Seo title={jobPageTitle(data.job)} {description} {canonical} image={ogImage} />

<svelte:head>
  <!-- JobPosting structured data — eligible for Google Jobs. -->
  {@html jsonLd}
</svelte:head>

<div class="mx-auto w-full max-w-6xl px-4 py-6">
  <JobView job={data.job} />
</div>
