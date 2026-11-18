<script lang="ts">
  import { ConfirmDialog } from '#lib/ui/index.js';
  import type { SessionItem } from '#lib/assistant/sessions.js';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { t } from '#lib/i18n/t.js';
  import { messages } from './AssistantChat.messages';

  // Deleting a chat from the rail, and the confirmation it may need first.
  let {
    sessions,
    newChatLabel,
    onRemove,
  }: {
    sessions: SessionItem[];
    newChatLabel: string;
    onRemove: (id: string) => Promise<void>;
  } = $props();

  const s = $derived(t(messages, locale()));

  let removeTargetId = $state<string | null>(null);
  let confirmRemoveChatOpen = $state(false);

  // A chat that has been named has real history — confirm before deleting. A
  // still-unnamed one has nothing in it worth confirming.
  export function request(id: string) {
    const named = sessions.find((sess) => sess.id === id)?.label !== newChatLabel;
    if (named) {
      removeTargetId = id;
      confirmRemoveChatOpen = true;
    } else {
      void onRemove(id);
    }
  }

  function confirmRemoveChat() {
    if (!removeTargetId) return Promise.resolve();
    return onRemove(removeTargetId);
  }
</script>

<ConfirmDialog
  bind:open={confirmRemoveChatOpen}
  title={s.deleteThisChat}
  description={s.cannotUndo}
  confirmLabel={s.delete}
  variant="destructive"
  onConfirm={confirmRemoveChat}
/>
