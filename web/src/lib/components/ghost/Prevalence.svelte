<script lang="ts">
  import { PREVALENCE, prevalenceWaffle } from '$lib/ghostDiagrams';

  // A hundred postings, and the share of them nobody is working to fill.
  //
  // The band is drawn differently from the floor because the sources differ: everyone
  // agrees on the lower bound, and the cells above it are the width of the disagreement.
  // One averaged figure would be easier to draw and would claim a precision nobody has —
  // on a page whose whole argument is that it states only what it can check.
  const cells = prevalenceWaffle(PREVALENCE);

  const TONE: Record<string, string> = {
    solid: 'bg-warning',
    banded: 'bg-warning-muted',
    empty: 'bg-muted-foreground/15',
  };
</script>

<figure class="flex flex-wrap items-center gap-5 sm:flex-nowrap">
  <div class="grid w-fit shrink-0 grid-cols-10 gap-1" aria-hidden="true">
    {#each cells as cell, i (i)}
      <span class="size-3.5 rounded-[3px] {TONE[cell]}"></span>
    {/each}
  </div>

  <figcaption class="text-sm leading-relaxed text-muted-foreground">
    <span class="block text-2xl font-semibold tracking-tight text-foreground tabular-nums">
      {PREVALENCE.low}–{PREVALENCE.high}%
    </span>
    of listings are jobs nobody is working to fill — kept up to collect CVs, to look like
    growth, or because nobody took them down. The solid cells are the floor every study
    agrees on; the paler ones are how much they disagree.
  </figcaption>
</figure>
