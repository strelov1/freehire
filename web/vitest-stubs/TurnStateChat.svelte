<!-- Test-only stub for AssistantChat: no boot, no network. Two buttons drive the
     onTurnStateChange callback a host reads to know whether a reply is streaming. -->
<script lang="ts">
  let { onTurnStateChange }: { onTurnStateChange?: (active: boolean) => void } = $props();
</script>

<button type="button" onclick={() => onTurnStateChange?.(true)}>stub: reply starts</button>
<button type="button" onclick={() => onTurnStateChange?.(false)}>stub: reply ends</button>
