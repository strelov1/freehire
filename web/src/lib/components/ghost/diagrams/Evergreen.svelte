<script lang="ts">
  // One role, advertised over and over, with several copies live at the same time.
  //
  // There is deliberately NO time axis. The criterion's example copy opens with a
  // posting's age, which invites one — but age alone does not fire it, and in the live
  // catalogue effectively never does: the marked postings converge through the repost
  // and duplicate-copy thresholds instead. A timeline would be the most legible thing
  // here and would name the wrong cause to a reader who came to find out why their
  // posting is marked.
  //
  // The numbers mirror the criterion's illustrative `fact` line, which renders beside
  // this. They are an example, not a threshold.
  const COPIES = 7;
  const REPOSTS = 13;
</script>

<div class="flex h-24 items-center gap-4" aria-hidden="true">
  <!-- A fanned deck: the same posting, several times over. Overlap is the point, so the
       cards are translucent and the stacked region darkens on its own. -->
  <div class="relative h-20 flex-1">
    {#each { length: COPIES }, i (i)}
      <div
        class="absolute h-8 w-24 rounded-sm border border-warning/50 bg-warning/20"
        style="left: {i * 12}px; top: {i * 6}px;"
      ></div>
    {/each}
  </div>

  <div class="shrink-0 font-mono text-xs text-muted-foreground">
    <div class="text-warning-strong">×{COPIES}</div>
    <div>live at once</div>
    <div class="mt-1.5 text-warning-strong">↻ {REPOSTS}</div>
    <div>reposts</div>
  </div>
</div>
