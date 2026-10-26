<script lang="ts">
  import { ApiError, api } from '#lib/api.js';
  import type { Conversation } from '#lib/types.js';
  import { Button, Dialog } from '#lib/ui/index.js';

  // The first message to a member, which is what an invitation is: the conversation opens
  // for writing only once they accept. One instance per page, aimed at a handle by the
  // caller; `onsent` receives the new pending conversation.
  const MAX = 500;
  let {
    open = $bindable(false),
    handle,
    onsent,
  }: { open?: boolean; handle: string; onsent: (conversation: Conversation) => void } = $props();

  let body = $state('');
  let error = $state('');
  let busy = $state(false);

  // A fresh draft for every opening, so a message meant for one member never lingers
  // into the dialog aimed at the next.
  $effect(() => {
    if (open) { body = ''; error = ''; }
  });

  async function send() {
    if (busy || !body.trim() || body.length > MAX) return;
    busy = true; error = '';
    try {
      const conversation = await api.inviteBuddy(handle, body.trim());
      open = false;
      onsent(conversation);
    } catch (err) {
      if (err instanceof ApiError && err.status === 429) {
        const retryAt = err.body?.retry_at;
        error = `${err.message}${typeof retryAt === 'string' ? ` Try again after ${new Date(retryAt).toLocaleString()}.` : ''}`;
      } else error = err instanceof Error ? err.message : 'Could not send the message.';
    } finally { busy = false; }
  }
</script>

<Dialog
  bind:open
  title="Message {handle}"
  description="Your first message is an invitation. You can keep chatting once they accept."
  class="sm:max-w-lg"
>
  <form class="flex flex-col gap-3" onsubmit={(e) => { e.preventDefault(); void send(); }}>
    <textarea bind:value={body} maxlength={MAX} rows="5" class="w-full rounded-md border border-border bg-background p-3 text-sm" aria-label="Message"></textarea>
    <span class="text-right text-xs text-muted-foreground">{body.length}/{MAX}</span>
    {#if error}<p role="alert" class="text-sm text-destructive">{error}</p>{/if}
    <Button type="submit" variant="primary" disabled={busy || !body.trim()}>Send message</Button>
  </form>
</Dialog>
