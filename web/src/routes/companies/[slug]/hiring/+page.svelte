<script lang="ts">
  import { page } from '$app/state';
  import { invalidateAll } from '$app/navigation';
  import { resolve } from '$app/paths';
  import ActivityPanel from '#lib/components/ActivityPanel.svelte';
  import CompanyHeader from '#lib/components/CompanyHeader.svelte';
  import CompanyTabs from '#lib/components/CompanyTabs.svelte';
  import Seo from '#lib/components/Seo.svelte';
  import { api } from '#lib/api.js';
  import {
    breakdownLabel,
    formatGrowth,
    hiringRobots,
    replyRate,
    salaryRange,
    topEntries,
  } from '#lib/companyHiring.js';
  import { formatSalary } from '#lib/insights.js';
  import { breadcrumbJsonLd, datasetJsonLd, jsonLdScript } from '#lib/seo.js';
  import { Breadcrumbs, Card } from '#lib/ui/index.js';
  import type { PageData } from './$types';

  let { data }: { data: PageData } = $props();

  const origin = $derived(page.url.origin);
  const companyHref = $derived(resolve('/companies/[slug]', { slug: data.slug }));
  const canonical = $derived(`${origin}${companyHref}/hiring`);
  const name = $derived(data.company.name);
  const description = $derived(
    `How ${name} hires: open roles and their 30-day change, how often applications get a reply, ` +
      `how long postings stay open, and who they hire.`,
  );
  const breadcrumbItems = $derived([
    { name: 'Companies', href: '/companies' },
    { name, href: companyHref },
    { name: 'Hiring' },
  ]);
  const jsonLd = $derived(
    jsonLdScript([
      datasetJsonLd(`${name} hiring activity`, description, canonical, origin, [
        { name: 'Company hiring API', contentUrl: `${origin}/api/v1/companies/${data.slug}/hiring` },
      ]),
      breadcrumbJsonLd(
        breadcrumbItems.map((item) => ({ name: item.name, url: item.href ? `${origin}${item.href}` : canonical })),
      ),
    ]),
  );

  // "Who they hire": each block names one facet of the company's OPEN postings, and each
  // value links into the Jobs tab filtered by it — the number is a door, not a dead end.
  const BREAKDOWNS = [
    { param: 'category', title: 'Roles', limit: 6 },
    { param: 'seniority', title: 'Seniority', limit: 6 },
    { param: 'countries', title: 'Countries', limit: 6 },
    { param: 'work_mode', title: 'Work format', limit: 3 },
    { param: 'skills', title: 'Top skills', limit: 10 },
  ];
  const breakdowns = $derived(
    BREAKDOWNS.map((b) => ({ ...b, rows: topEntries(data.breakdown?.facets[b.param], b.limit) })).filter(
      (b) => b.rows.length > 0,
    ),
  );

  const salary = $derived(data.salary ? salaryRange(data.salary.stats, data.salary.total) : null);
</script>

<Seo
  title={`${name} hiring trends — open roles, reply rate · freehire`}
  {description}
  {canonical}
  robots={hiringRobots(data.hiring)}
  image={`${origin}${companyHref}/og.png`}
/>

<svelte:head>
  <!-- eslint-disable-next-line svelte/no-at-html-tags -- non-executable JSON-LD built by jsonLdScript, which escapes `<`; raw injection is the only way to emit a structured-data <script> -->
  {@html jsonLd}
</svelte:head>

