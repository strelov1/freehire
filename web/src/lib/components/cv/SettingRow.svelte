<script lang="ts">
  // One settings line: label on the left, control on the right. The workspace's left panel is
  // dragged between 340px and 720px independently of the window, so a viewport breakpoint says
  // nothing about the space a control has — a row that simply lets its label shrink holds at
  // every width, which a multi-column grid of steppers does not.
  import type { Snippet } from 'svelte';

  let {
    label,
    hint,
    control,
  }: { label: string; hint?: string; control: Snippet } = $props();
</script>

<div class="flex items-center justify-between gap-3 py-1">
  <div class="min-w-0">
    <p class="truncate text-sm font-medium text-foreground">{label}</p>
    {#if hint}<p class="truncate text-xs text-muted-foreground">{hint}</p>{/if}
  </div>
  <div class="shrink-0">{@render control()}</div>
</div>
