<script lang="ts">
  // Every chat, at one address. The id is optional: `/my/assistant` opens the newest
  // conversation (or starts one) and rewrites the address to that chat's own, while
  // `/my/assistant/<id>` opens the chat it names. The id is in the path rather than a
  // query string so a chat is a real page — bookmarkable, shareable with yourself, and
  // reachable with the browser's back button after switching away from it.
  //
  // Both live in ONE route node on purpose. As two pages, the rewrite from the bare
  // address to a chat's own address swapped one <AssistantChat> for another, and the
  // unmount cancelled whatever turn had just started — which is exactly what an opening
  // message needs to survive.
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import AssistantChat from '$lib/assistant/AssistantChat.svelte';
  import AccountNavRail from '$lib/components/AccountNavRail.svelte';
  import { entryFromQuery, historyModeFor } from '$lib/assistant/presets';

  const session = $derived(page.params.id);
  // Read once, not reactively: the query says what this ARRIVAL asked for, and the
  // address is rewritten to the session's own URL — dropping the query — moments later.
  const entry = entryFromQuery(page.url.searchParams);

  function onSessionChange(id: string) {
    const mode = historyModeFor(page.params.id, id);
    if (mode === 'none') return;
    void goto(resolve('/my/assistant/[[id]]', { id }), { replaceState: mode === 'replace' });
  }
</script>

<svelte:head><title>Agent — freehire</title></svelte:head>

<div class="flex h-[calc(100dvh-3.5rem)]">
  <AccountNavRail />
  <AssistantChat
    {session}
    preset={entry.preset}
    kickoff={entry.kickoff}
    {onSessionChange}
    showSessionRail
  />
</div>
