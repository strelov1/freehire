<script lang="ts">
  import { PIPELINE_BUCKETS } from '$lib/pipeline';
  import type { PipelineBuckets } from '$lib/types';

  // A single-level Sankey snapshot: one Applications source fanning into the
  // status buckets, ribbon and node heights proportional to each count. Hand-built
  // SVG — no charting dependency. The parent guarantees applications > 0 (the
  // empty state is handled there); we guard anyway.
  let { applications, buckets }: { applications: number; buckets: PipelineBuckets } = $props();

  // SVG geometry, in viewBox units (the element scales to its container width).
  // The left source bar fills HH; the right nodes share the same heights but are
  // spread with a gap between them, so the ribbons fan out instead of running flat.
  const W = 460;
  const PAD = 10;
  const GAP = 8;
  const HH = 224;
  const LX = 6;
  const LW = 16;
  const RX = 250;
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
  }

  const model = $derived.by(() => {
    const visible = PIPELINE_BUCKETS.map((b) => ({ ...b, count: buckets[b.key] })).filter(
      (b) => b.count > 0,
    );
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
        key: b.key,
        label: b.label,
        color: b.color,
        count: b.count,
        path: `M ${LX + LW} ${ly0} C ${MID} ${ly0}, ${MID} ${ry0}, ${RX} ${ry0} L ${RX} ${ry1} C ${MID} ${ry1}, ${MID} ${ly1}, ${LX + LW} ${ly1} Z`,
        nodeY: ry0,
        nodeH: h,
        labelY: ry0 + h / 2,
      };
    });
    return { height, barY, ribbons };
  });
</script>

<svg viewBox="0 0 {W} {model.height}" class="w-full" role="img" aria-label="Application pipeline by status">
  <!-- Source bar: all applications -->
  <rect x={LX} y={model.barY} width={LW} height={HH} rx="3" class="fill-foreground" />

  {#each model.ribbons as r (r.key)}
    <path d={r.path} fill={r.color} fill-opacity="0.5" />
    <rect x={RX} y={r.nodeY} width={RW} height={Math.max(r.nodeH, 1)} rx="2" fill={r.color} />
    <text x={RX + RW + 8} y={r.labelY} dy="0.32em" class="fill-foreground text-[0.72rem]">
      {r.label}<tspan class="fill-muted-foreground"> {r.count}</tspan>
    </text>
  {/each}
</svg>
