<script lang="ts">
  import { Bot } from '@lucide/svelte';
  // The same posting, with the two facts nothing in this title could have told a
  // dictionary — pulled from the body text an LLM actually read, and marked as such
  // rather than blended in beside the derived facets.
</script>

<div class="flex h-28 flex-col justify-center gap-2" aria-hidden="true">
  <div class="rounded-md border border-border bg-card px-3 py-2">
    <p class="truncate text-xs font-medium">Senior Software Engineer — Berlin, DE</p>
  </div>
  <div class="flex flex-col gap-1 pl-1 text-[11px] text-muted-foreground">
    <div class="flex items-center gap-1.5">
      <Bot class="size-3 shrink-0 text-brand" />
      Team size: ~12 engineers
    </div>
    <div class="flex items-center gap-1.5">
      <Bot class="size-3 shrink-0 text-brand" />
      Visa sponsorship: not mentioned
    </div>
  </div>
</div>
