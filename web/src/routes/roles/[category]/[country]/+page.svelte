<script lang="ts">
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import JobsView from '$lib/components/JobsView.svelte';
  import Seo from '$lib/components/Seo.svelte';
  import { formatSalary } from '$lib/insights';
  import { categoryLabel, SENIORITY_LABELS, titleCase } from '$lib/labels';
  import { breadcrumbJsonLd, collectionPageJsonLd, jobListItems, jsonLdScript } from '$lib/seo';
  import { MIN_ENGLISH_COVERAGE } from '$lib/roleLandings';
  import { Card, CountryFlag, Table } from '$lib/ui';
  import type { PageData } from './$types';

  let { data }: { data: PageData } = $props();

  const origin = $derived(page.url.origin);
  const base = $derived(`${origin}/roles/${data.categorySlug}/${data.countrySlug}`);
  // Each paginated page is canonical to ITSELF, not to page one: pointing them all at
  // the first page tells Google the deeper rows duplicate it, which is how the rows
  // those pages exist to expose stop being indexed.
  const canonical = $derived(data.pageNumber > 1 ? `${base}?page=${data.pageNumber}` : base);

  const what = $derived(categoryLabel(data.category));
  const where = $derived(data.countryLabel);
  const heading = $derived(`${what} Jobs in ${where}`);
  const pageTitle = $derived(
    data.pageNumber > 1
      ? `${heading} — page ${data.pageNumber} · freehire`
      : `${heading} · freehire`,
  );

  /** Facet distribution → the rows a strip renders, biggest first. */
  const strip = (counts: Record<string, number>, limit = 6) =>
    Object.entries(counts)
      .sort((a, b) => b[1] - a[1])
      .slice(0, limit);

  const skills = $derived(strip(data.skills, 8));
  const seniority = $derived(strip(data.seniority));
  const workMode = $derived(strip(data.workMode));
  const companySize = $derived(strip(data.companySize));
  // The share of the pair that sponsors, among the postings that say either way.
  const visaDeclared = $derived((data.visa.true ?? 0) + (data.visa.false ?? 0));

  const share = (n: number, of: number) => (of > 0 ? `${Math.round((n / of) * 100)}%` : '—');
  const count = (n: number) => n.toLocaleString('en-US');

  const jsonLd = $derived(
    jsonLdScript([
      collectionPageJsonLd(heading, data.intro, canonical, jobListItems(data.initial.items, origin)),
      breadcrumbJsonLd([
        { name: 'freehire', url: `${origin}/` },
        { name: 'Roles', url: `${origin}/roles` },
        { name: what, url: `${origin}/roles/${data.categorySlug}` },
        { name: heading, url: base },
      ]),
    ]),
  );
</script>

<Seo title={pageTitle} description={data.intro} {canonical} />
<svelte:head>
  <!-- eslint-disable-next-line svelte/no-at-html-tags -- non-executable JSON-LD from jsonLdScript, which escapes `<` -->
  {@html jsonLd}
</svelte:head>

