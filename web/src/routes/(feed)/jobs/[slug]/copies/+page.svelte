<script lang="ts">
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import Seo from '$lib/components/Seo.svelte';
  import type { PageData } from './$types';

  let { data }: { data: PageData } = $props();

  const from = $derived(data.total === 0 ? 0 : data.offset + 1);
  const to = $derived(data.offset + data.copies.length);
  const hasPrev = $derived(data.offset > 0);
  const hasNext = $derived(data.offset + data.pageSize < data.total);
  const copiesHref = $derived(resolve('/jobs/[slug]/copies', { slug: data.job.public_slug }));
  const prevHref = $derived(`${copiesHref}?offset=${Math.max(0, data.offset - data.pageSize)}`);
  const nextHref = $derived(`${copiesHref}?offset=${data.offset + data.pageSize}`);
  const canonical = $derived(`${page.url.origin}${copiesHref}`);
</script>

<Seo title={`${data.job.title} — locations`} {canonical} />
<svelte:head>
  <!-- A navigation aggregate over the per-city job pages (which are themselves
       indexable); the list page adds no unique content, so keep it out of the index. -->
  <meta name="robots" content="noindex, follow" />
</svelte:head>

<div class="mx-auto w-full max-w-3xl px-4 py-6">
  <a
    href={resolve('/jobs/[slug]', { slug: data.job.public_slug })}
    class="text-sm text-gray-500 hover:text-gray-800">← {data.job.title}</a
  >

  <h1 class="mt-2 text-xl font-semibold">{data.total} openings across locations</h1>
  <p class="mt-1 text-sm text-gray-500">{data.job.title}</p>

  <ul class="mt-5 divide-y divide-gray-100 overflow-hidden rounded-lg border border-gray-100">
    {#each data.copies as copy (copy.public_slug)}
      <li>
        <a
          href={resolve('/jobs/[slug]', { slug: copy.public_slug })}
          class="flex items-center justify-between px-4 py-2.5 text-sm hover:bg-gray-50"
        >
          <span class="text-gray-800">{copy.location || 'Location not specified'}</span>
          <span class="text-xs text-gray-400">View →</span>
        </a>
      </li>
    {/each}
  </ul>

  {#if hasPrev || hasNext}
    <nav class="mt-5 flex items-center justify-between text-sm">
      {#if hasPrev}
        <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- copiesHref is already resolve()'d; only a query-string offset is appended, which the linter can't trace through the variable -->
        <a href={prevHref} class="font-medium text-blue-600 hover:text-blue-700" data-sveltekit-noscroll
          >← Previous</a
        >
      {:else}
        <span></span>
      {/if}
      <span class="text-gray-500">{from}–{to} of {data.total}</span>
      {#if hasNext}
        <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- copiesHref is already resolve()'d; only a query-string offset is appended, which the linter can't trace through the variable -->
        <a href={nextHref} class="font-medium text-blue-600 hover:text-blue-700" data-sveltekit-noscroll
          >Next →</a
        >
      {:else}
        <span></span>
      {/if}
    </nav>
  {/if}
</div>
