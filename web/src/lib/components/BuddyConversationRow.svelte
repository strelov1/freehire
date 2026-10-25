<script lang="ts">
  import { buddyConversationName, buddyOutgoingLabel } from '#lib/buddies.js';
  import type { BuddyConversation } from '#lib/types.js';
  import BuddyIdentity from './BuddyIdentity.svelte';

  // One conversation in a list: who, how many unread, and where it stands. Shared by the
  // Chats tab and the messaging dock so the two lists read the same.
  let {
    conversation,
    selected = false,
    onselect,
  }: { conversation: BuddyConversation; selected?: boolean; onselect: (c: BuddyConversation) => void } = $props();

  const name = $derived(buddyConversationName(conversation.former_member, conversation.other.handle));
</script>

<button
  type="button"
  class="w-full p-3 text-left hover:bg-accent {selected ? 'bg-accent' : ''}"
  aria-pressed={selected}
  onclick={() => onselect(conversation)}
>
  <span class="flex items-start justify-between gap-2">
    {#if conversation.former_member}
      <span class="truncate text-sm font-medium">{name}</span>
    {:else}
      <BuddyIdentity handle={conversation.other.handle} summary={conversation.other.card_summary} size="sm" link={false} />
    {/if}
    {#if conversation.unread}<span class="shrink-0 rounded-full bg-brand px-2 text-xs text-brand-foreground">{conversation.unread}<span class="sr-only"> unread</span></span>{/if}
  </span>
  <span class="mt-1 block text-xs text-muted-foreground {conversation.former_member ? '' : 'pl-11'}">{buddyOutgoingLabel(conversation.status_for_me)}</span>
</button>
