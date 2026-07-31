<script lang="ts">
  import { PREVALENCE, prevalenceWaffle } from '$lib/ghostDiagrams';

  // A hundred postings, and the share of them nobody is working to fill.
  //
  // The band is drawn differently from the floor because the sources differ: everyone
  // agrees on the lower bound, and the cells above it are the width of the disagreement.
  // One averaged figure would be easier to draw and would claim a precision nobody has —
  // on a page whose whole argument is that it states only what it can check.
  //
  // The legend is two words per state rather than a sentence explaining the picture. A
  // graphic that needs a paragraph to be read is not carrying its weight, and the
  // paragraph was longer than the claim it annotated.
  const cells = prevalenceWaffle(PREVALENCE);

  // The band carries a ring as well as a tint. In dark the muted warning token lands at
  // almost exactly the lightness of the neutral remainder, so hue alone separates them —
  // and hue alone is the one distinction a colour-blind reader does not get. The outline
  // makes the band a different SHAPE, which survives both themes and any vision.
  const TONE: Record<string, string> = {
    solid: 'bg-warning',
    banded: 'bg-warning-muted ring-1 ring-inset ring-warning/50',
    // Barely there on purpose. Three quarters of the grid is the remainder, and drawn at
    // any real contrast those 73 cells become the texture the eye reads first — the
    // opposite of what the figure is for.
    empty: 'bg-muted-foreground/8',
  };
</script>

<figure class="flex w-fit flex-col gap-4">
  <figcaption class="max-w-xs">
    <span class="block text-4xl font-semibold tracking-tight tabular-nums">
      {PREVALENCE.low}–{PREVALENCE.high}%
    </span>
    <span class="mt-1 block text-sm leading-relaxed text-muted-foreground">
      of listings on the big boards are jobs nobody is working to fill.
    </span>
  </figcaption>

  <div class="grid w-fit grid-cols-10 gap-1" aria-hidden="true">
    {#each cells as cell, i (i)}
      <span class="size-3.5 rounded-sm {TONE[cell]}"></span>
    {/each}
  </div>

  <div class="flex gap-4 text-xs text-muted-foreground" aria-hidden="true">
    <span class="flex items-center gap-1.5">
      <span class="size-2.5 rounded-sm {TONE.solid}"></span>
      agreed
    </span>
    <span class="flex items-center gap-1.5">
      <span class="size-2.5 rounded-sm {TONE.banded}"></span>
      disputed
    </span>
  </div>
</figure>
