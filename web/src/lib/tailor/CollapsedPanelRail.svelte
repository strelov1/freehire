<script lang="ts">
  import { PanelLeftOpen } from '@lucide/svelte';

  // The workspace's collapsed left panel. Desktop-only, and it carries the one control that
  // brings the panel back — a collapse with no visible way out is a lost panel.
  let { onExpand }: { onExpand: () => void } = $props();
</script>

<div class="hidden shrink-0 flex-col items-center gap-2 border-r border-border bg-background px-1.5 py-2 lg:flex">
  <button
    type="button"
    onclick={onExpand}
    aria-label="Expand the editor panel"
    class="rounded p-1 text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
  >
    <PanelLeftOpen class="size-4" />
  </button>
</div>
