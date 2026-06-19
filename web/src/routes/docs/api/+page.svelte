<script lang="ts">
  import { page } from '$app/state';
  import DocsApiView from '$lib/components/DocsApiView.svelte';
  import Seo from '$lib/components/Seo.svelte';

  const canonical = $derived(`${page.url.origin}/docs/api`);
</script>

<Seo
  title="freehire API reference — query jobs by filters"
  description="The freehire HTTP API: a read-first, open endpoint set over the job catalogue. Search and filter jobs by seniority, skills, region, salary and more, read companies, and track applications with an API key."
  {canonical}
/>

<div class="mx-auto w-full max-w-6xl px-4 py-6">
  <DocsApiView />
</div>
