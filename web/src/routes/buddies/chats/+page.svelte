<script lang="ts">
  import { page } from '$app/state';
  import BuddyChatView from '#lib/components/BuddyChatView.svelte';
  import BuddyInviteDialog from '#lib/components/BuddyInviteDialog.svelte';

  // `?c=<id>` (the unread notice's link) names the conversation to open.
  const linkedParam = Number(page.url.searchParams.get('c'));
  let linked = $state<number | null>(Number.isInteger(linkedParam) && linkedParam > 0 ? linkedParam : null);
  let inviteHandle = $state('');
  let inviteOpen = $state(false);
  // A fresh invitation (re-opening a conversation closed by a leave) lands on it here.
  let chatKey = $state(0);
</script>

<svelte:head><title>Chats · Interview buddies</title></svelte:head>

<h1 class="sr-only">Chats</h1>
{#key chatKey}
  <BuddyChatView initialId={linked} oninvite={(h) => { inviteHandle = h; inviteOpen = true; }} />
{/key}
<BuddyInviteDialog bind:open={inviteOpen} handle={inviteHandle} onsent={(conversation) => { linked = conversation.id; chatKey++; }} />
