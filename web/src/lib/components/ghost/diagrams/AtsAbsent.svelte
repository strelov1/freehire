<script lang="ts">
  // Where the role is, and where it is not.
  //
  // The middle panel — the absence itself — is the one that fires, so it carries the
  // weight and the other two are context. Three equal boxes, which is how this began,
  // gave the reader no way to tell the finding from its surroundings.
  //
  // The third panel earns its place by answering the question a reader whose posting is
  // NOT marked arrives with: usually we do not crawl that employer's board at all, so the
  // criterion is never put to it. Drawing only "found" and "absent" would report our
  // blind spot as the employer's fault, which is what the coverage gate exists to prevent.
</script>

<div class="flex h-24 items-center gap-2 text-xs" aria-hidden="true">
  <div class="flex h-16 w-24 shrink-0 flex-col justify-center gap-1.5 rounded-md border border-border px-2.5">
    <span class="text-muted-foreground">aggregator</span>
    <span class="h-1 w-full rounded-full bg-warning"></span>
    <span class="h-1 w-2/3 rounded-full bg-muted-foreground/25"></span>
  </div>

  <span class="shrink-0 text-muted-foreground/50">→</span>

  <div
    class="flex h-20 flex-1 flex-col items-center justify-center gap-0.5 rounded-md border border-dashed border-warning/70 bg-warning/5"
  >
    <span class="text-sm font-medium text-warning-strong">not on their own board</span>
    <span class="text-muted-foreground">checked 2 days ago · expires if we stop</span>
  </div>

  <div class="flex h-16 w-24 shrink-0 items-center rounded-md bg-muted px-2.5 text-muted-foreground">
    no board of theirs → not judged
  </div>
</div>
