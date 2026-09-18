<script lang="ts">
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import Seo from '$lib/components/Seo.svelte';
  import InsightsPageShell from '$lib/components/InsightsPageShell.svelte';
  import { breadcrumbJsonLd, datasetJsonLd, jsonLdScript } from '$lib/seo';
  import { Table } from '$lib/ui';
  import type { PageData } from './$types';

  let { data }: { data: PageData } = $props();

  const origin = $derived(page.url.origin);
  const canonical = $derived(`${origin}/insights/roles/${data.category}/${data.seniority}`);
  const heading = $derived(`What ${data.roleName} Jobs Ask For`);
  const title = $derived(`${heading} · freehire`);
  const updated = $derived(
    new Date().toLocaleDateString('en-US', { year: 'numeric', month: 'long', day: 'numeric' }),
  );

  const skills = $derived(data.role.skills ?? []);
  const sample = $derived(data.role.sample_size ?? 0);
  const coverage = $derived(data.role.coverage);

  // How the caller stands on each skill, keyed by skill so the table can annotate the
  // ranked rows in place rather than repeating the list.
  const held = $derived(new Set(coverage?.matched ?? []));
  const via = $derived(new Map((coverage?.adjacent ?? []).map((a) => [a.name, a.via])));

  // The filter that reproduces this page's population in the feed.
  const jobsQuery = $derived(
    new URLSearchParams({ category: data.category, seniority: data.seniority }).toString(),
  );

  const jsonLd = $derived(
    jsonLdScript([
      datasetJsonLd(`Skills asked for in ${data.roleName} jobs`, data.intro, canonical, origin),
      breadcrumbJsonLd([
        { name: 'freehire', url: `${origin}/` },
        { name: 'Insights', url: `${origin}/insights` },
        { name: `${data.label} Roles`, url: `${origin}/insights/roles/${data.category}` },
        { name: data.roleName, url: canonical },
      ]),
    ]),
  );

  function pct(share: number): string {
    return `${Math.round(share * 100)}%`;
  }
</script>

<!-- A role below the demand floor is served (the job page links here from any posting
     carrying both facets, and a link into a 404 is the failure the gate exists to avoid)
     but not indexed — a thin page in the index is the other failure. -->
<Seo
  {title}
  description={data.intro}
  {canonical}
  robots={data.thin ? 'noindex, follow' : undefined}
/>
<svelte:head>
  <!-- eslint-disable-next-line svelte/no-at-html-tags -- non-executable JSON-LD from jsonLdScript, which escapes `<` -->
  {@html jsonLd}
</svelte:head>

<InsightsPageShell
  category={data.category}
  label={data.label}
  kind="roles"
  {heading}
  intro={data.intro}
  {updated}
  covered={data.covered}
>
  <p class="mb-4 text-sm text-muted-foreground">
    {data.role.open_count.toLocaleString('en-US')} open {data.roleName} postings.
    {#if sample > 0}
      The percentages below are measured over the {sample.toLocaleString('en-US')} of them
      that list skills at all.
    {/if}
  </p>

  {#if skills.length === 0}
    <p class="text-muted-foreground">
      Not enough of these postings list skills yet to rank what they ask for.
    </p>
  {:else}
    <Table>
      {#snippet header()}
        <tr class="border-b border-border text-muted-foreground">
          <th class="py-2 pr-4 text-left font-medium">Skill</th>
          <th class="py-2 pr-4 font-medium text-right">Mentioned in</th>
          {#if coverage}
            <th class="py-2 font-medium text-left">You</th>
          {/if}
        </tr>
      {/snippet}
      {#each skills as s (s.skill)}
        <tr class="border-b border-border">
          <td class="py-2 pr-4 font-medium text-foreground">{s.skill}</td>
          <td class="py-2 pr-4 text-right tabular-nums">{pct(s.share)}</td>
          {#if coverage}
            <td class="py-2">
              {#if held.has(s.skill)}
                <!-- No success/positive token exists in the design system, so the three
                     states read brand-strong / warning-strong / muted rather than raw
                     palette colours. Adding a token for one table is not this change. -->
                <span class="text-brand-strong">You have this</span>
              {:else if via.has(s.skill)}
                <span class="text-warning-strong">Close — you have {via.get(s.skill)}</span>
              {:else}
                <span class="text-muted-foreground">Not on your profile</span>
              {/if}
            </td>
          {/if}
        </tr>
      {/each}
    </Table>

    <!-- "Mentioned in", never "required by": jobs.skills tags a skill named anywhere in
         a posting, including in a nice-to-have list or a stack blurb, so every share here
         is an upper bound on what the role actually requires. -->
    <p class="mt-3 text-xs text-muted-foreground">
      "Mentioned in" counts postings that name the skill anywhere — including as a
      nice-to-have — so treat it as the ceiling, not the bar.
    </p>
  {/if}

  {#if coverage}
    <p class="mt-4 text-sm">
      Your profile covers <strong>{coverage.exact_count} of {coverage.total}</strong>
      of these skills outright{#if coverage.adjacent_count > 0}, plus {coverage.adjacent_count}
        you have a close match for{/if}.
    </p>
  {/if}

  <p class="mt-4">
    <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- internal /jobs filter link; resolve()d base plus a query string, no dynamic route to resolve -->
    <a class="text-primary hover:underline" href={`${resolve('/jobs')}?${jobsQuery}`}>
      Browse {data.role.open_count.toLocaleString('en-US')} open {data.roleName} jobs →
    </a>
  </p>

  {#if data.categorySkills.length > 0}
    <!-- The category-wide list beside the role's. Only 39% of open technical postings
         state a seniority, so the role slice above is a minority of the market; these
         figures cover 98.7% of it and are what stop the narrower number standing alone. -->
    <section class="mt-8">
      <h2 class="mb-2 text-lg font-semibold">Across all {data.label} levels</h2>
      <p class="mb-3 text-sm text-muted-foreground">
        Most {data.label} postings never state a level. This wider list covers them too.
      </p>
      <ul class="flex flex-wrap gap-2">
        {#each data.categorySkills as s (s.skill)}
          <li class="rounded-full border border-border px-3 py-1 text-sm">
            {s.skill}
            <span class="text-muted-foreground tabular-nums">
              {s.open_count.toLocaleString('en-US')}
            </span>
          </li>
        {/each}
      </ul>
    </section>
  {/if}
</InsightsPageShell>
