<script lang="ts">
  import { onDestroy, onMount, tick, untrack } from 'svelte';
  import { ApiError, api } from '#lib/api.js';
  import { buddyConversationName, mergeBuddyMessages } from '#lib/buddies.js';
  import { buddyDock } from '#lib/buddyDock.svelte.js';
  import type { BuddyConversation, BuddyMessage, BuddyReport } from '#lib/types.js';
  import { Button, ConfirmDialog } from '#lib/ui/index.js';

  // One conversation: its messages, the invitation answer, the composer, block and
  // report. The one implementation behind both the Chats tab and a dock window — mount it
  // keyed by the conversation id, so a different conversation is a fresh pane rather than
  // a pane that has to forget the last one's messages and drafts.
  //
  // `conversation` is the caller's live copy from the shared list, so a status the other
  // member changed (an accepted invitation, a departure) arrives through the next list
  // read. Every change made here re-reads that list, which is what keeps the unread
  // counts in the dock and the Chats tab in step.

  // A conversation is read page by page until a page comes back short, bounded so a runaway
  // history cannot spin the loop.
  const PAGE_SIZE = 100;
  const MAX_PAGES = 20;
  const POLL_MS = 5000;

  let {
    conversation,
    compact = false,
    onchange,
  }: {
    conversation: BuddyConversation;
    /** In a dock window, whose own header already names the member. */
    compact?: boolean;
    /** The conversation was answered or ended from here; `declined` and `blocked` mean it is gone. */
    onchange?: (change: 'accepted' | 'declined' | 'blocked') => void;
  } = $props();

  const id = untrack(() => conversation.id);
  const name = $derived(buddyConversationName(conversation.former_member, conversation.other.handle));
  const live = $derived(!conversation.former_member);

  let messages = $state<BuddyMessage[]>([]);
  let body = $state('');
  let error = $state('');
  let actionBusy = $state(false);
  let confirmBlockOpen = $state(false);
  let reportReason = $state<BuddyReport['reason']>('spam');
  let reportNote = $state('');
  let reportOpen = $state(false);
  let reportSuccess = $state(false);
  let visible = $state(false);
  let list = $state<HTMLOListElement>();
  let pollInFlight = false;
  let destroyed = false;

  async function refreshMessages() {
    if (destroyed || document.visibilityState === 'hidden' || pollInFlight) return;
    pollInFlight = true;
    try {
      let fetched = 0;
      for (let i = 0; i < MAX_PAGES; i++) {
        const page = await api.listBuddyMessages(id, messages.at(-1)?.id, PAGE_SIZE);
        if (destroyed) return;
        messages = mergeBuddyMessages(messages, page);
        fetched += page.length;
        if (page.length < PAGE_SIZE) break;
      }
      const lastMessage = messages.at(-1);
      if (fetched && lastMessage) {
        await api.markBuddyRead(id, lastMessage.id);
        if (!destroyed) await buddyDock.refresh();
      }
    } catch { /* A transient poll failure is recovered by the next poll. */ }
    finally { pollInFlight = false; }
  }

  // Polls only an open conversation, and only while the page is visible.
  $effect(() => {
    if (!visible || conversation.status_for_me !== 'open') return;
    const timer = setInterval(() => void refreshMessages(), POLL_MS);
    return () => clearInterval(timer);
  });

  // Keep the newest message in view as messages arrive.
  $effect(() => {
    void messages.length;
    void tick().then(() => { if (list) list.scrollTop = list.scrollHeight; });
  });

  function onVisibility() {
    visible = document.visibilityState === 'visible';
    if (visible) void refreshMessages();
  }

  async function invitation(action: 'accept' | 'decline') {
    actionBusy = true;
    error = '';
    try {
      await api.decideBuddyInvitation(id, action);
      onchange?.(action === 'accept' ? 'accepted' : 'declined');
      await buddyDock.refresh().catch(() => { /* the next poll catches up */ });
      if (action === 'accept') void refreshMessages();
    }
    catch (err) { error = err instanceof Error ? err.message : 'Could not update the invitation.'; }
    finally { actionBusy = false; }
  }
  async function send() {
    const text = body.trim();
    if (!text || body.length > 4000) return;
    actionBusy = true;
    error = '';
    try {
      const sent = await api.sendBuddyMessage(id, text);
      body = '';
      messages = mergeBuddyMessages(messages, [sent]);
      void buddyDock.refresh().catch(() => { /* the next poll catches up */ });
    }
    catch (err) { error = err instanceof Error ? err.message : 'Could not send message.'; }
    finally { actionBusy = false; }
  }
  async function block() {
    error = '';
    try {
      await api.blockBuddyConversation(id);
      onchange?.('blocked');
      await buddyDock.refresh().catch(() => { /* the next poll catches up */ });
    }
    catch (err) { error = err instanceof Error ? err.message : 'Could not block this member.'; }
  }
  async function report() {
    if (reportNote.length > 1000) return;
    error = ''; reportSuccess = false; actionBusy = true;
    try { await api.reportBuddyConversation(id, reportReason, reportNote.trim()); reportSuccess = true; reportOpen = false; reportNote = ''; }
    catch (err) { error = err instanceof ApiError ? err.message : 'Could not send report.'; }
    finally { actionBusy = false; }
  }

  onMount(() => {
    visible = document.visibilityState === 'visible';
    document.addEventListener('visibilitychange', onVisibility);
    void refreshMessages();
  });
  onDestroy(() => {
    destroyed = true;
    if (typeof document !== 'undefined') document.removeEventListener('visibilitychange', onVisibility);
  });
