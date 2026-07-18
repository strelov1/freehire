<script lang="ts">
  import { page } from '$app/state';
  import Seo from '$lib/components/Seo.svelte';
  import InsightsPageShell from '$lib/components/InsightsPageShell.svelte';
  import { breadcrumbJsonLd, datasetJsonLd, jsonLdScript } from '$lib/seo';
  import { formatSalary, seniorityLabel } from '$lib/insights';
  import type { PageData } from './$types';

  let { data }: { data: PageData } = $props();

  const origin = $derived(page.url.origin);
  const canonical = $derived(`${origin}/insights/salary/${data.category}`);
  const title = $derived(`${data.label} Salaries · freehire`);
  const updated = $derived(
    new Date().toLocaleDateString('en-US', { year: 'numeric', month: 'long', day: 'numeric' }),
  );
  const jsonLd = $derived(
    jsonLdScript([
      datasetJsonLd(`${data.label} salaries`, data.intro, canonical, origin),
      breadcrumbJsonLd([
        { name: 'freehire', url: `${origin}/` },
        { name: 'Insights', url: `${origin}/insights` },
        { name: `${data.label} Salaries`, url: canonical },
      ]),
    ]),
  );
</script>

<Seo {title} description={data.intro} {canonical} />
<svelte:head>
  <!-- eslint-disable-next-line svelte/no-at-html-tags -- non-executable JSON-LD from jsonLdScript, which escapes `<` -->
  {@html jsonLd}
</svelte:head>

<InsightsPageShell
  category={data.category}
  label={data.label}
  kind="salary"
  {title}
  intro={data.intro}
  {updated}
  covered={data.covered}
>
  {#if data.bands.length === 0}
    <p class="text-muted-foreground">No salary figures disclosed for this category yet.</p>
  {:else}
    <table class="w-full border-collapse text-left text-sm">
      <thead>
        <tr class="border-b border-border text-muted-foreground">
          <th class="py-2 pr-4 font-medium">Level</th>
          <th class="py-2 pr-4 font-medium">Currency</th>
          <th class="py-2 pr-4 font-medium">Period</th>
          <th class="py-2 pr-4 font-medium text-right">25th</th>
          <th class="py-2 pr-4 font-medium text-right">Median</th>
          <th class="py-2 pr-4 font-medium text-right">75th</th>
          <th class="py-2 font-medium text-right">Postings</th>
        </tr>
      </thead>
      <tbody>
        {#each data.bands as b (b.seniority + b.currency + b.period)}
          <tr class="border-b border-border">
            <td class="py-2 pr-4 font-medium text-foreground">{seniorityLabel(b.seniority)}</td>
            <td class="py-2 pr-4 text-muted-foreground">{b.currency.toUpperCase()}</td>
            <td class="py-2 pr-4 text-muted-foreground">{b.period}</td>
            <td class="py-2 pr-4 text-right tabular-nums">{formatSalary(b.p25, b.currency)}</td>
            <td class="py-2 pr-4 text-right font-semibold tabular-nums">{formatSalary(b.p50, b.currency)}</td>
            <td class="py-2 pr-4 text-right tabular-nums">{formatSalary(b.p75, b.currency)}</td>
            <td class="py-2 text-right tabular-nums text-muted-foreground">{b.sample_size}</td>
          </tr>
        {/each}
      </tbody>
    </table>
  {/if}
</InsightsPageShell>
