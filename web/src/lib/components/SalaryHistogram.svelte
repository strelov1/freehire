<script lang="ts">
  import { formatSalary } from '#lib/insights.js';
  import { histogramFromQuantiles } from '#lib/salaries.js';

  // The distribution of one band: how many postings pay what, from the rollup's 19
  // quantiles. Bars are shares of all the band's postings; the bins span its 5th to
  // 95th percentile, so the extremes a few outliers would stretch the axis to are left out.
  let { quantiles, currency, period }: { quantiles: number[]; currency: string; period: string } = $props();

  const bins = $derived(histogramFromQuantiles(quantiles));
  const maxShare = $derived(Math.max(0, ...bins.map((b) => b.share)));
  let hovered = $state<number | null>(null);
  const active = $derived(hovered === null ? null : (bins[hovered] ?? null));
  const pct = (share: number) => `${Math.round(share * 100)}%`;
  const range = (b: { lo: number; hi: number }) => `${formatSalary(b.lo, currency)} – ${formatSalary(b.hi, currency)}`;
  // Every other edge once the bins crowd a phone-width axis.
  const labelEvery = $derived(bins.length > 8 ? 2 : 1);
</script>

{#if bins.length}
  <div onpointerleave={() => (hovered = null)}>
    <div
      class="flex h-44 items-end gap-0.5"
      role="group"
      aria-label="Postings by advertised salary, {currency} per {period}"
    >
      {#each bins as b, i (b.lo)}
        <button
          type="button"
          class="group relative flex h-full flex-1 items-end focus-visible:outline-2 focus-visible:outline-ring"
          aria-label="{range(b)}: {pct(b.share)} of postings"
          onpointerenter={() => (hovered = i)}
          onfocus={() => (hovered = i)}
        >
          <span
            class="w-full rounded-t-sm transition-colors {hovered === i
              ? 'bg-brand'
              : 'bg-brand/60 group-hover:bg-brand'}"
            style:height="{maxShare > 0 ? Math.max(2, (b.share / maxShare) * 100) : 0}%"
          ></span>
        </button>
      {/each}
    </div>
    <div class="mt-1 flex gap-0.5 text-xs text-muted-foreground tabular-nums">
      {#each bins as b, i (b.lo)}
        <span class="flex-1 truncate">{i % labelEvery === 0 ? formatSalary(b.lo, currency) : ''}</span>
      {/each}
    </div>
    <p class="mt-2 h-4 text-xs text-muted-foreground" aria-live="polite">
      {#if active}
        <span class="font-medium text-foreground">{range(active)}</span>: {pct(active.share)} of postings
      {:else}
        The middle 90% of postings, per {period}; hover a bar for its share.
      {/if}
    </p>
  </div>
{:else}
  <p class="text-sm text-muted-foreground">The distribution appears after the next recount of the figures.</p>
{/if}
