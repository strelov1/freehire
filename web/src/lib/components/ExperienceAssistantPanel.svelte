<script lang="ts">
  /**
   * The experience interviewer, opened beside the bank in the shared docked panel.
   */
  import AssistantChat from '#lib/assistant/AssistantChat.svelte';
  import AssistantPanel from '#lib/components/AssistantPanel.svelte';

  let {
    open = false,
    launch,
    onClose,
    onBankChanged,
    onTurnStateChange = undefined,
  }: {
    open?: boolean;
    /**
     * Which conversation to hold, and what to open it with. The id is a token, not an
     * identifier the server knows: bumping it remounts the chat, which is the only way to
     * aim it at a new subject — `AssistantChat` reads `kickoff` once at construction into a
     * deliberately non-reactive `arrival` and spends it after the first send, so setting the
     * prop on a live chat does nothing at all.
     */
    launch: { id: number; kickoff: string };
    onClose: () => void;
    /** A turn finished; the bank behind this panel may no longer match what it shows. */
    onBankChanged: () => void;
    onTurnStateChange?: (active: boolean) => void;
  } = $props();
</script>

<AssistantPanel {open} title="Experience assistant" {onClose}>
  {#snippet children(setSessionId)}
    {#key launch.id}
      <AssistantChat
        preset="profile"
        kickoff={launch.kickoff}
        showSessionRail={false}
        sessionLabel="Experience"
        onSessionChange={setSessionId}
        onTurnComplete={onBankChanged}
        {onTurnStateChange}
      />
    {/key}
  {/snippet}
</AssistantPanel>
