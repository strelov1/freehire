<script lang="ts">
  import { timeAgo } from '$lib/utils';
  import type { HealthStatus, IngestStatus, ProviderKind } from '$lib/types';

  // The presentational half of the /status page: given the ingest-fleet rollup (or
  // null when the API read failed), it renders the overall banner and the
  // worst-first provider list. Kept separate from the route so it can be previewed
  // and reasoned about without a live API. The route owns data loading + SEO.
  let { status }: { status: IngestStatus | null } = $props();

  // Status → display metadata. Tone classes mirror RealityBadge's light/dark
  // convention; the dot is the at-a-glance signal, the pill spells it out.
  const STATUS_META: Record<HealthStatus, { label: string; dot: string; pill: string }> = {
    operational: {
      label: 'Operational',
      dot: 'bg-emerald-500',
      pill: 'border-emerald-500/40 bg-emerald-500/10 text-emerald-700 dark:text-emerald-400',
    },
    degraded: {
      label: 'Degraded',
      dot: 'bg-amber-500',
      pill: 'border-amber-500/40 bg-amber-500/10 text-amber-700 dark:text-amber-400',
    },
    down: {
      label: 'Down',
      dot: 'bg-red-500',
      pill: 'border-red-500/40 bg-red-500/10 text-red-700 dark:text-red-400',
    },
  };

  const OVERALL_HEADLINE: Record<HealthStatus, string> = {
    operational: 'All systems operational',
    degraded: 'Partial degradation',
    down: 'Major outage',
  };

  const SEVERITY: Record<HealthStatus, number> = { operational: 0, degraded: 1, down: 2 };

  // Human labels for the source-kind taxonomy the backend derives from adapter type.
  const KIND_LABEL: Record<ProviderKind, string> = {
    ats: 'ATS',
    aggregator: 'Aggregators',
    company: 'Company pages',
    other: 'Other',
  };
  // Order the kind chips deliberately (ATS first — the bulk), then filter to those
  // that actually appear in the data.
  const KIND_ORDER: ProviderKind[] = ['ats', 'aggregator', 'company', 'other'];

  const titleCase = (s: string) => s.charAt(0).toUpperCase() + s.slice(1).replace(/[_-]/g, ' ');
  const nf = new Intl.NumberFormat('en');

  // Worst-first, then alphabetical — problem providers surface at the top.
  const providers = $derived(
    [...(status?.providers ?? [])].sort(
      (a, b) => SEVERITY[b.status] - SEVERITY[a.status] || a.provider.localeCompare(b.provider),
    ),
  );
  const overall = $derived(status?.overall ?? null);

  // Kind chips: only kinds present in the data, each with its live count.
  const kindCounts = $derived.by(() => {
    const counts: Partial<Record<ProviderKind, number>> = {};
    for (const p of providers) counts[p.kind] = (counts[p.kind] ?? 0) + 1;
    return KIND_ORDER.filter((k) => counts[k]).map((k) => ({ kind: k, count: counts[k]! }));
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
      return p.provider.toLowerCase().includes(q) || titleCase(p.provider).toLowerCase().includes(q);
    });
  });
</script>

{#if !status || !overall}
  <div class="rounded-xl border border-border p-8 text-center text-muted-foreground">
    Status is unavailable right now. Try again in a moment.
  </div>
{:else}
  <!-- Overall banner -->
  <div class="mb-10 flex items-center gap-4 rounded-xl border p-5 sm:p-6 {STATUS_META[overall].pill}">
    <span class="inline-flex h-3 w-3 shrink-0 rounded-full {STATUS_META[overall].dot}"></span>
    <div>
      <div class="text-lg font-semibold tracking-tight">{OVERALL_HEADLINE[overall]}</div>
      <div class="text-sm opacity-80">
        {providers.length} provider{providers.length === 1 ? '' : 's'} monitored
      </div>
    </div>
  </div>

  <!-- Provider list -->
  <section>
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
          class="w-full rounded-lg border border-border bg-background px-3 py-2 text-sm outline-none placeholder:text-muted-foreground focus:border-foreground"
        />
      </label>

      {#if kindCounts.length > 1}
        <div class="mt-3 flex flex-wrap gap-2">
          <button
            type="button"
            onclick={() => (kind = 'all')}
            class="rounded-full border px-3 py-1 text-xs font-medium transition-colors {kind === 'all'
              ? 'border-foreground bg-foreground text-background'
              : 'border-border text-muted-foreground hover:text-foreground'}"
          >
            All {nf.format(providers.length)}
          </button>
          {#each kindCounts as kc (kc.kind)}
            <button
              type="button"
              onclick={() => (kind = kc.kind)}
              class="rounded-full border px-3 py-1 text-xs font-medium transition-colors {kind === kc.kind
                ? 'border-foreground bg-foreground text-background'
                : 'border-border text-muted-foreground hover:text-foreground'}"
            >
              {KIND_LABEL[kc.kind]} {nf.format(kc.count)}
            </button>
          {/each}
        </div>
      {/if}

      {#if filtered.length === 0}
        <p class="mt-6 text-sm text-muted-foreground">
          No providers match “{query.trim()}”.
        </p>
      {:else}
        <ul class="mt-4 divide-y divide-border overflow-hidden rounded-xl border border-border">
          {#each filtered as p (p.provider)}
          {@const meta = STATUS_META[p.status]}
          <li class="flex flex-wrap items-center gap-x-4 gap-y-1 bg-background p-4 sm:p-5">
            <span class="h-2.5 w-2.5 shrink-0 rounded-full {meta.dot}"></span>
            <div class="min-w-0 flex-1">
              <div class="flex items-center gap-2">
                <span class="font-medium">{titleCase(p.provider)}</span>
                <span class="font-mono text-[10px] uppercase tracking-wide text-muted-foreground">
                  {KIND_LABEL[p.kind]}
                </span>
              </div>
              <div class="text-sm text-muted-foreground">
                {nf.format(p.healthy_boards)} / {nf.format(p.total_boards)} boards healthy{#if p.cooled_boards > 0}<span
                    class="text-amber-600 dark:text-amber-500"
                  >
                    · {nf.format(p.cooled_boards)} in cooldown</span
                  >{/if}
              </div>
            </div>
            <div class="flex flex-col items-end gap-1">
              <span
                class="inline-flex items-center rounded-md border px-2 py-0.5 text-xs font-medium {meta.pill}"
              >
                {meta.label}
              </span>
              {#if p.last_run}
                <span class="text-xs text-muted-foreground" title={p.last_run}>
                  ran {timeAgo(p.last_run)}
                </span>
              {/if}
            </div>
          </li>
          {/each}
        </ul>
      {/if}
    {/if}
  </section>
{/if}
