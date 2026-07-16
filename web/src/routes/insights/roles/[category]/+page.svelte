<script lang="ts">
  import { page } from '$app/state';
  import Seo from '$lib/components/Seo.svelte';
  import InsightsPageShell from '$lib/components/InsightsPageShell.svelte';
  import { breadcrumbJsonLd, datasetJsonLd, jsonLdScript } from '$lib/seo';
  import { seniorityLabel } from '$lib/insights';
  import type { PageData } from './$types';

  let { data }: { data: PageData } = $props();

  const origin = $derived(page.url.origin);
  const canonical = $derived(`${origin}/insights/roles/${data.category}`);
  const title = $derived(`Most-Hiring ${data.label} Roles · freehire`);
  const updated = $derived(
    new Date().toLocaleDateString('en-US', { year: 'numeric', month: 'long', day: 'numeric' }),
  );
  const jsonLd = $derived(
    jsonLdScript([
      datasetJsonLd(`${data.label} roles by demand`, data.intro, canonical, origin),
      breadcrumbJsonLd([
        { name: 'freehire', url: `${origin}/` },
        { name: 'Insights', url: `${origin}/insights` },
        { name: `${data.label} Roles`, url: canonical },
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
  kind="roles"
  {title}
  intro={data.intro}
  {updated}
  covered={data.covered}
>
  {#if data.roles.length === 0}
    <p class="text-gray-500">No role data for this category yet.</p>
  {:else}
    <table class="w-full border-collapse text-left text-sm">
      <thead>
        <tr class="border-b border-gray-300 text-gray-500">
          <th class="py-2 pr-4 font-medium">Level</th>
          <th class="py-2 pr-4 font-medium text-right">Open roles</th>
          <th class="py-2 font-medium text-right">30-day growth</th>
        </tr>
      </thead>
      <tbody>
        {#each data.roles as r (r.seniority)}
          <tr class="border-b border-gray-100">
            <td class="py-2 pr-4 font-medium text-gray-900">{seniorityLabel(r.seniority)}</td>
            <td class="py-2 pr-4 text-right tabular-nums">{r.open_count.toLocaleString('en-US')}</td>
            <td
              class="py-2 text-right tabular-nums"
              class:text-green-600={r.growth > 0}
              class:text-gray-400={r.growth === 0}
              class:text-red-600={r.growth < 0}
            >
              {r.growth > 0 ? '+' : ''}{r.growth.toLocaleString('en-US')}
            </td>
          </tr>
        {/each}
      </tbody>
    </table>
  {/if}
</InsightsPageShell>
