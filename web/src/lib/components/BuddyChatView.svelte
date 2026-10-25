<script lang="ts">
  import { onDestroy, onMount } from 'svelte';
  import { ApiError, api } from '#lib/api.js';
  import type { BuddyConversation, BuddyMessage, BuddyReport } from '#lib/types.js';
  import { Button } from '#lib/ui/index.js';
  import { buddyConversationName, buddyOutgoingLabel, mergeBuddyMessages } from '#lib/buddies.js';
  import States from './States.svelte';

  // A conversation is read page by page until a page comes back short, bounded so a runaway
  // history cannot spin the loop.
  const PAGE_SIZE = 100;
  const MAX_PAGES = 20;
  let { initialId = null }: { initialId?: number | null } = $props();
  let deepLinkPending = true;

  let conversations = $state<BuddyConversation[]>([]);
  let messages = $state<BuddyMessage[]>([]);
  let selected = $state<BuddyConversation | null>(null);
  let body = $state('');
  let status = $state<'loading' | 'ready' | 'error'>('loading');
  let error = $state('');
  let actionBusy = $state(false);
  let reportReason = $state<BuddyReport['reason']>('spam');
  let reportNote = $state('');
  let reportOpen = $state(false);
  let reportSuccess = $state(false);
  let listTimer: ReturnType<typeof setInterval> | undefined;
  let messageTimer: ReturnType<typeof setInterval> | undefined;
  let messagePollInFlight = false;

  function stopMessagePolling() {
    if (messageTimer) clearInterval(messageTimer);
    messageTimer = undefined;
  }
  function clearSelection() {
    selected = null; messages = []; reportOpen = false;
    stopMessagePolling();
  }

  async function refreshList() {
    try {
      conversations = await api.listBuddyConversations();
      if (selected) selected = conversations.find((c) => c.id === selected?.id) ?? null;
      if (deepLinkPending) {
        deepLinkPending = false;
        const linked = initialId === null ? undefined : conversations.find((c) => c.id === initialId);
        if (linked) choose(linked);
      }
      if (!selected && messageTimer) { stopMessagePolling(); reportOpen = false; }
      status = 'ready';
    }
    catch { status = 'error'; }
  }
  async function refreshMessages() {
    const conv = selected;
    if (!conv || document.visibilityState === 'hidden' || messagePollInFlight) return;
    messagePollInFlight = true;
    try {
      let fetched = 0;
      for (let i = 0; i < MAX_PAGES; i++) {
        const page = await api.listBuddyMessages(conv.id, messages.at(-1)?.id, PAGE_SIZE);
        if (selected?.id !== conv.id) return;
        messages = mergeBuddyMessages(messages, page);
        fetched += page.length;
        if (page.length < PAGE_SIZE) break;
      }
      const lastMessage = messages.at(-1);
      if (fetched && lastMessage) {
        await api.markBuddyRead(conv.id, lastMessage.id);
        if (selected?.id !== conv.id) return;
        await refreshList();
      }
    } catch { /* A transient poll failure is recovered by the next poll. */ }
    finally {
      messagePollInFlight = false;
      if (selected && selected.id !== conv.id) void refreshMessages();
    }
  }
  function choose(c: BuddyConversation) {
    selected = c; messages = [];
    reportOpen = false;
    stopMessagePolling();
    void refreshMessages();
    if (c.status_for_me === 'open' && document.visibilityState === 'visible') messageTimer = setInterval(() => void refreshMessages(), 5000);
  }
  function onVisibility() {
    if (document.visibilityState === 'hidden') {
      if (listTimer) clearInterval(listTimer);
      listTimer = undefined;
      stopMessagePolling();
      return;
    }
    void refreshList();
    void refreshMessages();
    if (!listTimer) listTimer = setInterval(() => void refreshList(), 30000);
    if (selected?.status_for_me === 'open' && !messageTimer) {
      messageTimer = setInterval(() => void refreshMessages(), 5000);
    }
  }
  async function invitation(id: number, action: 'accept' | 'decline') {
    actionBusy = true;
    error = '';
    try {
      const updated = await api.decideBuddyInvitation(id, action);
      if (action === 'decline') { conversations = conversations.filter((c) => c.id !== id); if (selected?.id === id) clearSelection(); }
      else { conversations = conversations.map((c) => c.id === id ? updated : c); if (selected?.id === id) selected = updated; }
      await refreshList();
      if (action === 'accept' && selected?.id === id) choose(updated);
    }
    catch (err) { error = err instanceof Error ? err.message : 'Could not update the invitation.'; }
    finally { actionBusy = false; }
  }
  async function send() {
    const conv = selected;
    if (!conv || !body.trim() || body.length > 4000) return;
    actionBusy = true;
    try {
      const sent = await api.sendBuddyMessage(conv.id, body.trim());
      body = '';
      // The reader may have switched conversations meanwhile; the next poll of `conv` picks the message up.
      if (selected?.id === conv.id) messages = mergeBuddyMessages(messages, [sent]);
    }
    catch (err) { error = err instanceof Error ? err.message : 'Could not send message.'; }
    finally { actionBusy = false; }
  }
  async function block() {
    if (!selected || !window.confirm(`Block ${buddyConversationName(selected.former_member, selected.other.handle)}?`)) return;
    error = '';
    try { await api.blockBuddyConversation(selected.id); clearSelection(); await refreshList(); }
    catch (err) { error = err instanceof Error ? err.message : 'Could not block this member.'; }
  }
  async function report() {
    if (!selected || reportNote.length > 1000) return;
    error = ''; reportSuccess = false; actionBusy = true;
    try { await api.reportBuddyConversation(selected.id, reportReason, reportNote.trim()); reportSuccess = true; reportOpen = false; reportNote = ''; }
    catch (err) { error = err instanceof ApiError ? err.message : 'Could not send report.'; }
    finally { actionBusy = false; }
  }
  onMount(() => {
    void refreshList();
    if (document.visibilityState === 'visible') listTimer = setInterval(() => void refreshList(), 30000);
    document.addEventListener('visibilitychange', onVisibility);
  });
  onDestroy(() => {
    if (listTimer) clearInterval(listTimer);
    stopMessagePolling();
    document.removeEventListener('visibilitychange', onVisibility);
  });
