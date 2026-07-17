<script lang="ts">
  // Thin host: the agent chat now lives in the reusable <AssistantChat> (shared with the
  // /tailor surface). This page just renders it with the session rail, honouring an optional
  // ?session deep-link. The full-width own layout comes from +layout@.svelte.
  import { page } from '$app/state';
  import AssistantChat from '$lib/assistant/AssistantChat.svelte';

  const session = $derived(page.url.searchParams.get('session') ?? undefined);
</script>

<svelte:head><title>Agent — freehire</title></svelte:head>

<div class="flex h-[calc(100svh-3.5rem)]">
  <AssistantChat {session} showSessionRail />
</div>
