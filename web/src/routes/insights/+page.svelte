<script lang="ts">
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import Seo from '$lib/components/Seo.svelte';
  import { breadcrumbJsonLd, jsonLdScript } from '$lib/seo';
  import type { PageData } from './$types';

  let { data }: { data: PageData } = $props();

  const origin = $derived(page.url.origin);
  const canonical = $derived(`${origin}/insights`);
  const description =
    'Aggregate job-market data from freehire: salaries, in-demand skills, and hiring demand for every tech category.';
  const jsonLd = $derived(
    jsonLdScript([
      breadcrumbJsonLd([
        { name: 'freehire', url: `${origin}/` },
        { name: 'Insights', url: canonical },
      ]),
    ]),
  );
</script>

<Seo title="Job Market Insights · freehire" {description} {canonical} />
<svelte:head>
  <!-- eslint-disable-next-line svelte/no-at-html-tags -- non-executable JSON-LD from jsonLdScript, which escapes `<` -->
  {@html jsonLd}
</svelte:head>

<div class="mx-auto w-full max-w-4xl px-4 py-8">
  <h1 class="text-3xl font-bold tracking-tight text-foreground">Job Market Insights</h1>
  <p class="mt-3 text-lg text-muted-foreground">{description}</p>
  <p class="mt-1 text-sm text-muted-foreground">
    Aggregated from open postings on freehire, refreshed through the day. All data is
    also available on the <a href={resolve('/docs/api')} class="text-blue-600 hover:underline">open API</a>.
  </p>

  <a
    href={resolve('/insights/companies')}
    class="mt-6 flex items-center justify-between rounded-lg border border-border p-4 hover:bg-accent"
  >
    <span>
      <span class="text-lg font-semibold text-foreground">Company hiring signal</span>
      <span class="mt-0.5 block text-sm text-muted-foreground">Which companies are ramping up or slowing down hiring</span>
    </span>
    <span aria-hidden="true" class="text-muted-foreground">→</span>
  </a>

  {#if data.covered.length === 0}
    <p class="mt-8 text-muted-foreground">Insights are being computed — check back shortly.</p>
  {:else}
    <div class="mt-8 grid gap-4 sm:grid-cols-2">
      {#each data.covered as c (c.category)}
        <div class="rounded-lg border border-border p-4">
          <h2 class="text-lg font-semibold text-foreground">{c.label}</h2>
          <p class="mt-0.5 text-xs text-muted-foreground">{c.openCount.toLocaleString('en-US')} open roles</p>
          <div class="mt-2 flex flex-wrap gap-x-4 gap-y-1 text-sm">
            <a href={resolve('/insights/salary/[category]', { category: c.category })} class="text-blue-600 hover:underline">Salaries</a>
            <a href={resolve('/insights/skills/[category]', { category: c.category })} class="text-blue-600 hover:underline">Skills</a>
            <a href={resolve('/insights/roles/[category]', { category: c.category })} class="text-blue-600 hover:underline">Roles</a>
          </div>
        </div>
      {/each}
    </div>
  {/if}
</div>
