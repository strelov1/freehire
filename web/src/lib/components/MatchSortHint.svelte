<script lang="ts">
  import { Button } from '#lib/ui/index.js';

  // The job list's hint while a minimum skill match (`minMatch`, a percentage) filters a feed
  // sorted some other way; `onSort` switches it to best match.
  let { minMatch, onSort }: { minMatch: number | null; onSort: () => void } = $props();
</script>

<!-- The threshold trims each fetched page, so under any other ordering the jobs that
     clear it are scattered across pages. Best match brings them to the front. -->
<div
  role="status"
  class="my-4 flex flex-col gap-3 rounded-xl border border-border bg-muted/40 p-3 text-sm sm:flex-row sm:items-center sm:justify-between"
>
  <p class="text-muted-foreground">
    Your {minMatch}%+ skill match filters one page at a time. Sort by best match to see those jobs first.
  </p>
  <Button variant="secondary" size="sm" onclick={onSort} class="shrink-0">Sort by best match</Button>
</div>
