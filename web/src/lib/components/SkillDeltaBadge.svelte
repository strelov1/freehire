<script lang="ts">
  import { TrendingUp, TrendingDown, Minus } from '@lucide/svelte';
  import { fmtPct } from '$lib/skillPulseFormat';

  // The percent-change pill for one skill's trend — shared by the compact card
  // grid and the per-skill detail page. `pct` is the raw (unrounded) figure;
  // rounding happens once, here, so the icon/color and the printed number
  // never disagree (a sub-0.5% change must not show a colored arrow next to
  // a printed "0%").
  let { pct }: { pct: number | null } = $props();

  const rounded = $derived(pct === null ? null : Math.round(pct));
</script>

{#if rounded === null}
  <span
    class="inline-flex items-center gap-1 rounded-md bg-secondary px-1.5 py-0.5 text-xs text-muted-foreground"
  >
    New
  </span>
{:else if rounded > 0}
  <span
    class="inline-flex items-center gap-1 rounded-md bg-emerald-500/10 px-1.5 py-0.5 text-xs font-medium text-emerald-600 dark:text-emerald-400"
  >
    <TrendingUp class="size-3" />{fmtPct(rounded)}
  </span>
{:else if rounded < 0}
  <span
    class="inline-flex items-center gap-1 rounded-md bg-rose-500/10 px-1.5 py-0.5 text-xs font-medium text-rose-600 dark:text-rose-400"
  >
    <TrendingDown class="size-3" />{fmtPct(rounded)}
  </span>
{:else}
  <span
    class="inline-flex items-center gap-1 rounded-md bg-secondary px-1.5 py-0.5 text-xs text-muted-foreground"
  >
    <Minus class="size-3" />0%
  </span>
{/if}