</script>

<div class="flex min-h-0 flex-1 flex-col">
  {#if !compact || (live && (conversation.status_for_me === 'open' || conversation.status_for_me === 'incoming'))}
    <header class="flex items-center gap-2 border-b border-border {compact ? 'justify-end px-3 py-2' : 'justify-between pb-3'}">
      {#if !compact}<strong>{name}</strong>{/if}
      {#if live && (conversation.status_for_me === 'open' || conversation.status_for_me === 'incoming')}
        <div class="flex gap-2">
          <Button variant="outline" size="sm" onclick={() => { reportOpen = !reportOpen; reportSuccess = false; }}>Report</Button>
          <Button variant="outline" size="sm" onclick={() => (confirmBlockOpen = true)}>Block</Button>
        </div>
      {/if}
    </header>
  {/if}
  <div class="flex min-h-0 flex-1 flex-col {compact ? 'px-3' : ''}">
    {#if error}<p role="alert" class="py-2 text-sm text-destructive">{error}</p>{/if}
    {#if reportSuccess}<p role="status" class="py-2 text-sm text-muted-foreground">Report sent.</p>{/if}
    {#if reportOpen && live}<form class="mt-2 flex flex-col gap-2 rounded-md border border-border p-3" onsubmit={(e) => { e.preventDefault(); void report(); }}>
      <label class="text-sm">Reason<select bind:value={reportReason} class="mt-1 block w-full rounded-md border border-border bg-background p-2"><option value="spam">Spam</option><option value="harassment">Harassment</option><option value="scam">Scam</option><option value="other">Other</option></select></label>
      <label class="text-sm">Optional note<textarea bind:value={reportNote} maxlength="1000" rows="3" class="mt-1 block w-full rounded-md border border-border bg-background p-2"></textarea></label>
      <span class="text-right text-xs text-muted-foreground">{reportNote.length}/1000</span>
      <div class="flex justify-end gap-2"><Button variant="outline" type="button" onclick={() => (reportOpen = false)}>Cancel</Button><Button variant="primary" type="submit" disabled={actionBusy || reportNote.length > 1000}>Send report</Button></div>
    </form>{/if}
    <ol bind:this={list} class="flex flex-1 flex-col gap-3 overflow-y-auto py-4" aria-label="Messages">
      {#each messages as message (message.id)}<li class="max-w-prose rounded-lg bg-muted p-3 text-sm {message.mine ? 'self-end' : 'self-start'}">{message.body}<time class="mt-1 block text-right text-xs text-muted-foreground">{new Date(message.created_at).toLocaleString()}</time></li>{/each}
    </ol>
    {#if conversation.status_for_me === 'incoming' && live}<div class="flex gap-2 border-t border-border py-3"><Button variant="primary" disabled={actionBusy} onclick={() => void invitation('accept')}>Accept</Button><Button variant="outline" disabled={actionBusy} onclick={() => void invitation('decline')}>Decline</Button></div>
    {:else if conversation.status_for_me === 'outgoing' && live}<p role="status" class="border-t border-border py-3 text-sm text-muted-foreground">Sent. You can keep chatting once they accept.</p>
    {:else if conversation.status_for_me === 'open' && live}<form class="flex flex-col gap-2 border-t border-border py-3" onsubmit={(e) => { e.preventDefault(); void send(); }}><textarea bind:value={body} maxlength="4000" rows={compact ? 2 : 3} class="w-full rounded-md border border-border bg-background p-3 text-sm" aria-label="Message to {name}"></textarea><div class="flex items-center justify-between"><span class="text-xs text-muted-foreground">{body.length}/4000</span><Button variant="primary" size={compact ? 'sm' : 'md'} type="submit" disabled={actionBusy || !body.trim()}>Send</Button></div></form>{/if}
  </div>
</div>

<ConfirmDialog
  bind:open={confirmBlockOpen}
  title={`Block ${name}?`}
  description="The conversation closes for good, and you stop seeing each other in lists, the feed and invitations. This cannot be undone."
  confirmLabel="Block"
  variant="destructive"
  onConfirm={block}
/>
