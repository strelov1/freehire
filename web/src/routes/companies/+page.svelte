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
  <CompaniesView initial={data.initial} />
</div>
