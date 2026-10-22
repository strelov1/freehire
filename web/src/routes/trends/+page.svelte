<script lang="ts">
  import { page } from '$app/state';
  import Seo from '#lib/components/Seo.svelte';
  import ActivityPanel from '#lib/components/ActivityPanel.svelte';
  import { api } from '#lib/api.js';
  import { breadcrumbJsonLd, datasetJsonLd, jsonLdScript } from '#lib/seo.js';
  import type { PageData } from './$types';

  let { data }: { data: PageData } = $props();

  const origin = $derived(page.url.origin);
  const canonical = $derived(`${origin}/trends`);
  const description =
    'How the freehire catalogue moves over time — new vacancies added versus postings removed, by day, week, or month.';
  // A time series, not a page of prose: the Dataset names the endpoint the chart
  // reads so an engine can fetch the series rather than infer it from the bars.
  const jsonLd = $derived(
    jsonLdScript([
      datasetJsonLd('freehire job-posting activity over time', description, canonical, origin, [
        { name: 'Jobs activity API', contentUrl: `${origin}/api/v1/stats/jobs-activity` },
      ]),
      breadcrumbJsonLd([
        { name: 'freehire', url: `${origin}/` },
        { name: 'Trends', url: canonical },
      ]),
    ]),
  );
</script>

<!-- "Trends · freehire" was 17 characters: it named no subject, so nothing in it
     could match a query, and it left two thirds of the SERP title width unused. -->
<Seo title="Tech hiring trends — jobs added and removed daily · freehire" {description} {canonical} />

<svelte:head>
  <!-- eslint-disable-next-line svelte/no-at-html-tags -- non-executable JSON-LD built by jsonLdScript, which escapes `<`; raw injection is the only way to emit a structured-data <script> -->
  {@html jsonLd}
</svelte:head>

<div class="mx-auto w-full max-w-6xl px-4 py-6">
  <ActivityPanel initial={data.initial} load={(g) => api.jobsActivity(g)}>
    {#snippet title()}
      <h1 class="text-xl font-semibold tracking-tight">Trends</h1>
      <p class="mt-1 text-sm text-muted-foreground">
        New vacancies added versus postings removed over time.
      </p>
    {/snippet}
  </ActivityPanel>
</div>
