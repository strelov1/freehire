<script lang="ts">
  // One settings line: label on the left, control on the right. A caller's
  // available width isn't always tied to the viewport (e.g. a resizable side
  // panel), so a breakpoint can't say how much room the control has — letting
  // the label shrink instead holds at every width a multi-column grid would not.
  import type { Snippet } from 'svelte';

  let {
    label,
    hint,
    grow = false,
    control,
  }: { label: string; hint?: string; grow?: boolean; control: Snippet } = $props();
</script>

<div class="flex items-center justify-between gap-3 py-1">
  <div class="min-w-0">
    <p class="truncate text-sm font-medium text-foreground">{label}</p>
    {#if hint}<p class="truncate text-xs text-muted-foreground">{hint}</p>{/if}
  </div>
  <!-- `grow` is for controls whose content varies in length (e.g. a select showing a
       font name): they take the spare width when the container is wide, instead of
       truncating a label that would otherwise fit. Steppers stay fixed — a number
       needs no more room than it needs. -->
  <div class={grow ? 'min-w-0 max-w-2xs flex-1' : 'shrink-0'}>{@render control()}</div>
</div>