<div class="mx-auto w-full max-w-6xl px-4 py-6">
  <nav class="mb-4 flex items-center gap-2 text-sm text-muted-foreground" aria-label="Breadcrumb">
    <a href={resolve('/')} class="hover:underline">freehire</a>
    <span>/</span>
    <a href={resolve('/roles')} class="hover:underline">Roles</a>
    <span>/</span>
    <a href={resolve('/roles/[category]', { category: data.categorySlug })} class="hover:underline">
      {what}
    </a>
  </nav>

  <header class="mb-8">
    <h1 class="flex items-center gap-2.5 text-2xl font-semibold tracking-tight">
      <CountryFlag code={data.countryCode} label={where} />
      {heading}
    </h1>
    <p class="mt-2 max-w-3xl text-sm leading-relaxed text-muted-foreground">{data.intro}</p>
  </header>

  <div class="mb-8 grid gap-4 md:grid-cols-2">
    {#if data.salaryBands.length > 0}
      <Card class="p-4">
        <h2 class="mb-3 text-sm font-semibold">Salary</h2>
        <Table>
          {#snippet header()}
            <tr class="border-b border-border text-muted-foreground">
              <th class="py-2 pr-4 text-left font-medium">Currency</th>
              <th class="py-2 pr-4 text-left font-medium">Period</th>
              <th class="py-2 pr-4 text-right font-medium">25th</th>
              <th class="py-2 pr-4 text-right font-medium">Median</th>
              <th class="py-2 pr-4 text-right font-medium">75th</th>
              <th class="py-2 text-right font-medium">Postings</th>
            </tr>
          {/snippet}
          {#each data.salaryBands as band (`${band.currency}-${band.period}`)}
            <tr class="border-b border-border/50">
              <td class="py-2 pr-4">{band.currency}</td>
              <td class="py-2 pr-4">{band.period}</td>
              <td class="py-2 pr-4 text-right">{formatSalary(band.p25, band.currency)}</td>
              <td class="py-2 pr-4 text-right font-medium">{formatSalary(band.p50, band.currency)}</td>
              <td class="py-2 pr-4 text-right">{formatSalary(band.p75, band.currency)}</td>
              <td class="py-2 text-right text-muted-foreground">{count(band.sample_size)}</td>
            </tr>
          {/each}
        </Table>
        <!-- The sample is stated beside the figure, never implied: a median over 21
             postings and one over 3,197 render identically otherwise. -->
        <p class="mt-2 text-xs text-muted-foreground">
          From postings that disclose pay. Currencies are counted separately, never converted.
        </p>
      </Card>
    {/if}

    {#if skills.length > 0}
      <Card class="p-4">
        <h2 class="mb-3 text-sm font-semibold">Most requested skills</h2>
        <ul class="flex flex-wrap gap-2">
          {#each skills as [skill, n] (skill)}
            <li class="rounded-full bg-secondary px-3 py-1 text-sm text-secondary-foreground">
              {skill}
              <span class="text-muted-foreground">{share(n, data.total)}</span>
            </li>
          {/each}
        </ul>
      </Card>
    {/if}

    {#if workMode.length > 0}
      <Card class="p-4">
        <h2 class="mb-3 text-sm font-semibold">How the work is done</h2>
        <ul class="space-y-1.5 text-sm">
          {#each workMode as [mode, n] (mode)}
            <li class="flex justify-between">
              <span>{titleCase(mode)}</span>
              <span class="text-muted-foreground">{count(n)} · {share(n, data.total)}</span>
            </li>
          {/each}
        </ul>
        {#if visaDeclared > 0}
          <p class="mt-3 border-t border-border pt-3 text-sm">
            Visa sponsorship offered in
            <strong>{share(data.visa.true ?? 0, visaDeclared)}</strong>
            of the {count(visaDeclared)} postings that state a position on it.
          </p>
        {/if}
      </Card>
    {/if}

    {#if seniority.length > 0}
      <Card class="p-4">
        <h2 class="mb-3 text-sm font-semibold">Seniority</h2>
        <ul class="space-y-1.5 text-sm">
          {#each seniority as [level, n] (level)}
            <li class="flex justify-between">
              <span>{SENIORITY_LABELS[level] ?? titleCase(level)}</span>
              <span class="text-muted-foreground">{count(n)}</span>
            </li>
          {/each}
        </ul>
      </Card>
    {/if}

    {#if companySize.length > 0}
      <Card class="p-4">
        <h2 class="mb-3 text-sm font-semibold">Who is hiring</h2>
        <ul class="space-y-1.5 text-sm">
          {#each companySize as [size, n] (size)}
            <li class="flex justify-between">
              <span>{size} employees</span>
              <span class="text-muted-foreground">{count(n)}</span>
            </li>
          {/each}
        </ul>
      </Card>
    {/if}

    <!-- Rendered only where enough postings declare a level; below the floor the
         distribution describes which postings got annotated, not the market. -->
    {#if data.english}
      <Card class="p-4">
        <h2 class="mb-3 text-sm font-semibold">English level</h2>
        <ul class="space-y-1.5 text-sm">
          {#each data.english as row (row.level)}
            <li class="flex justify-between">
              <span>{row.level.toUpperCase()}</span>
              <span class="text-muted-foreground">{Math.round(row.share * 100)}%</span>
            </li>
          {/each}
        </ul>
        <p class="mt-2 text-xs text-muted-foreground">
          Share of the postings that state a required level (at least
          {Math.round(MIN_ENGLISH_COVERAGE * 100)}% of this market do).
        </p>
      </Card>
    {/if}
  </div>

  {#key `${data.categorySlug}/${data.countrySlug}`}
    <JobsView
      initial={data.initial}
      scope={data.scope}
      excludeFacets={['category', 'countries']}
      currentPage={data.pageNumber}
    />
  {/key}

  <footer class="mt-10 space-y-6 border-t border-border pt-6 text-sm">
    {#if data.otherCountries.length > 0}
      <section>
        <h2 class="mb-2 font-semibold">{what} jobs in other countries</h2>
        <ul class="flex flex-wrap gap-2">
          {#each data.otherCountries as c (c.code)}
            <li>
              <a
                href={resolve('/roles/[category]/[country]', {
                  category: data.categorySlug,
                  country: c.slug,
                })}
                class="rounded-full bg-secondary px-3 py-1 text-secondary-foreground hover:bg-accent"
              >
                {c.label}
                <span class="text-muted-foreground">{count(c.openCount)}</span>
              </a>
            </li>
          {/each}
        </ul>
      </section>
    {/if}

    {#if data.otherCategories.length > 0}
      <section>
        <h2 class="mb-2 font-semibold">Other roles hiring in {where}</h2>
        <ul class="flex flex-wrap gap-2">
          {#each data.otherCategories as c (c.category)}
            <li>
              <a
                href={resolve('/roles/[category]/[country]', {
                  category: c.slug,
                  country: data.countrySlug,
                })}
                class="rounded-full bg-secondary px-3 py-1 text-secondary-foreground hover:bg-accent"
              >
                {c.label}
                <span class="text-muted-foreground">{count(c.openCount)}</span>
              </a>
            </li>
          {/each}
        </ul>
      </section>
    {/if}
  </footer>
</div>
