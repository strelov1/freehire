<script lang="ts">
  import { onDestroy, onMount, tick, untrack } from 'svelte';
  import { ApiError, api } from '#lib/api.js';
  import { buddyConversationName, followsNewMessages, isSendShortcut, mergeBuddyMessages, messageActionFor } from '#lib/buddies.js';
  import { buddyDock } from '#lib/buddyDock.svelte.js';
  import type { BuddyConversation, BuddyMessage, BuddyReport } from '#lib/types.js';
  import { Button, ConfirmDialog } from '#lib/ui/index.js';
  import BuddyIdentity from './BuddyIdentity.svelte';

  // One conversation: its messages, the invitation answer, the composer, block and
  // report. The one implementation behind both the Chats tab and a dock window — mount it
  // keyed by the conversation id, so a different conversation is a fresh pane rather than
  // a pane that has to forget the last one's messages. The composer's draft is held in
  // `buddyDock` by conversation id, so unmounting the pane (minimizing a dock window,
  // switching conversations) never throws away what was typed.
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
    oninvite,
  }: {
    conversation: BuddyConversation;
    /** In a dock window, whose own header already names the member. */
    compact?: boolean;
    /** The conversation was answered or ended from here; `declined` and `blocked` mean it is gone. */
    onchange?: (change: 'accepted' | 'declined' | 'blocked') => void;
    /** A conversation closed by a leave can be started again with a new first message. */
    oninvite?: (handle: string) => void;
  } = $props();

  const id = untrack(() => conversation.id);
  const uid = $props.id();
  const name = $derived(buddyConversationName(conversation.former_member, conversation.other.handle));
  const live = $derived(!conversation.former_member);
  const status = $derived(conversation.status_for_me);
  const reinvite = $derived(
    Boolean(oninvite) && status === 'closed' && live
      && messageActionFor(buddyDock.conversations, conversation.other.handle).kind === 'invite',
  );

  let messages = $state<BuddyMessage[]>([]);
  let body = $state(untrack(() => buddyDock.draft(id)));
  // The first read decides between the history, an empty conversation and a failure; a
  // later poll that fails keeps whatever is already on screen.
  let history = $state<'loading' | 'ready' | 'error'>('loading');
  // New messages are announced only once the history is on screen, so a screen reader is
  // not read the whole conversation on arrival.
  let announce = $state(false);
  // Whether the list stays pinned to the newest message: true until the reader scrolls up.
  let following = true;
  let sendKey = $state('Ctrl');
  let error = $state('');
  let actionBusy = $state(false);
  let confirmBlockOpen = $state(false);
  let reportReason = $state<BuddyReport['reason']>('spam');
  let reportNote = $state('');
  let reportOpen = $state(false);
  let reportSuccess = $state(false);
  let visible = $state(false);
  let list = $state<HTMLElement>();
  let reasonSelect = $state<HTMLSelectElement>();
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
      if (history !== 'ready') {
        history = 'ready';
        void tick().then(() => { announce = true; });
      }
      const lastMessage = messages.at(-1);
      if (fetched && lastMessage) {
        await api.markBuddyRead(id, lastMessage.id);
        if (!destroyed) await buddyDock.refresh();
      }
    } catch {
      // A failed first read is shown with a retry; a later one is recovered by the next poll.
      if (history !== 'ready') history = 'error';
    } finally { pollInFlight = false; }
  }

  function retryHistory() {
    history = 'loading';
    void refreshMessages();
  }

  // Polls only an open conversation, and only while the page is visible.
  $effect(() => {
    if (!visible || conversation.status_for_me !== 'open') return;
    const timer = setInterval(() => void refreshMessages(), POLL_MS);
    return () => clearInterval(timer);
  });

  $effect(() => { buddyDock.setDraft(id, body); });

  // Keep the newest message in view as messages arrive, unless the reader has scrolled up
  // into the history (sending brings them back down; see send).
  $effect(() => {
    void messages.length;
    if (!following) return;
    void tick().then(() => { if (list) list.scrollTop = list.scrollHeight; });
  });

  function onVisibility() {
    visible = document.visibilityState === 'visible';
    if (visible) void refreshMessages();
  }

  function toggleReport() {
    reportOpen = !reportOpen;
    reportSuccess = false;
    if (reportOpen) void tick().then(() => reasonSelect?.focus());
  }

  function onComposerKeydown(e: KeyboardEvent) {
    if (!isSendShortcut(e)) return;
    e.preventDefault();
    if (!actionBusy) void send();
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
      following = true;
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
    if (/Mac|iPhone|iPad/.test(navigator.platform)) sendKey = '⌘';
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
  {#if !compact || (live && (status === 'open' || status === 'incoming'))}
    <header class="flex items-center gap-2 border-b border-border {compact ? 'justify-end px-3 py-2' : 'justify-between pb-3'}">
      {#if !compact}
        {#if live}<BuddyIdentity handle={conversation.other.handle} summary={conversation.other.card_summary} size="sm" />{:else}<strong>{name}</strong>{/if}
      {/if}
      {#if live && (status === 'open' || status === 'incoming')}
        <div class="flex gap-2">
          <Button variant="outline" size="sm" aria-expanded={reportOpen} aria-controls="{uid}-report" onclick={toggleReport}>Report</Button>
          <Button variant="outline" size="sm" onclick={() => (confirmBlockOpen = true)}>Block</Button>
        </div>
      {/if}
    </header>
  {/if}
  <div class="flex min-h-0 flex-1 flex-col {compact ? 'px-3' : ''}">
    {#if error}<p role="alert" class="py-2 text-sm text-destructive">{error}</p>{/if}
    {#if reportSuccess}<p role="status" class="py-2 text-sm text-muted-foreground">Report sent.</p>{/if}
    {#if reportOpen && live}<form id="{uid}-report" aria-label="Report {name}" class="mt-2 flex flex-col gap-2 rounded-md border border-border p-3" onsubmit={(e) => { e.preventDefault(); void report(); }}>
      <label class="text-sm">Reason<select bind:this={reasonSelect} bind:value={reportReason} class="mt-1 block w-full rounded-md border border-border bg-background p-2"><option value="spam">Spam</option><option value="harassment">Harassment</option><option value="scam">Scam</option><option value="other">Other</option></select></label>
      <label class="text-sm">Optional note<textarea bind:value={reportNote} maxlength="1000" rows="3" class="mt-1 block w-full rounded-md border border-border bg-background p-2"></textarea></label>
      <span class="text-right text-xs text-muted-foreground">{reportNote.length}/1000</span>
      <div class="flex justify-end gap-2"><Button variant="outline" type="button" onclick={() => (reportOpen = false)}>Cancel</Button><Button variant="primary" type="submit" disabled={actionBusy || reportNote.length > 1000}>Send report</Button></div>
    </form>{/if}
    <div bind:this={list} onscroll={() => { if (list) following = followsNewMessages(list); }} class="min-h-0 flex-1 overflow-y-auto py-4">
      {#if history === 'loading' && messages.length === 0}
        <p class="text-sm text-muted-foreground">Loading messages…</p>
      {:else if history === 'error'}
        <div role="alert" class="flex flex-col items-start gap-2 text-sm">
          <p class="text-destructive">Could not load the messages.</p>
          <Button variant="outline" size="sm" onclick={retryHistory}>Retry</Button>
        </div>
      {:else if messages.length === 0 && status === 'open' && live}
        <p class="text-sm text-muted-foreground">No messages yet. Say hello to {name}.</p>
      {/if}
      <div role="log" aria-live={announce ? 'polite' : 'off'}>
        <ol aria-label="Messages with {name}" class="flex flex-col gap-3">
          {#each messages as message (message.id)}
            <li class="w-fit max-w-4/5 whitespace-pre-wrap break-words rounded-lg p-3 text-sm {message.mine ? 'self-end bg-brand-muted' : 'self-start bg-muted'}">
              <span class="sr-only">{message.mine ? 'You' : name}: </span>{message.body}<time class="mt-1 block text-right text-xs text-muted-foreground">{new Date(message.created_at).toLocaleString()}</time>
            </li>
          {/each}
        </ol>
      </div>
    </div>
    {#if status === 'incoming' && live}<div class="flex gap-2 border-t border-border py-3"><Button variant="primary" disabled={actionBusy} onclick={() => void invitation('accept')}>Accept</Button><Button variant="outline" disabled={actionBusy} onclick={() => void invitation('decline')}>Decline</Button></div>
    {:else if status === 'outgoing' && live}<p role="status" class="border-t border-border py-3 text-sm text-muted-foreground">Sent. You can keep chatting once they accept.</p>
    {:else if status === 'open' && live}<form class="flex flex-col gap-2 border-t border-border py-3" onsubmit={(e) => { e.preventDefault(); void send(); }}><textarea bind:value={body} maxlength="4000" rows={compact ? 2 : 3} class="w-full rounded-md border border-border bg-background p-3 text-sm" aria-label="Message to {name}" aria-describedby="{uid}-composer-hint" placeholder="Write a message" onkeydown={onComposerKeydown}></textarea><div class="flex items-center justify-between gap-2"><span id="{uid}-composer-hint" class="text-xs text-muted-foreground">{body.length}/4000 · {sendKey}+Enter sends</span><Button variant="primary" size={compact ? 'sm' : 'md'} type="submit" disabled={actionBusy || !body.trim()}>Send</Button></div></form>
    {:else}
      <div class="flex flex-col items-start gap-2 border-t border-border py-3 text-sm text-muted-foreground">
        <p>{live ? 'This conversation closed when one of you left interview buddies.' : 'This member is no longer in interview buddies, so the conversation is closed.'}</p>
        {#if reinvite}<Button variant="primary" size={compact ? 'sm' : 'md'} onclick={() => oninvite?.(conversation.other.handle)}>Message {name} again</Button>{/if}
      </div>
    {/if}
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
