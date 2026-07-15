<script lang="ts">
  import { timeAgo } from '$lib/utils';
  import type { HealthStatus, IngestStatus } from '$lib/types';

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

  const titleCase = (s: string) => s.charAt(0).toUpperCase() + s.slice(1).replace(/[_-]/g, ' ');
  const nf = new Intl.NumberFormat('en');

  // Worst-first, then alphabetical — problem providers surface at the top.
  const providers = $derived(
    [...(status?.providers ?? [])].sort(
      (a, b) => SEVERITY[b.status] - SEVERITY[a.status] || a.provider.localeCompare(b.provider),
    ),
  );
  const overall = $derived(status?.overall ?? null);
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
      <ul class="mt-6 divide-y divide-border overflow-hidden rounded-xl border border-border">
        {#each providers as p (p.provider)}
          {@const meta = STATUS_META[p.status]}
          <li class="flex flex-wrap items-center gap-x-4 gap-y-1 bg-background p-4 sm:p-5">
            <span class="h-2.5 w-2.5 shrink-0 rounded-full {meta.dot}"></span>
            <div class="min-w-0 flex-1">
              <div class="font-medium">{titleCase(p.provider)}</div>
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
  </section>
{/if}
