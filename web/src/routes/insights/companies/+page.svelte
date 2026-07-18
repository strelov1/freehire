<script lang="ts">
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import Seo from '$lib/components/Seo.svelte';
  import { breadcrumbJsonLd, datasetJsonLd, jsonLdScript } from '$lib/seo';
  import type { InsightCompany } from '$lib/api';
  import type { PageData } from './$types';

  let { data }: { data: PageData } = $props();

  const origin = $derived(page.url.origin);
  const canonical = $derived(`${origin}/insights/companies`);
  const title = 'Company Hiring Signal · freehire';
  const description =
    'Which companies are ramping up or slowing down hiring — ranked by the 30-day change in their number of open jobs across the freehire catalogue.';
  const updated = $derived(
    new Date().toLocaleDateString('en-US', { year: 'numeric', month: 'long', day: 'numeric' }),
  );
  const jsonLd = $derived(
    jsonLdScript([
      datasetJsonLd('Company hiring signal', description, canonical, origin),
      breadcrumbJsonLd([
        { name: 'freehire', url: `${origin}/` },
        { name: 'Insights', url: `${origin}/insights` },
        { name: 'Companies', url: canonical },
      ]),
    ]),
  );
</script>

<Seo {title} {description} {canonical} />
<svelte:head>
  <!-- eslint-disable-next-line svelte/no-at-html-tags -- non-executable JSON-LD from jsonLdScript, which escapes `<` -->
  {@html jsonLd}
</svelte:head>

{#snippet board(heading: string, blurb: string, rows: InsightCompany[])}
  <section class="rounded-lg border border-gray-200 p-4 sm:p-5">
    <h2 class="text-lg font-semibold text-gray-900">{heading}</h2>
    <p class="mt-0.5 text-xs text-gray-500">{blurb}</p>
    {#if rows.length === 0}
      <p class="mt-4 text-sm text-gray-500">Not enough data yet.</p>
    {:else}
      <table class="mt-3 w-full border-collapse text-left text-sm">
        <thead>
          <tr class="border-b border-gray-300 text-gray-500">
            <th class="py-2 pr-3 font-medium">Company</th>
            <th class="py-2 pr-3 text-right font-medium">Open</th>
            <th class="py-2 text-right font-medium">30-day</th>
          </tr>
        </thead>
        <tbody>
          {#each rows as c, i (c.company_slug)}
            <tr class="border-b border-gray-100">
              <td class="py-2 pr-3">
                <span class="mr-1.5 tabular-nums text-gray-400">{i + 1}.</span>
                <a
                  href={resolve('/companies/[slug]', { slug: c.company_slug })}
                  class="font-medium text-gray-900 hover:text-blue-600 hover:underline"
                >
                  {c.company_name}
                </a>
              </td>
              <td class="py-2 pr-3 text-right tabular-nums text-gray-700">
                {c.open_now.toLocaleString('en-US')}
              </td>
              <td
                class="py-2 text-right font-medium tabular-nums"
                class:text-green-600={c.growth_30d > 0}
                class:text-gray-400={c.growth_30d === 0}
                class:text-red-600={c.growth_30d < 0}
              >
                {c.growth_30d > 0 ? '+' : ''}{c.growth_30d.toLocaleString('en-US')}
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    {/if}
  </section>
{/snippet}

<div class="mx-auto w-full max-w-4xl px-4 py-8">
  <nav class="text-sm text-gray-500">
    <a href={resolve('/insights')} class="hover:underline">Insights</a>
    <span class="mx-1">/</span>
    <span class="text-gray-700">Companies</span>
  </nav>

  <h1 class="mt-3 text-3xl font-bold tracking-tight text-gray-900">Company Hiring Signal</h1>
  <p class="mt-3 text-lg text-gray-600">{description}</p>
  <p class="mt-1 text-sm text-gray-500">
    Change is the difference between open jobs now and 30 days ago. Refreshed daily; also on the
    <a href={resolve('/docs/api')} class="text-blue-600 hover:underline">open API</a>.
    <span class="text-gray-400">Updated {updated}.</span>
  </p>

  <div class="mt-8 grid gap-4 md:grid-cols-2">
    {@render board('Ramping up', 'Biggest 30-day increase in open jobs', data.ramping)}
    {@render board('Slowing down', 'Biggest 30-day decrease in open jobs', data.freezing)}
  </div>
</div>
