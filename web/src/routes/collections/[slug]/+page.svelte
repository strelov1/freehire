<script lang="ts">
  import { page } from '$app/state';
  import JobsView from '$lib/components/JobsView.svelte';
  import Seo from '$lib/components/Seo.svelte';
  import type { PageData } from './$types';

  let { data }: { data: PageData } = $props();

  const canonical = $derived(`${page.url.origin}/collections/${data.collection.slug}`);
</script>

<Seo
  title={`${data.collection.title} jobs · freehire`}
  description={data.collection.description}
  {canonical}
/>

<div class="mx-auto w-full max-w-6xl px-4 py-6">
  <!-- Remount on slug change so the seeded paginator/filters start fresh per collection. -->
  {#key data.collection.slug}
    <header class="mb-6">
      <p class="font-mono text-xs uppercase tracking-[0.2em] text-muted-foreground">// collection</p>
      <h1 class="mt-3 text-2xl font-semibold tracking-tight">{data.collection.title}</h1>
      <p class="mt-2 max-w-2xl text-sm leading-relaxed text-muted-foreground">
        {data.collection.description}
      </p>
    </header>
    <JobsView initial={data.initial} scope={{ collections: data.collection.slug }} />
  {/key}
</div>
