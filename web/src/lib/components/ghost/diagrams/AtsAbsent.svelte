<script lang="ts">
  // Where the role is, and where it is not — plus the state that is neither.
  //
  // The third panel is the one that earns its place. A reader whose posting is NOT
  // marked asks why, and the answer is usually that we do not crawl that employer's
  // board at all, so the criterion is never put to it. Drawing only "found" and
  // "absent" would report our blind spot as the employer's fault, which is exactly what
  // the coverage gate exists to prevent.
</script>

<div class="flex h-24 items-stretch gap-3 text-[10px]" aria-hidden="true">
  <div class="flex flex-1 flex-col rounded-md border border-border p-2">
    <span class="mb-1.5 text-muted-foreground">aggregator</span>
    <span class="mb-1 h-1.5 w-full rounded-full bg-muted-foreground/30"></span>
    <span class="mb-1 h-1.5 w-2/3 rounded-full bg-warning"></span>
    <span class="h-1.5 w-4/5 rounded-full bg-muted-foreground/30"></span>
  </div>

  <div class="flex flex-1 flex-col rounded-md border border-dashed border-warning/60 p-2">
    <span class="mb-1.5 text-muted-foreground">their own board</span>
    <span class="flex flex-1 items-center justify-center text-warning-strong">not there</span>
    <!-- The check expires. A stale answer goes quiet rather than standing. -->
    <span class="text-muted-foreground">checked 2d ago</span>
  </div>

  <div class="flex flex-1 flex-col rounded-md bg-muted p-2">
    <span class="mb-1.5 text-muted-foreground">board we don't crawl</span>
    <span class="flex flex-1 items-center justify-center text-muted-foreground">
      not judged
    </span>
  </div>
</div>
