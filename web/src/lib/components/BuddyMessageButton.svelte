<script lang="ts">
  import { messageActionFor, messageActionLabel } from '#lib/buddies.js';
  import type { BuddyConversation } from '#lib/types.js';
  import { Button } from '#lib/ui/index.js';

  // "Message" for one member. What it does depends on the conversation already held with
  // them: none opens the first-message dialog (`oninvite`), an open or pending one goes to
  // it (`onconversation` — the Chats tab on the buddies page, a dock window elsewhere), and
  // a closed one shows nothing.
  let {
    handle,
    conversations,
    oninvite,
    onconversation,
    size = 'sm',
  }: {
    handle: string;
    conversations: BuddyConversation[];
    oninvite: (handle: string) => void;
    onconversation: (id: number) => void;
    size?: 'sm' | 'md';
  } = $props();

  const action = $derived(messageActionFor(conversations, handle));
</script>

{#if action.kind !== 'none'}
  {@const current = action}
  <Button
    {size}
    variant={current.kind === 'invite' || current.kind === 'open' ? 'primary' : 'outline'}
    onclick={() => (current.kind === 'invite' ? oninvite(handle) : onconversation(current.conversationId))}
  >
    {messageActionLabel(current.kind)}
  </Button>
{/if}
