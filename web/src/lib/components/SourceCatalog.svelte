<script lang="ts">
  import { resolve } from '$app/paths';
  import { locale } from '$lib/i18n/currentLocale.svelte';
  import { timeAgo } from '$lib/utils';
  // A source key is a search-facet code, so it renders through the one label map every
  // other surface uses: a source must not be "WhatJobs" on the filter panel and
  // "Whatjobs" here.
  import { sourceLabel } from '$lib/facets';
  import { SOURCE_LOGO_DOMAINS, sourceLogoUrl } from '$lib/logo';
  import { EntityLogo } from '$lib/ui';
  import type { ProviderKind, SourceEntry } from '$lib/types';

  // The presentational half of the /sources page: given the catalogue read (or null when
  // the API read failed), it renders the search box and the kind-grouped list. Kept
  // separate from the route so it can be previewed without a live API; the route owns
  // data loading and SEO.
  let { sources }: { sources: SourceEntry[] | null } = $props();

  let query = $state('');

  // The order the groups are read in: the platforms most of the catalogue comes through,
  // then the republishers, then single employers, then whatever is not a crawl adapter.
  const GROUPS: { kind: ProviderKind; title: string; blurb: string }[] = [
    {
      kind: 'ats',
      title: 'ATS platforms',
      blurb: 'Applicant tracking systems. One adapter reads the postings of many employers.',
    },
    {
      kind: 'aggregator',
      title: 'Aggregators',
      blurb: 'Boards that republish postings from many companies at once.',
    },
    {
      kind: 'company',
      title: 'Company career pages',
      blurb: "A single employer's own careers site, read directly.",
    },
    {
      kind: 'other',
      title: 'Other sources',
      blurb: 'Not crawl adapters — extraction pipelines and hand-curated intake.',
    },
  ];

  // What a source's headline count is: the DE-DUPLICATED figure, because that is what the
  // link next to it opens. Showing the raw count here would put a number on the card that
  // the search it links to contradicts. Null means the index could not be measured — a
  // different answer from zero, and rendered as one.
  function browsable(entry: SourceEntry): number | null {
    return entry.jobs?.browsable ?? null;
  }

  // Sorting needs one number, and an unmeasured source has none. It sorts last rather than
  // as a zero — "we do not know" is not "it is empty".
  function sortWeight(entry: SourceEntry): number {
    return browsable(entry) ?? entry.jobs?.open ?? -1;
  }

  const matches = $derived.by(() => {
    const q = query.trim().toLowerCase();
    const all = sources ?? [];
    if (!q) return all;
    // Match the key as well as the label: somebody who read `source=smartrecruiters` in a
    // URL should find it by typing exactly that.
    return all.filter(
      (s) => s.source.toLowerCase().includes(q) || sourceLabel(s.source).toLowerCase().includes(q)
    );
  });

  const grouped = $derived(
    GROUPS.map((group) => ({
      ...group,
      entries: matches
        .filter((s) => s.kind === group.kind)
        .sort((a, b) => sortWeight(b) - sortWeight(a)),
    })).filter((group) => group.entries.length > 0)
  );

  // The headline total sums only the sources whose count was actually MEASURED.
  //
  // `?? 0` here would be the one substitution the whole feature exists to prevent: an
  // unreachable search index leaves every `browsable` null, and a page that folds those
  // into a sum prints "0 jobs indexed" in its largest number while every card beneath it
  // correctly says "not measured yet". The per-card discipline has to reach the aggregate.
  // Narrowed to numbers BEFORE summing, so the reduce has no null to decide about and the
  // shape cannot express the bug it is guarding against.
  const measured = $derived(
    (sources ?? []).map((s) => s.jobs?.browsable).filter((n): n is number => n != null)
  );
  const totalJobs = $derived(measured.reduce((sum, n) => sum + n, 0));

  // When the snapshot was taken. Every row of one run shares it, so it is stated once here
  // rather than on 200-odd cards. The figures are a snapshot, and a snapshot that will not
  // say when it was taken is asking to be read as live.
  const measuredAt = $derived((sources ?? []).find((s) => s.jobs)?.jobs?.measured_at ?? null);

  function count(n: number): string {
    return n.toLocaleString();
  }

  // A percentage that never rounds INTO an absolute it cannot support. 1 posting in 100,000
  // is not "0%", and its complement is not "100% not matched to one" — which is the
  // strongest form of the claim this feature refuses to make in words, arrived at by
  // rounding. Only a genuine 0 or a genuine all reads as 0% or 100%.
  function pctLabel(part: number, total: number): string {
    if (part <= 0) return '0%';
    if (part >= total) return '100%';
    const pct = Math.round((part / total) * 100);
    if (pct <= 0) return '<1%';
    if (pct >= 100) return '>99%';
    return `${pct}%`;
  }

  // An aggregator's overlap with the first-party ATS crawl, as two labels and a bar width.
  // Both labels come from their OWN count rather than one being 100 minus the other, so a
  // rounding decision on one cannot invent a figure for the other.
  // `base` is carried because the percentages are shares of the RAW posting count, while
  // the headline number on the card is the de-duplicated one. Without printing the base, a
  // card reading "40 jobs · 30% not matched to one" gives a reader two numbers they cannot
  // reconcile and no way to notice they are measuring different things.
  function overlap(
    entry: SourceEntry
  ): { matched: string; unmatched: string; width: number; base: number } | null {
    const jobs = entry.jobs;
    if (!jobs || jobs.ats_matched == null || jobs.ats_unmatched == null || jobs.open <= 0) {
      return null;
    }
    return {
      matched: pctLabel(jobs.ats_matched, jobs.open),
      unmatched: pctLabel(jobs.ats_unmatched, jobs.open),
      width: (jobs.ats_matched / jobs.open) * 100,
      base: jobs.open,
    };
  }

  // The search this source's count links to. Kept as a function so the count and the link
  // beside it can never be built from different things.
  function jobsHref(source: string): string {
    return `${resolve('/jobs')}?source=${encodeURIComponent(source)}`;
  }

