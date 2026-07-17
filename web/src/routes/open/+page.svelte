<script lang="ts">
  import { page } from '$app/state';
  import Seo from '$lib/components/Seo.svelte';
  import ActivityBars from '$lib/components/ActivityBars.svelte';
  import GrowthArea from '$lib/components/GrowthArea.svelte';
  import type { PageData } from './$types';

  let { data }: { data: PageData } = $props();

  const canonical = $derived(`${page.url.origin}/open`);

  // Repo constants — the crawler covers this many ATS platforms and Telegram
  // channels. Not DB rows; they change only when adapters/channels are added (i.e.
  // on deploy), mirroring the homepage stat-strip.
  const ATS_PLATFORMS = 98;
  const TELEGRAM_CHANNELS = 87;

  const nf = new Intl.NumberFormat('en');
  const compactNf = new Intl.NumberFormat('en', { notation: 'compact', maximumFractionDigits: 1 });
  const fmt = (n: number | null, fallback: string) => (n == null ? fallback : compactNf.format(n));

  const regionNames = new Intl.DisplayNames(['en'], { type: 'region' });
  function countryName(code: string): string {
    try {
      return regionNames.of(code.toUpperCase()) ?? code.toUpperCase();
    } catch {
      return code.toUpperCase();
    }
  }
  const titleCase = (s: string) => s.charAt(0).toUpperCase() + s.slice(1).replace(/_/g, ' ');

  type Bar = { label: string; count: number };
  function top(facet: Record<string, number> | undefined, n: number, label: (k: string) => string): Bar[] {
    if (!facet) return [];
    return Object.entries(facet)
      .sort((a, b) => b[1] - a[1])
      .slice(0, n)
      .map(([k, count]) => ({ label: label(k), count }));
  }

  const facets = $derived(data.facets);
  const countries = $derived(top(facets?.countries, 8, countryName));
  const skills = $derived(top(facets?.skills, 8, titleCase));
  const seniority = $derived(top(facets?.seniority, 6, titleCase));
  const workMode = $derived(top(facets?.work_mode, 3, titleCase));

  // The snapshot endpoint always returns the four facet keys (empty maps before the
  // daily rollup's first run), so `facets` is truthy even when there's nothing to
  // show. Gate the section on actual values so a not-yet-populated snapshot — or a
  // failed load — falls back to the "unavailable" message instead of empty headings.
  const hasFacets = $derived(
    countries.length > 0 || skills.length > 0 || seniority.length > 0 || workMode.length > 0,
  );

  // Remote share across the resolved work-mode buckets.
  const remoteShare = $derived.by(() => {
    const wm = facets?.work_mode;
    if (!wm) return null;
    const total = Object.values(wm).reduce((a, b) => a + b, 0);
    if (total === 0) return null;
    return Math.round(((wm.remote ?? 0) / total) * 100);
  });

  const stats = $derived([
    { value: fmt(data.scale.jobs, '3M+'), label: 'open jobs', href: '/api/v1/jobs' },
    { value: fmt(data.scale.companies, '220K+'), label: 'companies', href: '/api/v1/companies' },
    { value: nf.format(ATS_PLATFORMS), label: 'ATS platforms', href: null },
    { value: nf.format(TELEGRAM_CHANNELS), label: 'Telegram channels', href: null },
  ]);

  const gh = $derived(data.github);
  const members = $derived.by(() => {
    const last = data.growth[data.growth.length - 1];
    return last ? last.total : null;
  });

  const engagement = $derived.by(() => {
    const e = data.engagement;
    if (!e) return null;
    return [
      { value: nf.format(e.saved), label: 'jobs saved' },
      { value: nf.format(e.applied), label: 'applications' },
      { value: nf.format(e.viewed), label: 'jobs viewed' },
    ];
  });
</script>

