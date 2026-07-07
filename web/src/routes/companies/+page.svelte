<script lang="ts">
  import { page } from '$app/state';
  import CompaniesView from '$lib/components/CompaniesView.svelte';
  import Seo from '$lib/components/Seo.svelte';
  import type { PageData } from './$types';

  let { data }: { data: PageData } = $props();

  const canonical = $derived(`${page.url.origin}/companies`);
</script>

<Seo
  title="Companies · freehire"
  description="Browse companies hiring in tech, with their current open roles — aggregated by freehire."
  {canonical}
/>

<div class="mx-auto w-full max-w-6xl px-4 py-6">
  <!-- Primary heading for search engines and assistive tech. pl-12 clears the
       mobile filters edge tab (reset at md), matching the list's top row. -->
  <h1 class="mb-4 pl-12 text-2xl font-semibold tracking-tight md:pl-0">Companies hiring in tech</h1>
  <CompaniesView initial={data.initial} />
</div>
