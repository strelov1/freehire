<script lang="ts">
  import { untrack } from 'svelte';
  import { ConfirmDialog } from '#lib/ui/index.js';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { t } from '#lib/i18n/t.js';
  import { messages } from './AssistantChat.messages';

  type Target = { id: string; fromAddress: boolean };

  // The dialog a conversation switch waits on while messages are queued: switching would
  // drop them. Confirming replays the switch (`onConfirm`); closing it otherwise declines it.
  let {
    open = $bindable(),
    onConfirm,
    onDecline,
  }: { open: boolean; onConfirm: (target: Target) => Promise<void>; onDecline: (target: Target) => void } = $props();

  const s = $derived(t(messages, locale()));

  // The switch deferred to this dialog, replayed when it is confirmed.
  let pending = $state<Target | null>(null);

  export function ask(id: string, fromAddress: boolean) {
    pending = { id, fromAddress };
    open = true;
  }

  async function confirm() {
    const target = pending;
    pending = null;
    if (!target) return;
    await onConfirm(target);
  }

  // The dialog closed without confirming (confirm clears `pending` before it resolves, so a
  // target still set here was declined).
  $effect(() => {
    if (open) return;
    untrack(() => {
      const declined = pending;
      if (!declined) return;
      pending = null;
      onDecline(declined);
    });
  });
</script>

<ConfirmDialog
  bind:open
  title={s.unsentMessageConfirm}
  confirmLabel={s.switchAndDiscard}
  variant="destructive"
  onConfirm={confirm}
/>
