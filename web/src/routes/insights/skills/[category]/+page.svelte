<script lang="ts">
  import { page } from '$app/state';
  import Seo from '$lib/components/Seo.svelte';
  import InsightsPageShell from '$lib/components/InsightsPageShell.svelte';
  import { breadcrumbJsonLd, datasetJsonLd, jsonLdScript } from '$lib/seo';
  import type { PageData } from './$types';

  let { data }: { data: PageData } = $props();

  const origin = $derived(page.url.origin);
  const canonical = $derived(`${origin}/insights/skills/${data.category}`);
  const title = $derived(`Most In-Demand ${data.label} Skills · freehire`);
  const updated = $derived(
    new Date().toLocaleDateString('en-US', { year: 'numeric', month: 'long', day: 'numeric' }),
  );
  const jsonLd = $derived(
    jsonLdScript([
      datasetJsonLd(`In-demand ${data.label} skills`, data.intro, canonical, origin),
      breadcrumbJsonLd([
        { name: 'freehire', url: `${origin}/` },
        { name: 'Insights', url: `${origin}/insights` },
        { name: `${data.label} Skills`, url: canonical },
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
  kind="skills"
  {title}
  intro={data.intro}
  {updated}
  covered={data.covered}
>
  {#if data.skills.length === 0}
    <p class="text-muted-foreground">No skill data for this category yet.</p>
  {:else}
    <table class="w-full border-collapse text-left text-sm">
      <thead>
        <tr class="border-b border-border text-muted-foreground">
          <th class="py-2 pr-4 font-medium">#</th>
          <th class="py-2 pr-4 font-medium">Skill</th>
          <th class="py-2 pr-4 font-medium text-right">Open postings</th>
          <th class="py-2 font-medium text-right">30-day growth</th>
        </tr>
      </thead>
      <tbody>
        {#each data.skills as s, i (s.skill)}
          {@const growthTone =
            s.growth > 0
              ? 'text-green-600 dark:text-green-400'
              : s.growth < 0
                ? 'text-red-600 dark:text-red-400'
                : 'text-muted-foreground'}
          <tr class="border-b border-border">
            <td class="py-2 pr-4 text-muted-foreground tabular-nums">{i + 1}</td>
            <td class="py-2 pr-4 font-medium text-foreground">{s.skill}</td>
            <td class="py-2 pr-4 text-right tabular-nums">{s.open_count.toLocaleString('en-US')}</td>
            <td class="py-2 text-right tabular-nums {growthTone}">
              {s.growth > 0 ? '+' : ''}{s.growth.toLocaleString('en-US')}
            </td>
          </tr>
        {/each}
      </tbody>
    </table>
  {/if}
</InsightsPageShell>
