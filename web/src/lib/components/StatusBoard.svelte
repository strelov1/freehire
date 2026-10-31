<script lang="ts">
  import { CircleAlert, CircleCheck, CircleX } from '@lucide/svelte';
  import { timeAgo, must } from '#lib/utils.js';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  // A provider key is a source facet code, so it renders through the one label map every
  // other surface uses: a provider must not be "WhatJobs" on the filter panel and
  // "Whatjobs" here.
  import { sourceLabel } from '#lib/facets.js';
  import { TabStrip, tabStripId } from '#lib/ui/index.js';
  import type { HealthStatus, IngestStatus, ProviderKind } from '#lib/types.js';
  import StatusBarCalendar from './status/StatusBarCalendar.svelte';

  // The presentational half of the /status page: given the status read (or null
  // when the API read failed), it renders the site/API's own status, the
  // ingest-fleet overall banner, and the worst-first provider list. Kept separate
  // from the route so it can be previewed and reasoned about without a live API.
  // The route owns data loading + SEO.
  let { status }: { status: IngestStatus | null } = $props();

  const STATUS_META: Record<
    HealthStatus,
    { label: string; dot: string; icon: string; Icon: typeof CircleCheck }
  > = {
    operational: { label: 'Operational', dot: 'bg-success', icon: 'text-success', Icon: CircleCheck },
    degraded: { label: 'Degraded', dot: 'bg-warning', icon: 'text-warning', Icon: CircleAlert },
    down: { label: 'Down', dot: 'bg-destructive', icon: 'text-destructive', Icon: CircleX },
  };

  const OVERALL_HEADLINE: Record<HealthStatus, string> = {
    operational: 'All systems operational',
    degraded: 'Partial degradation',
    down: 'Major outage',
  };

  // The site/API's own status is a separate concern from the ingest fleet above —
  // one is "is freehire.me itself working", the other is "are the crawlers working".
  const SITE_HEADLINE: Record<HealthStatus, string> = {
    operational: 'API operational',
    degraded: 'API degraded',
    down: 'API down',
  };
  const nfPercent = new Intl.NumberFormat('en', { style: 'percent', maximumFractionDigits: 1 });

  // The daily history strip: 90 tiles, oldest first, each either a recorded
  // HealthStatus or null for "no sample recorded that day" — a gap must render
  // as its own neutral treatment, never silently as "operational".
  const HISTORY_DAYS = 90;

  function utcDateString(d: Date): string {
    return d.toISOString().slice(0, 10);
  }

  const historyTiles = $derived.by(() => {
    const byDay = new Map((status?.site.history ?? []).map((h) => [h.day, h.status]));
    // Anchored on the response's own generated_at (UTC) rather than the visitor's
    // local clock, so "today" here always matches the day the backend last wrote —
    // the two can disagree by up to a day if left to the browser's local timezone.
    const anchor = status?.generated_at ? new Date(status.generated_at) : new Date();
    const tiles: { day: string; status: HealthStatus | null }[] = [];
    for (let i = HISTORY_DAYS - 1; i >= 0; i--) {
      const d = new Date(Date.UTC(anchor.getUTCFullYear(), anchor.getUTCMonth(), anchor.getUTCDate() - i));
      const day = utcDateString(d);
      tiles.push({ day, status: byDay.get(day) ?? null });
    }
    return tiles;
  });

  const uptime = $derived.by(() => {
    let sampled = 0;
    let operational = 0;
    for (const tile of historyTiles) {
      if (!tile.status) continue;
      sampled++;
      if (tile.status === 'operational') operational++;
    }
    if (sampled === 0) return null;
    return operational / sampled;
  });

  const historySpan = $derived.by(() => {
    const from = historyTiles[0]?.day;
    const to = historyTiles[historyTiles.length - 1]?.day;
    if (!from || !to) return null;
    return { from, to };
  });

  const SEVERITY: Record<HealthStatus, number> = { operational: 0, degraded: 1, down: 2 };

  // Human labels for the source-kind taxonomy the backend derives from adapter type.
  const KIND_LABEL: Record<ProviderKind, string> = {
    ats: 'ATS',
    aggregator: 'Aggregators',
    company: 'Company pages',
    social: 'Social',
    other: 'Other',
  };
  // Order the kind chips deliberately (ATS first — the bulk), then filter to those
  // that actually appear in the data.
  const KIND_ORDER: ProviderKind[] = ['ats', 'aggregator', 'company', 'social', 'other'];

  const nf = new Intl.NumberFormat('en');

  // The site/API and the ingest fleet are different concerns (see SITE_HEADLINE's
  // comment above) — tabs keep them from competing for the same screen instead of
  // stacking sections a visitor has to scroll past to reach the one they want.
  // TabStrip (the underline style every /my/* section nav already uses), not the
  // pill/segmented Tabs primitive — this page reads as part of the same product,
  // not a different widget kit bolted on.
  const SECTION_TABS = [
    { id: 'site', label: 'Site status' },
    { id: 'fleet', label: 'Ingest fleet' },
  ] as const;
  const SECTION_PANEL_ID = 'status-section-panel';
  let activeSection = $state<'site' | 'fleet'>('site');

  // Worst-first, then alphabetical — problem providers surface at the top.
  const providers = $derived(
    [...(status?.providers ?? [])].sort(
      (a, b) => SEVERITY[b.status] - SEVERITY[a.status] || a.provider.localeCompare(b.provider),
    ),
  );
  function boardPercent(healthy: number, total: number): number | null {
    if (total <= 0) return null;
    return Math.min(100, Math.round((healthy / total) * 100));
  }

  // Kind chips: only kinds present in the data, each with its live count.
  const kindCounts = $derived.by(() => {
    const counts: Partial<Record<ProviderKind, number>> = {};
    for (const p of providers) counts[p.kind] = (counts[p.kind] ?? 0) + 1;
    return KIND_ORDER.filter((k) => counts[k]).map((k) => ({ kind: k, count: must(counts[k]) }));
  });

  // Two combined filters: a kind chip ('all' = no kind constraint) and a free-text
  // match over the name. With dozens of providers, "narrow to aggregators, then find
  // the one I care about" beats scrolling.
  let kind = $state<ProviderKind | 'all'>('all');
  let query = $state('');
  const filtered = $derived.by(() => {
    const q = query.trim().toLowerCase();
    return providers.filter((p) => {
      if (kind !== 'all' && p.kind !== kind) return false;
      if (!q) return true;
      return p.provider.toLowerCase().includes(q) || sourceLabel(p.provider).toLowerCase().includes(q);
    });
  });