</script>

{#if sources === null}
  <p class="rounded-lg border border-border bg-muted/30 p-6 text-sm text-muted-foreground">
    The source catalogue is unavailable right now. It is rebuilt on a schedule — try again in
    a few minutes.
  </p>
{:else}
  <div class="mb-8">
    <label class="sr-only" for="source-search">Search sources</label>
    <input
      id="source-search"
      type="search"
      bind:value={query}
      placeholder="Search {sources.length} sources — greenhouse, adzuna, telegram…"
      class="w-full rounded-lg border border-border bg-background px-4 py-2.5 text-sm outline-none placeholder:text-muted-foreground focus:border-foreground/30"
    />
    <p class="mt-2 text-xs text-muted-foreground">
      {count(sources.length)} sources ·
      {#if measured.length === 0}
        job counts not measured yet
      {:else if measured.length < sources.length}
        {count(totalJobs)} jobs indexed across {count(measured.length)} of them
      {:else}
        {count(totalJobs)} jobs indexed
      {/if}
      {#if measuredAt}
        · counted {timeAgo(measuredAt, locale(), 'short')}
      {/if}
    </p>
  </div>

  {#if sources.length === 0}
    <p class="rounded-lg border border-border bg-muted/30 p-6 text-sm text-muted-foreground">
      No sources are listed yet. The catalogue is measured on a schedule — check back shortly.
    </p>
  {:else if grouped.length === 0}
    <p class="rounded-lg border border-border bg-muted/30 p-6 text-sm text-muted-foreground">
      No source matches “{query}”.
    </p>
  {:else}
    {#each grouped as group (group.kind)}
      <section class="mb-12">
        <h2 class="text-xl font-semibold tracking-tight">{group.title}</h2>
        <p class="mt-1 text-sm text-muted-foreground">{group.blurb}</p>

        <ul class="mt-5 grid gap-3 sm:grid-cols-2">
          {#each group.entries as entry (entry.source)}
            {@const label = sourceLabel(entry.source)}
            {@const jobs = browsable(entry)}
            {@const share = overlap(entry)}
            <li class="rounded-lg border border-border bg-background p-4">
              <div class="flex items-start gap-3">
                <!-- The display name, plus the platform's own domain where one is curated:
                     a name the proxy resolves wrongly answers 200 with somebody else's mark,
                     and a domain settles it. EntityLogo, not a hand-rolled <img>: besides the
                     onerror fallback it catches a miss that happened BEFORE hydration, which
                     this page needs precisely because it is server-rendered — the browser
                     fetches the logo and the 404 fires while no handler exists yet. -->
                <EntityLogo
                  name={label}
                  src={sourceLogoUrl(label, SOURCE_LOGO_DOMAINS[entry.source]) ?? undefined}
                  shape="square"
                  size="sm"
                  class="mt-0.5 shrink-0"
                />

                <div class="min-w-0 flex-1">
                  <p class="truncate font-medium">{label}</p>

                  <p class="mt-0.5 text-sm">
                    {#if jobs === null}
                      <span class="text-muted-foreground">Job count not measured yet</span>
                    {:else if jobs === 0}
                      <span class="text-muted-foreground">No open jobs right now</span>
                    {:else}
                      <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- internal /jobs filter link; resolve()d base plus a query string, no dynamic route to resolve -->
                      <a href={jobsHref(entry.source)} class="underline-offset-4 hover:underline"
                        >{count(jobs)} jobs</a
                      >
                    {/if}
                  </p>

                  <p class="mt-1 font-mono text-xs text-muted-foreground">
                    {#if entry.health?.last_success}
                      read {timeAgo(entry.health.last_success, locale(), 'short')}
                      {#if entry.health.ingested_total > 0}
                        · {count(entry.health.ingested_total)} ingested
                      {/if}
                    {:else if entry.health}
                      never read successfully
                    {:else if entry.kind === 'other'}
                      not a crawl adapter
                    {:else}
                      <!-- A registered adapter with no health record yet. It is emphatically
                           NOT "not a crawl adapter" — that sentence about Greenhouse is worse
                           than saying nothing, and `health === null` means both things. -->
                      no crawl recorded yet
                    {/if}
                  </p>
                </div>
              </div>

              {#if share !== null}
                <div class="mt-3">
                  <div
                    class="flex h-1.5 overflow-hidden rounded-full bg-muted"
                    role="img"
                    aria-label="{share.matched} of these postings were also found on a first-party ATS; {share.unmatched} were not matched to one"
                  >
                    <span class="bg-foreground/70" style="width: {share.width}%"></span>
                  </div>
                  <p class="mt-1.5 text-xs text-muted-foreground">
                    Of {count(share.base)} postings before de-duplication:
                    {share.matched} also found on an ATS · {share.unmatched} not matched to one
                  </p>
                </div>
              {/if}
            </li>
          {/each}
        </ul>
      </section>
    {/each}
  {/if}
{/if}
