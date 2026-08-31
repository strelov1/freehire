<script lang="ts">
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import Seo from '$lib/components/Seo.svelte';
  import { MIN_PAIR_OPEN } from '$lib/roleLandings';
  import { breadcrumbJsonLd, jsonLdScript } from '$lib/seo';
  import { CountryFlag, Table } from '$lib/ui';
  import type { PageData } from './$types';

  let { data }: { data: PageData } = $props();

  const origin = $derived(page.url.origin);
  const canonical = $derived(`${origin}/roles/${data.categorySlug}`);
  const heading = $derived(`${data.label} Jobs by Country`);
  const count = (n: number) => n.toLocaleString('en-US');
  // "countries with at least N openings", not "countries hiring": the table lists only
  // what clears the publication gate, and a country with 40 postings is hiring and
  // absent here. Same rule as the salary and english blocks — say what is shown.
  const description = $derived(
    `${count(data.total)} open ${data.label} jobs on freehire, across the ${data.countries.length} countries with at least ${MIN_PAIR_OPEN} of them.`,
  );
  // The category's own feed. A query string on a resolve()d base — there is no
  // dynamic route segment here for resolve() to fill.
  const jobsHref = $derived(`${resolve('/jobs')}?category=${encodeURIComponent(data.category)}`);

  const jsonLd = $derived(
    jsonLdScript([
      breadcrumbJsonLd([
        { name: 'freehire', url: `${origin}/` },
        { name: 'Roles', url: `${origin}/roles` },
        { name: data.label, url: canonical },
      ]),
    ]),
  );
</script>

<Seo title={`${heading} · freehire`} {description} {canonical} />
<svelte:head>
  <!-- eslint-disable-next-line svelte/no-at-html-tags -- non-executable JSON-LD from jsonLdScript, which escapes `<` -->
  {@html jsonLd}
</svelte:head>

<div class="mx-auto w-full max-w-4xl px-4 py-6">
  <nav class="mb-4 flex items-center gap-2 text-sm text-muted-foreground" aria-label="Breadcrumb">
    <a href={resolve('/')} class="hover:underline">freehire</a>
    <span>/</span>
    <a href={resolve('/roles')} class="hover:underline">Roles</a>
  </nav>

  <header class="mb-8">
    <h1 class="text-2xl font-semibold tracking-tight">{heading}</h1>
    <p class="mt-2 max-w-2xl text-sm leading-relaxed text-muted-foreground">{description}</p>
    <!-- The feed lives at /collections; this page is the map. Linking it keeps the
         two from reading as rivals for the same query. -->
    <p class="mt-3 text-sm">
      <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- internal /jobs filter link; resolve()d base plus a query string, no dynamic route to resolve -->
      <a href={jobsHref} class="text-primary hover:underline">Browse all {data.label} jobs →</a>
    </p>
  </header>

  <Table>
    {#snippet header()}
      <tr class="border-b border-border text-muted-foreground">
        <th class="py-2 pr-4 text-left font-medium">Country</th>
        <th class="py-2 text-right font-medium">Open jobs</th>
      </tr>
    {/snippet}
    {#each data.countries as c (c.code)}
      <tr class="border-b border-border/50">
        <td class="py-2 pr-4">
          <a
            href={resolve('/roles/[category]/[country]', {
              category: data.categorySlug,
              country: c.slug,
            })}
            class="flex items-center gap-2 text-primary hover:underline"
          >
            <CountryFlag code={c.code} label={c.label} />
            {c.label}
          </a>
        </td>
        <td class="py-2 text-right tabular-nums">{count(c.openCount)}</td>
      </tr>
    {/each}
  </Table>

  <footer class="mt-10 border-t border-border pt-6 text-sm">
    <h2 class="mb-2 font-semibold">Other roles</h2>
    <ul class="flex flex-wrap gap-2">
      {#each data.siblings as s (s.category)}
        <li>
          <a
            href={resolve('/roles/[category]', { category: s.slug })}
            class="rounded-full bg-secondary px-3 py-1 text-secondary-foreground hover:bg-accent"
          >
            {s.label}
          </a>
        </li>
      {/each}
    </ul>
  </footer>
</div>
