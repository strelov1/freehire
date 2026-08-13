<script lang="ts">
  // Owns the `active` state so a reviewer can click/arrow through the strip and see
  // selection, scroll-into-view and the fade mask actually move.
  import TabStrip from '../../src/tab-strip.svelte';

  let { tabs }: { tabs: readonly { id: string; label: string }[] } = $props();
  let active = $state(tabs[0]?.id ?? '');
</script>

<div class="max-w-md">
  <TabStrip
    {tabs}
    {active}
    onSelect={(id) => (active = id)}
    label="Demo sections"
    panelId="demo-panel"
  />
</div>
