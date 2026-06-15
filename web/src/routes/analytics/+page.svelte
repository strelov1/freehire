<script lang="ts">
  import { page } from '$app/state';
  import AnalyticsView from '$lib/components/AnalyticsView.svelte';
  import Seo from '$lib/components/Seo.svelte';
  import type { PageData } from './$types';

  let { data }: { data: PageData } = $props();

  const canonical = $derived(`${page.url.origin}/analytics`);
</script>

<Seo
  title="Job market analytics · freehire"
  description="See how many tech vacancies match your criteria — counts by region, stack, seniority, work mode and more, across the freehire catalogue."
  {canonical}
/>

<div class="mx-auto w-full max-w-6xl px-4 py-6">
  <h1 class="mb-4 text-xl font-semibold tracking-tight">Job market analytics</h1>
  <AnalyticsView initial={data.initial} />
</div>
