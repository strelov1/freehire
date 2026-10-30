<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import BuddyChatView from '#lib/components/BuddyChatView.svelte';
  import BuddyInviteDialog from '#lib/components/BuddyInviteDialog.svelte';

  // `?c=<id>` (the unread notice's link) names the conversation to open. It is read from the
  // address on every navigation, not once: following a notification link while this page is
  // already open reuses the component, and the view must move to the conversation it names.
  const linked = $derived.by(() => {
    const id = Number(page.url.searchParams.get('c'));
    return Number.isInteger(id) && id > 0 ? id : null;
  });
  let inviteHandle = $state('');
  let inviteOpen = $state(false);

  // A fresh invitation (re-opening a conversation closed by a leave) lands on it, through
  // the same address a notification would use.
  function openSent(id: number) {
    // eslint-disable-next-line svelte/no-navigation-without-resolve -- query string appended to a resolved path
    void goto(`${resolve('/buddies/chats')}?c=${id}`);
  }
</script>

<svelte:head><title>Chats · Interview buddies</title></svelte:head>

<h1 class="sr-only">Chats</h1>
<!-- BuddyChatView reads its initial conversation once, so a new one means a new view. -->
{#key linked}
  <BuddyChatView initialId={linked} oninvite={(h) => { inviteHandle = h; inviteOpen = true; }} />
{/key}
<BuddyInviteDialog bind:open={inviteOpen} handle={inviteHandle} onsent={(conversation) => openSent(conversation.id)} />
