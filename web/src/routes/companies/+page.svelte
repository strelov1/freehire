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
  <!-- Primary heading kept for search engines and assistive tech, but visually
       hidden: the list and filters make the page's purpose clear, so the on-screen
       title was redundant. sr-only takes no layout space. Mirrors /jobs. -->
  <h1 class="sr-only">Companies hiring in tech</h1>
  <CompaniesView initial={data.initial} />
</div>
