<script lang="ts">
  // A static, single-level Sankey snapshot for the landing page: one Applications
  // source bar fanning into the pipeline groups, ribbon and node heights
  // proportional to each count. Hand-built SVG — no charting dependency and no
  // live data; the counts are decorative props passed in by the homepage.
  //
  // The geometry is still this file's own — the real funnel now draws a per-stage
  // breakdown under each band, which a decorative preview has no use for. What the two
  // share is the vocabulary, which is the part that must not drift.

  import { PIPELINE_BANDS } from '$lib/pipeline';

  let { applications, buckets }: { applications: number; buckets: Record<string, number> } =
    $props();

  // The bands are the product's own, shared with the real Pipeline tab. The numbers below
  // them are illustrative, but the words must not be: a landing page naming a stage the
  // product does not have is a promise it cannot keep. This file used to carry a fourth
  // copy of the vocabulary, and it had already drifted from the other three.
  const VOCAB = PIPELINE_BANDS.map((b) => ({ key: b.id, label: b.label, color: b.color }));

  // SVG geometry, in viewBox units (the element scales to its container width).
  // The left source bar fills HH; the right nodes share the same heights but are
  // spread with a gap between them, so the ribbons fan out instead of running flat.
  // The viewBox is deliberately wide (W ≫ HH) so a full-width figure renders at a
  // sensible height — the aspect ratio, not the container, caps how tall it gets.
  const W = 760;
  const PAD = 10;
  const GAP = 8;
  const HH = 224;
  const LX = 6;
  const LW = 16;
  const RX = 470;
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
    const visible = VOCAB.map((b) => ({ ...b, count: buckets[b.key] ?? 0 })).filter(
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

<svg
  viewBox="0 0 {W} {model.height}"
  class="w-full"
  role="img"
  aria-label="Application pipeline by status"
>
  <!-- Source bar: all applications -->
  <rect x={LX} y={model.barY} width={LW} height={HH} rx="3" class="fill-foreground" />

  {#each model.ribbons as r (r.key)}
    <path d={r.path} fill={r.color} fill-opacity="0.5" />
    <rect x={RX} y={r.nodeY} width={RW} height={Math.max(r.nodeH, 1)} rx="2" fill={r.color} />
    <text x={RX + RW + 8} y={r.labelY} dy="0.32em" class="fill-foreground text-[0.72rem]">
      {r.label}<tspan dx="7" class="fill-muted-foreground">{r.count}</tspan>
    </text>
  {/each}
</svg>