</script>

{#snippet statusDot(dot: string)}
  <span class="relative flex size-2.5 shrink-0" aria-hidden="true">
    <span class="{dot} absolute inline-flex size-full rounded-full opacity-75 motion-safe:animate-ping"></span>
    <span class="{dot} relative inline-flex size-2.5 rounded-full"></span>
  </span>
{/snippet}

{#snippet kindChip(id: ProviderKind | 'all', label: string)}
  <button
    type="button"
    onclick={() => (kind = id)}
    class="rounded-full border px-3 py-1 text-xs font-medium transition-colors {kind === id
      ? 'border-foreground bg-foreground text-background'
      : 'border-border text-muted-foreground hover:text-foreground'}"
  >
    {label}
  </button>
{/snippet}

{#if !status}
  <div class="rounded-xl border border-border p-8 text-center text-muted-foreground">
    Status is unavailable right now. Try again in a moment.
  </div>
{:else}
  <TabStrip
    tabs={SECTION_TABS}
    active={activeSection}
    onSelect={(id) => (activeSection = id)}
    label="Status sections"
    panelId={SECTION_PANEL_ID}
  />

  <div
    id={SECTION_PANEL_ID}
    role="tabpanel"
    aria-labelledby={tabStripId(SECTION_PANEL_ID, activeSection)}
    class="mt-6"
  >
    {#if activeSection === 'site'}
      {@const siteMeta = STATUS_META[status.site.status]}
      <div class="overflow-hidden rounded-2xl border">
        <div class="flex items-center gap-3 px-4 py-3">
          {@render statusDot(siteMeta.dot)}
            <div class="min-w-0 flex-1">
              <p class="text-base leading-tight font-medium break-words">{SITE_HEADLINE[status.site.status]}</p>
              <p class="mt-1 text-sm text-muted-foreground">
                Database {status.site.database} · {nfPercent.format(status.site.error_rate)} error rate over the last {status.site.window_minutes} min
              </p>
              <!-- Reported beside the error rate, never conditionally. The two answer
                   different questions — the error rate describes requests that FINISHED,
                   this describes how much of the connection pool is held at this instant,
                   which is why requests may not start. An earlier draft showed this line
                   only above 90% and read as a warning; the live pool touches its ceiling
                   in ordinary bursts, so that phrasing cried wolf on a healthy site. A
                   sustained reading is what matters, and judging that needs history the
                   page does not have (the Grafana rule averages it over five minutes). -->
              <p class="text-sm text-muted-foreground">
                Database connections in use: {nfPercent.format(status.site.pool_pressure)}
              </p>
            </div>
            {#if uptime !== null}
              <div class="shrink-0 text-right">
                <p class="text-sm tabular-nums font-medium" title="Operational days among days with a recorded sample">
                  {nfPercent.format(uptime)}
                </p>
                <p class="text-xs text-muted-foreground">uptime</p>
              </div>
            {/if}
          </div>
          <div class="flex flex-col gap-1 px-4 pb-3">
            <StatusBarCalendar days={historyTiles} barHeight={22} radius={6} />
            {#if historySpan}
              <div class="flex min-w-0 justify-between gap-3">
                <p class="min-w-0 truncate text-xs font-medium text-muted-foreground">{historySpan.from}</p>
                <p class="min-w-0 truncate text-right text-xs font-medium text-muted-foreground">{historySpan.to}</p>
              </div>
            {/if}
        </div>
      </div>
    {:else}
      {@const fleetMeta = STATUS_META[status.overall]}
      <div class="flex items-center gap-3 rounded-2xl border px-4 py-3">
        {@render statusDot(fleetMeta.dot)}
        <div class="min-w-0">
          <p class="text-base leading-tight font-medium break-words">{OVERALL_HEADLINE[status.overall]}</p>
          <p class="mt-1 text-sm text-muted-foreground">
            {nf.format(providers.length)} provider{providers.length === 1 ? '' : 's'} monitored
          </p>
        </div>
      </div>

      <section class="mt-6">
        <div class="flex items-baseline justify-between">
          <p class="font-mono text-xs uppercase tracking-[0.2em] text-muted-foreground">// providers</p>
          <!-- A raw JSON API endpoint, not a SvelteKit page route — there is nothing
               for resolve() to map, so the internal-navigation rule doesn't apply. -->
          <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -->
          <a href="/api/v1/status" class="font-mono text-xs text-muted-foreground underline-offset-4 hover:text-foreground hover:underline">
            /status ↗
          </a>
        </div>

        {#if providers.length === 0}
          <p class="mt-6 text-sm text-muted-foreground">No providers have run yet.</p>
        {:else}
          <label class="mt-6 block">
            <span class="sr-only">Filter providers by name</span>
            <input
              type="search"
              bind:value={query}
              placeholder="Filter providers…"
              autocomplete="off"
              class="w-full rounded-lg border border-border bg-background px-3 py-2 text-sm transition-colors placeholder:text-muted-foreground focus-visible:border-ring focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            />
          </label>

          {#if kindCounts.length > 1}
            <div class="mt-3 flex flex-wrap gap-2">
              {@render kindChip('all', `All ${nf.format(providers.length)}`)}
              {#each kindCounts as kc (kc.kind)}
                {@render kindChip(kc.kind, `${KIND_LABEL[kc.kind]} ${nf.format(kc.count)}`)}
              {/each}
            </div>
          {/if}

          {#if filtered.length === 0}
            <p class="mt-6 text-sm text-muted-foreground">
              No providers match “{query.trim()}”.
            </p>
          {:else}
            <ul class="mt-4 overflow-hidden rounded-2xl border">
              {#each filtered as p, i (p.provider)}
                {@const meta = STATUS_META[p.status]}
                {@const StatusIcon = meta.Icon}
                {@const pct = boardPercent(p.healthy_boards, p.total_boards)}
                <li class="bg-background px-4 py-2.5 {i < filtered.length - 1 ? 'border-b' : ''}">
                  <div class="flex items-center gap-3">
                    <StatusIcon class="size-4 shrink-0 {meta.icon}" />
                    <div class="min-w-0 flex-1">
                      <p class="truncate font-medium">{sourceLabel(p.provider)}</p>
                      <p class="text-sm text-muted-foreground">
                        {KIND_LABEL[p.kind]} · {nf.format(p.healthy_boards)} / {nf.format(p.total_boards)} boards healthy{#if p.cooled_boards > 0}<span
                            class="text-warning-strong"
                          >
                            · {nf.format(p.cooled_boards)} in cooldown</span
                          >{/if}
                      </p>
                    </div>
                    <div class="shrink-0 text-right">
                      <p class="text-sm tabular-nums font-medium">{pct === null ? '—' : `${pct}%`}</p>
                      <p class="text-xs text-muted-foreground">{meta.label}</p>
                      {#if p.last_run}
                        <p class="text-xs text-muted-foreground" title={p.last_run}>
                          ran {timeAgo(p.last_run, locale())}
                        </p>
                      {/if}
                    </div>
                  </div>
                </li>
              {/each}
            </ul>
          {/if}
        {/if}
      </section>
    {/if}
  </div>
{/if}
