<script lang="ts">
  import { PIPELINE_BANDS, bandBreakdown, bandTotal } from '$lib/pipeline';
  import { humanizeStage } from '$lib/stages';
  import type { PipelineStats } from '$lib/types';

  // A single-level Sankey snapshot: one Applications source fanning into the four
  // groups, ribbon and node heights proportional to each count. Hand-built SVG — no
  // charting dependency. The parent guarantees applications > 0 (the empty state is
  // handled there); we guard anyway.
  //
  // Each band carries its stages beneath it, because the coarse number alone hides the
  // thing the reader came for: 28 Closed reads the same whether it is 28 rejections or
  // 28 accepted offers.
  let { stats }: { stats: PipelineStats } = $props();
  const applications = $derived(stats.applications);

  // SVG geometry, in viewBox units (the element scales to its container width).
  // The left source bar fills HH; the right nodes share the same heights but are
  // spread with a gap between them, so the ribbons fan out instead of running flat.
  // The viewBox is deliberately WIDE (W ≫ height): the element renders at the full
  // card width, so a wide box keeps the scale factor ≈1 — the funnel stays a
  // reasonable height and the labels read at their intended size rather than
  // ballooning. Right nodes sit near the right edge with just enough room reserved
  // for the labels, so the ribbons span almost the full width.
  const W = 940;
  const PAD = 12;
  const GAP = 10;
  const HH = 230;
  const LX = 8;
  const LW = 18;
  const RX = 760;
  const RW = 14;
  const MID = (LX + LW + RX) / 2;

  interface Ribbon {
    key: string;
    label: string;
    color: string;
    count: number;
    path: string;
    nodeY: number;
    nodeH: number;
    labelY: number;
    breakdown: string;
  }

  const model = $derived.by(() => {
    const visible = PIPELINE_BANDS.map((b) => ({
      ...b,
      count: bandTotal(stats, b),
      // Only the stages actually holding something: a breakdown listing three outcomes
      // where one happened is noise dressed as detail.
      breakdown: bandBreakdown(stats, b).filter((s) => s.count > 0),
    })).filter((b) => b.count > 0);
    if (applications <= 0 || visible.length === 0) {
      return { height: HH + PAD * 2, barY: PAD, ribbons: [] as Ribbon[] };
    }

    const totalGap = GAP * (visible.length - 1);
    const height = HH + totalGap + PAD * 2;
    const barY = PAD + totalGap / 2; // source bar centered against the spread nodes

    let left = barY;
    let right = PAD;
    const ribbons = visible.map((b): Ribbon => {
      const h = (b.count / applications) * HH;
      const ly0 = left;
      const ly1 = left + h;
      const ry0 = right;
      const ry1 = right + h;
      left = ly1;
      right = ry1 + GAP;
      return {
        key: b.id,
        label: b.label,
        color: b.color,
        count: b.count,
        // Rendered only when the band holds more than one kind of thing: "Offer · Offer 2"
        // says nothing the band label has not already said.
        breakdown:
          b.breakdown.length > 1
            ? b.breakdown.map((s) => `${humanizeStage(s.stage)} ${s.count}`).join(' · ')
            : '',
        path: `M ${LX + LW} ${ly0} C ${MID} ${ly0}, ${MID} ${ry0}, ${RX} ${ry0} L ${RX} ${ry1} C ${MID} ${ry1}, ${MID} ${ly1}, ${LX + LW} ${ly1} Z`,
        nodeY: ry0,
        nodeH: h,
        labelY: ry0 + h / 2,
      };
    });
    return { height, barY, ribbons };
  });
</script>

<svg
  viewBox="0 0 {W} {model.height}"
  style="aspect-ratio: {W} / {model.height}"
  class="block h-auto w-full"
  role="img"
  aria-label="Application pipeline by status"
>
  <!-- Source bar: all applications -->
  <rect x={LX} y={model.barY} width={LW} height={HH} rx="3" class="fill-foreground" />

  <!-- The label size is set once on the group and inherited by both lines, so the band's
       total and its per-stage breakdown cannot drift apart typographically. -->
  <g class="text-[0.72rem]">
    {#each model.ribbons as r (r.key)}
      <path d={r.path} fill={r.color} fill-opacity="0.5" />
      <rect x={RX} y={r.nodeY} width={RW} height={Math.max(r.nodeH, 1)} rx="2" fill={r.color} />
      <text
        x={RX + RW + 10}
        y={r.breakdown ? r.labelY - 6 : r.labelY}
        dy="0.32em"
        class="fill-foreground"
      >
        {r.label}<tspan dx="6" class="fill-muted-foreground">{r.count}</tspan>
      </text>
      {#if r.breakdown}
        <text x={RX + RW + 10} y={r.labelY + 7} dy="0.32em" class="fill-muted-foreground">
          {r.breakdown}
        </text>
      {/if}
    {/each}
  </g>
</svg>
