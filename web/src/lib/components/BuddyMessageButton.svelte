<script lang="ts">
  import { messageActionFor, messageActionLabel } from '#lib/buddies.js';
  import type { Conversation } from '#lib/types.js';
  import { Button } from '#lib/ui/index.js';

  // "Message" for one member. What it does depends on the conversation already held with
  // them: none (or one closed by a leave) opens the first-message dialog (`oninvite`), an
  // open or pending one goes to it (`onconversation` — the Chats tab on the buddies page, a
  // dock window elsewhere).
  let {
    handle,
    conversations,
    oninvite,
    onconversation,
    size = 'sm',
  }: {
    handle: string;
    conversations: Conversation[];
    oninvite: (handle: string) => void;
    onconversation: (id: number) => void;
    size?: 'sm' | 'md';
  } = $props();

  const action = $derived(messageActionFor(conversations, handle));
</script>

<Button
  {size}
  variant={action.kind === 'invite' || action.kind === 'open' ? 'primary' : 'outline'}
  onclick={() => (action.kind === 'invite' ? oninvite(handle) : onconversation(action.conversationId))}
>
  {messageActionLabel(action.kind)}
</Button>
