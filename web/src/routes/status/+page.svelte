<script lang="ts">
  import { page } from '$app/state';
  import Seo from '$lib/components/Seo.svelte';
  import StatusBoard from '$lib/components/StatusBoard.svelte';
  import type { PageData } from './$types';

  let { data }: { data: PageData } = $props();

  const canonical = $derived(`${page.url.origin}/status`);
</script>

<Seo
  title="Status — freehire"
  description="Live status of freehire: whether the site and API are up, and the health of every source adapter crawling job boards."
  {canonical}
/>

<div class="mx-auto w-full max-w-4xl px-4 py-10 sm:py-14">
  <header class="mb-10">
    <p class="font-mono text-xs uppercase tracking-[0.2em] text-muted-foreground">// system status</p>
    <h1 class="mt-4 text-4xl font-semibold tracking-tighter sm:text-5xl">Status.</h1>
    <p class="mt-4 max-w-2xl text-lg leading-relaxed text-muted-foreground">
      The site's own health, and the crawl fleet behind it — pick a tab below.
    </p>
  </header>

  <StatusBoard status={data.status} />
</div>
