<script lang="ts">
  import { page } from '$app/state';
  import Seo from '$lib/components/Seo.svelte';
  import type { PageData } from './$types';

  let { data }: { data: PageData } = $props();

  const canonical = $derived(`${page.url.origin}/collections`);

  const count = (slug: string) => data.counts[slug] ?? 0;
</script>

<Seo
  title="Collections · freehire"
  description="Curated collections of open tech jobs — Y Combinator–backed companies, Big Tech, and more."
  {canonical}
/>

<div class="mx-auto w-full max-w-6xl px-4 py-6">
  <header class="mb-8">
    <h1 class="text-2xl font-semibold tracking-tight">Collections</h1>
    <p class="mt-2 max-w-2xl text-sm leading-relaxed text-muted-foreground">
      Curated groups of companies, with their open roles in one feed.
    </p>
  </header>

  <div class="grid gap-px overflow-hidden rounded-xl border border-border bg-border sm:grid-cols-2">
    {#each data.collections as collection (collection.slug)}
      <a
        href={`/jobs?collections=${collection.slug}`}
        class="group flex flex-col bg-background p-6 transition-colors hover:bg-secondary/40"
      >
        <div class="flex items-baseline justify-between gap-3">
          <h2 class="text-lg font-semibold tracking-tight">{collection.title}</h2>
          <span class="shrink-0 font-mono text-xs text-muted-foreground">
            {count(collection.slug).toLocaleString()} jobs
          </span>
        </div>
        <p class="mt-2 text-sm leading-relaxed text-muted-foreground">{collection.description}</p>
      </a>
    {/each}
  </div>
</div>
