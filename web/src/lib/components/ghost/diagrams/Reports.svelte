<script lang="ts">
  import { WITNESS_GATE } from '$lib/ghost';

  // Below the gate and above it.
  //
  // The count on the left is drawn as an empty slot rather than a zero or a redaction.
  // That is what the payload does: below the gate the field is ABSENT, not suppressed —
  // a count of one identifies the single person who applied to the employer, so there is
  // no number held back, there is no number. A "0" or a "•••" would depict a system that
  // knows and declines to say, which is not the system that exists.
</script>

<div class="flex h-24 items-center gap-3 text-xs" aria-hidden="true">
  <div class="flex flex-1 flex-col gap-2 rounded-md border border-border p-2">
    <span class="text-muted-foreground">1 report</span>
    <span class="flex gap-1">
      <span class="size-2.5 rounded-full bg-muted-foreground/40"></span>
    </span>
    <span class="flex items-center gap-1 text-muted-foreground">
      count:
      <span class="h-3 w-8 rounded-sm border border-dashed border-muted-foreground/50"></span>
    </span>
  </div>

  <div class="flex flex-1 flex-col gap-2 rounded-md border border-warning/60 p-2">
    <span class="text-muted-foreground">{WITNESS_GATE} reports</span>
    <span class="flex gap-1">
      {#each { length: WITNESS_GATE }, i (i)}
        <span class="size-2.5 rounded-full bg-warning"></span>
      {/each}
    </span>
    <span class="text-warning-strong">count: {WITNESS_GATE}</span>
  </div>

  <div class="shrink-0 space-y-1 text-muted-foreground">
    <div>⏱ waits</div>
    <div>↩ withdrawable</div>
  </div>
</div>
