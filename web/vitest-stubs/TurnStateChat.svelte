<!-- Test-only stub for AssistantChat: no boot, no network. Buttons drive the callbacks a host
     reads: whether a reply is streaming, which session it is, and that a reply finished. -->
<script lang="ts">
  let {
    onTurnStateChange,
    onSessionChange,
    onTurnComplete,
  }: {
    onTurnStateChange?: (active: boolean) => void;
    onSessionChange?: (id: string) => void;
    onTurnComplete?: () => void;
  } = $props();
</script>

<button type="button" onclick={() => onTurnStateChange?.(true)}>stub: reply starts</button>
<button type="button" onclick={() => onTurnStateChange?.(false)}>stub: reply ends</button>
<button
  type="button"
  onclick={() => {
    onSessionChange?.('support-1');
    onTurnComplete?.();
  }}>stub: reply answered</button
>