{#snippet distributionBars(items: Bar[])}
  {@const peak = Math.max(1, ...items.map((i) => i.count))}
  <div class="flex flex-col gap-2.5">
    {#each items as it (it.label)}
      <div class="flex items-center gap-3">
        <div class="w-32 shrink-0 truncate text-sm" title={it.label}>{it.label}</div>
        <div class="h-2.5 flex-1 overflow-hidden rounded-full bg-secondary">
          <div class="h-full rounded-full bg-foreground" style="width: {(it.count / peak) * 100}%"></div>
        </div>
        <div class="w-16 shrink-0 text-right text-sm tabular-nums text-muted-foreground">
          {compactNf.format(it.count)}
        </div>
      </div>
    {/each}
  </div>
{/snippet}

{#snippet sourceLink(href: string, label: string)}
  <a
    {href}
    class="font-mono text-xs text-muted-foreground underline-offset-4 hover:text-foreground hover:underline"
  >
    {label} ↗
  </a>
{/snippet}

<Seo
  title="Open — freehire's numbers, live"
  description="The open startup page for freehire: live catalogue scale, daily job movement, what's inside, member growth, and open-source stats. Every figure comes straight from the public API."
  {canonical}
/>

<div class="mx-auto w-full max-w-4xl px-4 py-10 sm:py-14">
  <!-- Intro -->
  <header class="mb-12">
    <p class="font-mono text-xs uppercase tracking-[0.2em] text-muted-foreground">// open startup</p>
    <h1 class="mt-4 text-4xl font-semibold tracking-tighter sm:text-5xl">All our numbers, live.</h1>
    <p class="mt-4 max-w-2xl text-lg leading-relaxed text-muted-foreground">
      freehire is an open, free search engine for jobs — so our metrics are open too. Every figure
      below is pulled live from the same public API anyone (or any AI agent) can query. No dashboards
      behind a login, no vanity rounding.
    </p>
  </header>

  <!-- A. Catalogue scale -->
  <section class="mb-14">
    <p class="font-mono text-xs uppercase tracking-[0.2em] text-muted-foreground">// the catalogue</p>
    <dl class="mt-6 grid grid-cols-2 gap-px overflow-hidden rounded-xl border border-border bg-border sm:grid-cols-4">
      {#each stats as s (s.label)}
        <div class="bg-background p-5 sm:p-6">
          <dt class="font-mono text-xs uppercase tracking-wide text-muted-foreground">{s.label}</dt>
          <dd class="mt-2 text-3xl font-semibold tracking-tight tabular-nums sm:text-4xl">
            {#if s.href}
              <a href={s.href} class="hover:underline">{s.value}</a>
            {:else}
              {s.value}
            {/if}
          </dd>
        </div>
      {/each}
    </dl>
  </section>

  <!-- B. Catalogue movement -->
  <section class="mb-14">
    <div class="flex items-baseline justify-between">
      <p class="font-mono text-xs uppercase tracking-[0.2em] text-muted-foreground">// daily movement</p>
      {@render sourceLink('/api/v1/stats/jobs-activity', '/stats/jobs-activity')}
    </div>
    <h2 class="mt-3 text-xl font-semibold tracking-tight">Jobs added vs. removed</h2>
    <p class="mb-6 mt-1 text-sm text-muted-foreground">
      New vacancies crawled in versus postings we detected as closed, per day.
    </p>
    <ActivityBars points={data.activity} />
  </section>

  <!-- F. Member growth -->
  <section class="mb-14">
    <div class="flex items-baseline justify-between">
      <p class="font-mono text-xs uppercase tracking-[0.2em] text-muted-foreground">// members</p>
      {@render sourceLink('/api/v1/stats/user-growth', '/stats/user-growth')}
    </div>
    <h2 class="mt-3 text-xl font-semibold tracking-tight">
      {members == null ? 'People on freehire' : `${nf.format(members)} people on freehire`}
    </h2>
    <p class="mb-6 mt-1 text-sm text-muted-foreground">
      Cumulative registered members over time. Early days — and that's the point of showing it.
    </p>
    <GrowthArea points={data.growth} />
  </section>

  <!-- Engagement -->
  <section class="mb-14">
    <div class="flex items-baseline justify-between">
      <p class="font-mono text-xs uppercase tracking-[0.2em] text-muted-foreground">// engagement</p>
      {@render sourceLink('/api/v1/stats/engagement', '/stats/engagement')}
    </div>
    <h2 class="mt-3 text-xl font-semibold tracking-tight">What people do here</h2>
    <p class="mb-6 mt-1 text-sm text-muted-foreground">
      Signed-in interactions across freehire — jobs saved, applications tracked, and postings opened.
    </p>
    {#if engagement}
      <dl class="grid grid-cols-3 gap-px overflow-hidden rounded-xl border border-border bg-border">
        {#each engagement as e (e.label)}
          <div class="bg-background p-5 sm:p-6">
            <dt class="font-mono text-xs uppercase tracking-wide text-muted-foreground">{e.label}</dt>
            <dd class="mt-2 text-3xl font-semibold tracking-tight tabular-nums sm:text-4xl">{e.value}</dd>
          </div>
        {/each}
      </dl>
    {:else}
      <p class="text-sm text-muted-foreground">Engagement data is unavailable right now.</p>
    {/if}
  </section>

  <!-- C. What's inside -->
  <section class="mb-14">
    <div class="flex items-baseline justify-between">
      <p class="font-mono text-xs uppercase tracking-[0.2em] text-muted-foreground">// what's inside</p>
      {@render sourceLink('/api/v1/stats/facets', '/stats/facets')}
    </div>
    {#if hasFacets}
      <div class="mt-6 grid gap-x-10 gap-y-8 sm:grid-cols-2">
        <div>
          <h3 class="mb-4 text-sm font-semibold">Top countries</h3>
          {@render distributionBars(countries)}
        </div>
        <div>
          <h3 class="mb-4 text-sm font-semibold">Top skills</h3>
          {@render distributionBars(skills)}
        </div>
        <div>
          <h3 class="mb-4 text-sm font-semibold">Seniority</h3>
          {@render distributionBars(seniority)}
        </div>
        <div>
          <h3 class="mb-4 text-sm font-semibold">
            Work mode{#if remoteShare != null}<span class="ml-2 font-normal text-muted-foreground">{remoteShare}% remote</span>{/if}
          </h3>
          {@render distributionBars(workMode)}
        </div>
      </div>
    {:else}
      <p class="mt-6 text-sm text-muted-foreground">Distribution data is unavailable right now.</p>
    {/if}
  </section>

  <!-- D. Open source -->
  <section class="mb-6">
    <div class="flex items-baseline justify-between">
      <p class="font-mono text-xs uppercase tracking-[0.2em] text-muted-foreground">// open source</p>
      {@render sourceLink('https://github.com/strelov1/freehire', 'github.com/strelov1/freehire')}
    </div>
    <div class="mt-6 rounded-xl border border-border p-6 sm:p-8">
      {#if gh}
        <div class="flex flex-wrap gap-x-12 gap-y-6">
          <div>
            <div class="text-3xl font-semibold tabular-nums">{nf.format(gh.stars)}</div>
            <div class="font-mono text-xs uppercase tracking-wide text-muted-foreground">stars</div>
          </div>
          <div>
            <div class="text-3xl font-semibold tabular-nums">{nf.format(gh.forks)}</div>
            <div class="font-mono text-xs uppercase tracking-wide text-muted-foreground">forks</div>
          </div>
          {#if gh.contributors != null}
            <div>
              <div class="text-3xl font-semibold tabular-nums">{nf.format(gh.contributors)}</div>
              <div class="font-mono text-xs uppercase tracking-wide text-muted-foreground">contributors</div>
            </div>
          {/if}
          {#if gh.license}
            <div>
              <div class="text-3xl font-semibold">{gh.license}</div>
              <div class="font-mono text-xs uppercase tracking-wide text-muted-foreground">license</div>
            </div>
          {/if}
        </div>
      {/if}
      <p class="mt-6 max-w-xl text-sm leading-relaxed text-muted-foreground">
        The whole pipeline is open source. Missing a job source you use? Adding it is usually one
        small adapter — <a
          href="https://github.com/strelov1/freehire"
          class="font-medium text-foreground underline-offset-4 hover:underline">a PR is very welcome</a
        >.
      </p>
    </div>
  </section>
</div>