<div class="mx-auto w-full max-w-6xl px-4 py-6">
  <Breadcrumbs items={breadcrumbItems} class="mb-4" />

  {#key data.slug}
    <CompanyHeader company={data.company} slug={data.slug} />

    <CompanyTabs slug={data.slug} active="hiring">
      <div class="mt-6 flex flex-col gap-8">
        {#if data.hiring}
          {@const hiring = data.hiring}
          <section aria-label="Key figures" class="grid gap-4 sm:grid-cols-3">
            <Card class="p-4">
              <p class="text-sm text-muted-foreground">Open roles</p>
              <p class="mt-1 text-2xl font-semibold tabular-nums">{hiring.open}</p>
              <p class="mt-1 text-sm text-muted-foreground">
                {formatGrowth(hiring.growth)} in the last 30 days
              </p>
            </Card>

            <Card class="p-4">
              <p class="text-sm text-muted-foreground">Reply rate</p>
              {#if data.response}
                <p class="mt-1 text-2xl font-semibold tabular-nums">{replyRate(data.response)}%</p>
                <p class="mt-1 text-sm text-muted-foreground">
                  of {data.response.applications} applications freehire members tracked here got a reply{#if data.response.median_reply_days !== undefined},
                    the first after {Math.round(data.response.median_reply_days)} days (median){/if}
                </p>
              {:else}
                <p class="mt-1 text-sm text-muted-foreground">Not enough data yet.</p>
              {/if}
            </Card>

            <Card class="p-4">
              <p class="text-sm text-muted-foreground">Time a posting stays open</p>
              {#if hiring.lifetime}
                <p class="mt-1 text-2xl font-semibold tabular-nums">
                  {Math.round(hiring.lifetime.median_days)} days
                </p>
                <p class="mt-1 text-sm text-muted-foreground">
                  median over {hiring.lifetime.closed} postings closed in the last 6 months
                </p>
              {:else}
                <p class="mt-1 text-sm text-muted-foreground">Not enough data yet.</p>
              {/if}
            </Card>
          </section>

          <section>
            <ActivityPanel
              initial={hiring.series}
              load={(g) => api.companyHiring(data.slug, g).then((h) => h.series)}
            >
              {#snippet title()}
                <h2 class="text-lg font-semibold tracking-tight">Postings added vs. removed</h2>
              {/snippet}
            </ActivityPanel>
          </section>
        {:else}
          <div class="py-16 text-center text-sm text-muted-foreground">
            Couldn't load hiring activity.
            <button type="button" onclick={() => invalidateAll()} class="ml-1 underline hover:text-foreground">
              Retry
            </button>
          </div>
        {/if}

        {#if breakdowns.length > 0}
          <section>
            <h2 class="text-lg font-semibold tracking-tight">Who they hire</h2>
            <p class="mt-1 text-sm text-muted-foreground">What the company's open postings ask for.</p>
            <div class="mt-4 grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
              {#each breakdowns as block (block.param)}
                <Card class="p-4">
                  <h3 class="text-sm font-medium">{block.title}</h3>
                  <ul class="mt-2 flex flex-col gap-1 text-sm">
                    {#each block.rows as row (row.value)}
                      <li class="flex items-baseline justify-between gap-3">
                        <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- the path IS resolve()d; the rule can't see through the appended filter query -->
                        <a class="truncate hover:underline" href={`${resolve('/companies/[slug]', { slug: data.slug })}?${new URLSearchParams({ [block.param]: row.value })}`}>
                          {breakdownLabel(block.param, row.value)}
                        </a>
                        <span class="tabular-nums text-muted-foreground">{row.count}</span>
                      </li>
                    {/each}
                  </ul>
                </Card>
              {/each}
            </div>
          </section>
        {/if}

        {#if data.salary && salary}
          <section>
            <h2 class="text-lg font-semibold tracking-tight">Salary</h2>
            <p class="mt-2 text-2xl font-semibold tabular-nums">
              {formatSalary(salary.min, data.salary.currency)} – {formatSalary(salary.max, data.salary.currency)}
              <span class="text-base font-normal text-muted-foreground">per year</span>
            </p>
            <p class="mt-1 text-sm text-muted-foreground">
              Stated in {data.salary.total} open postings paid in {data.salary.currency}.
            </p>
          </section>
        {/if}
      </div>
    </CompanyTabs>
  {/key}
</div>
