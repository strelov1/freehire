<script lang="ts">
  // A single conversion-rate donut. Presentational: the parent passes an already
  // computed fraction (0–1). Hand-built SVG, no charting dependency.
  let {
    percent,
    label,
    sublabel,
  }: { percent: number; label: string; sublabel?: string } = $props();

  const R = 42;
  const CIRC = 2 * Math.PI * R;

  // Clamp so a malformed fraction can't draw a negative or overflowing arc.
  const fraction = $derived(Math.max(0, Math.min(1, percent)));
  const dash = $derived(`${CIRC * fraction} ${CIRC}`);
  const display = $derived(
    fraction === 0 ? '0%' : `${(fraction * 100).toFixed(fraction < 0.1 ? 1 : 0)}%`,
  );
</script>

<div class="flex flex-col items-center gap-1">
  <svg viewBox="0 0 100 100" class="h-28 w-28" role="img" aria-label="{label}: {display}">
    <circle cx="50" cy="50" r={R} fill="none" stroke-width="9" class="stroke-muted" />
    <circle
      cx="50"
      cy="50"
      r={R}
      fill="none"
      stroke="currentColor"
      stroke-width="9"
      stroke-linecap="round"
      stroke-dasharray={dash}
      transform="rotate(-90 50 50)"
      class="text-foreground transition-[stroke-dasharray] duration-500"
    />
    <text x="50" y="50" dy="0.34em" text-anchor="middle" class="fill-foreground text-[1.45rem] font-semibold">
      {display}
    </text>
  </svg>
  <div class="text-center">
    <p class="text-sm font-medium">{label}</p>
    {#if sublabel}
      <p class="text-xs text-muted-foreground">{sublabel}</p>
    {/if}
  </div>
</div>
