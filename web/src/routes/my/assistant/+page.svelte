<script lang="ts">
  // The bare /my/assistant entry point: it opens the caller's newest conversation
  // (or starts one) and then rewrites the URL to that chat's own address. The
  // rewrite REPLACES this entry rather than pushing a new one — landing here is
  // not a step in the user's history, it is a redirect to where they belong.
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import AssistantChat from '$lib/assistant/AssistantChat.svelte';
  import AccountNavRail from '$lib/components/AccountNavRail.svelte';

  function onSessionChange(id: string) {
    if (page.url.pathname === '/my/assistant') {
      void goto(resolve('/my/assistant/[id]', { id }), { replaceState: true });
    }
  }
</script>

<svelte:head><title>Agent — freehire</title></svelte:head>

<div class="flex h-[calc(100dvh-3.5rem)]">
  <AccountNavRail />
  <AssistantChat {onSessionChange} showSessionRail />
</div>