</script>

{#if status === 'loading'}<States state="loading" />{:else if status === 'error'}<States state="error" />{:else}
  <div class="grid gap-4 md:grid-cols-3">
    <section aria-label="Conversations" class="divide-y divide-border rounded-lg border border-border">
      {#if conversations.length === 0}<p class="p-4 text-sm text-muted-foreground">No conversations yet.</p>{/if}
      {#each conversations as c (c.id)}<button class="w-full p-3 text-left hover:bg-accent" aria-pressed={selected?.id === c.id} onclick={() => choose(c)}>
        <span class="flex justify-between gap-2 text-sm font-medium"><span>{buddyConversationName(c.former_member, c.other.handle)}</span>{#if c.unread}<span class="rounded-full bg-brand px-2 text-xs text-brand-foreground">{c.unread}</span>{/if}</span>
        <span class="text-xs text-muted-foreground">{buddyOutgoingLabel(c.status_for_me)}</span>
        {#if c.status_for_me === 'incoming' && !c.former_member}<span class="mt-2 flex gap-2"><Button size="sm" variant="primary" disabled={actionBusy} onclick={(e) => { e.stopPropagation(); void invitation(c.id, 'accept'); }}>Accept</Button><Button size="sm" variant="outline" disabled={actionBusy} onclick={(e) => { e.stopPropagation(); void invitation(c.id, 'decline'); }}>Decline</Button></span>{/if}
      </button>{/each}
    </section>
    <section class="flex min-h-80 flex-col rounded-lg border border-border p-4 md:col-span-2" aria-label="Chat">
      {#if !selected}<p class="m-auto text-sm text-muted-foreground">Choose a conversation to read.</p>
      {:else}
        <header class="flex items-center justify-between border-b border-border pb-3"><strong>{buddyConversationName(selected.former_member, selected.other.handle)}</strong>{#if !selected.former_member && (selected.status_for_me === 'open' || selected.status_for_me === 'incoming')}<div class="flex gap-2"><Button variant="outline" size="sm" onclick={() => { reportOpen = !reportOpen; reportSuccess = false; }}>Report</Button><Button variant="outline" size="sm" onclick={block}>Block</Button></div>{/if}</header>
        {#if error}<p role="alert" class="py-2 text-sm text-destructive">{error}</p>{/if}
        {#if reportSuccess}<p role="status" class="py-2 text-sm text-muted-foreground">Report sent.</p>{/if}
        {#if reportOpen && !selected.former_member}<form class="flex flex-col gap-2 rounded-md border border-border p-3" onsubmit={(e) => { e.preventDefault(); void report(); }}>
          <label class="text-sm">Reason<select bind:value={reportReason} class="mt-1 block w-full rounded-md border border-border bg-background p-2"><option value="spam">Spam</option><option value="harassment">Harassment</option><option value="scam">Scam</option><option value="other">Other</option></select></label>
          <label class="text-sm">Optional note<textarea bind:value={reportNote} maxlength="1000" rows="3" class="mt-1 block w-full rounded-md border border-border bg-background p-2"></textarea></label>
          <span class="text-right text-xs text-muted-foreground">{reportNote.length}/1000</span>
          <div class="flex justify-end gap-2"><Button variant="outline" type="button" onclick={() => reportOpen = false}>Cancel</Button><Button variant="primary" type="submit" disabled={actionBusy || reportNote.length > 1000}>Send report</Button></div>
        </form>{/if}
        <ol class="flex flex-1 flex-col gap-3 overflow-y-auto py-4">{#each messages as message (message.id)}<li class="max-w-prose rounded-lg bg-muted p-3 text-sm {message.mine ? 'self-end' : 'self-start'}">{message.body}<time class="mt-1 block text-right text-xs text-muted-foreground">{new Date(message.created_at).toLocaleString()}</time></li>{/each}</ol>
        {#if selected.status_for_me === 'incoming' && !selected.former_member}{@const pendingId = selected.id}<div class="flex gap-2 border-t border-border pt-3"><Button variant="primary" disabled={actionBusy} onclick={() => void invitation(pendingId, 'accept')}>Accept</Button><Button variant="outline" disabled={actionBusy} onclick={() => void invitation(pendingId, 'decline')}>Decline</Button></div>
        {:else if selected.status_for_me === 'open' && !selected.former_member}<form class="flex flex-col gap-2 border-t border-border pt-3" onsubmit={(e) => { e.preventDefault(); void send(); }}><textarea bind:value={body} maxlength="4000" rows="3" class="w-full rounded-md border border-border bg-background p-3 text-sm" aria-label="Message"></textarea><div class="flex items-center justify-between"><span class="text-xs text-muted-foreground">{body.length}/4000</span><Button variant="primary" type="submit" disabled={actionBusy || !body.trim()}>Send</Button></div></form>{/if}
      {/if}
    </section>
  </div>
{/if}
