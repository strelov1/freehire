<script lang="ts">
  // One conversation, addressed by its id. The id is in the path rather than in a
  // query string so a chat is a real page: bookmarkable, shareable with yourself,
  // and reachable with the browser's back button after switching away from it.
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import AssistantChat from '$lib/assistant/AssistantChat.svelte';
  import AccountNavRail from '$lib/components/AccountNavRail.svelte';

  const session = $derived(page.params.id);

  // Switching chats is a navigation, so each one becomes its own history entry and
  // Back returns to the chat you came from.
  function onSessionChange(id: string) {
    if (id !== page.params.id) void goto(resolve('/my/assistant/[id]', { id }));
  }
</script>

<svelte:head><title>Agent — freehire</title></svelte:head>

<div class="flex h-[calc(100svh-3.5rem)]">
  <AccountNavRail />
  <AssistantChat {session} {onSessionChange} showSessionRail />
</div>
