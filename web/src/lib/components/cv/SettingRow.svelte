<script lang="ts">
  // One settings line: label on the left, control on the right. The workspace's left panel is
  // dragged between 340px and 720px independently of the window, so a viewport breakpoint says
  // nothing about the space a control has — a row that simply lets its label shrink holds at
  // every width, which a multi-column grid of steppers does not.
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
  <!-- `grow` is for controls whose content varies in length (a select showing a font name):
       they take the spare width when the panel is dragged wide, instead of truncating a label
       that would fit. Steppers stay fixed — a number needs no more room than it needs. -->
  <div class={grow ? 'min-w-0 max-w-[18rem] flex-1' : 'shrink-0'}>{@render control()}</div>
</div>
