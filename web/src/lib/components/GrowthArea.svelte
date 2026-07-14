<script lang="ts">
  import type { UserGrowthPoint } from '$lib/types';

  // A minimal single-series cumulative area chart for the member-growth series —
  // one monotonically rising line, unlike ActivityBars' two-series added/removed
  // bars. Hand-built SVG scaled to its container width, no charting dependency,
  // matching the site's other bespoke charts. The series is dense and sorted by
  // date (the backend guarantees it), so the x-axis is just even spacing.
  let { points }: { points: UserGrowthPoint[] } = $props();

  const W = 640;
  const H = 180;
  const PAD = 10;
  const baseY = H - PAD;

  const model = $derived.by(() => {
    const n = points.length;
    const first = points[0];
    const last = points[n - 1];
    if (!first || !last) return null;
    const peak = Math.max(1, last.total); // monotonic → last point is the max
    const x = (i: number) => (n === 1 ? W / 2 : PAD + (i / (n - 1)) * (W - 2 * PAD));
    const y = (t: number) => PAD + (1 - t / peak) * (H - 2 * PAD);
    const line = points.map((p, i) => `${x(i).toFixed(1)},${y(p.total).toFixed(1)}`).join(' ');
    const area = `${x(0).toFixed(1)},${baseY} ${line} ${x(n - 1).toFixed(1)},${baseY}`;
    return {
      peak,
      line,
      area,
      endX: x(n - 1),
      endY: y(last.total),
      first: first.date,
      last: last.date,
    };
  });

  function shortDate(iso: string): string {
    return new Date(iso + 'T00:00:00Z').toLocaleDateString(undefined, {
      month: 'short',
      year: 'numeric',
      timeZone: 'UTC',
    });
  }
</script>

{#if !model}
  <p class="py-16 text-center text-sm text-muted-foreground">No members yet.</p>
{:else}
  <figure class="flex flex-col gap-3">
    <svg viewBox="0 0 {W} {H + 20}" class="w-full" role="img" aria-label="Cumulative members over time">
      <!-- peak reference line + label -->
      <line x1={PAD} y1={PAD} x2={W - PAD} y2={PAD} class="stroke-border" stroke-dasharray="2 3" />
      <text x={PAD} y={PAD - 2} class="fill-muted-foreground" font-size="11">
        {model.peak.toLocaleString()}
      </text>

      <!-- baseline -->
      <line x1="0" y1={baseY} x2={W} y2={baseY} class="stroke-border" stroke-width="1" />

      <!-- filled area + line -->
      <polygon points={model.area} class="fill-foreground/10" />
      <polyline
        points={model.line}
        fill="none"
        class="stroke-foreground"
        stroke-width="2"
        stroke-linejoin="round"
      />
      <!-- end marker -->
      <circle cx={model.endX} cy={model.endY} r="3.5" class="fill-foreground" />

      <!-- x-axis endpoints -->
      <text x={PAD} y={baseY + 15} class="fill-muted-foreground" font-size="11">
        {shortDate(model.first)}
      </text>
      <text x={W - PAD} y={baseY + 15} text-anchor="end" class="fill-muted-foreground" font-size="11">
        {shortDate(model.last)}
      </text>
    </svg>
  </figure>
{/if}
