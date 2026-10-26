<script lang="ts">
  import { conversationStatusLabel } from '#lib/messaging.js';
  import type { Conversation } from '#lib/types.js';
  import ConversationCounterpart from './ConversationCounterpart.svelte';
  import UnreadBadge from './UnreadBadge.svelte';

  // One conversation in a list: who, how many unread, and where it stands. Shared by the
  // buddies Chats tab and the messaging dock so the two lists read the same.
  let {
    conversation,
    selected = false,
    onselect,
  }: { conversation: Conversation; selected?: boolean; onselect: (c: Conversation) => void } = $props();
</script>

<button
  type="button"
  class="w-full p-3 text-left hover:bg-accent {selected ? 'bg-accent' : ''}"
  aria-pressed={selected}
  onclick={() => onselect(conversation)}
>
  <span class="flex items-start justify-between gap-2">
    <ConversationCounterpart counterpart={conversation.counterpart} nameClass="truncate text-sm font-medium" />
    <UnreadBadge count={conversation.unread} />
  </span>
  <!-- Indented under the avatar a reachable counterpart is drawn with. -->
  <span class="mt-1 block text-xs text-muted-foreground {conversation.counterpart.reachable ? 'pl-11' : ''}">{conversationStatusLabel(conversation.status_for_me)}</span>
</button>
