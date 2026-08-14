<script lang="ts">
  // The inbox's Settings pane: the two ways to get mail in. It owns its own
  // mutations (claim, release, disconnect) and the flags that go with them, and
  // reports the outcome upward — the mail list, its filters and its pager live in
  // InboxView and stay there.
  //
  // `sync` is deliberately NOT owned here even though its button is. Syncing polls
  // the listing repeatedly while Gmail catches up, and that loop belongs beside the
  // pager it polls; this component only renders the button and its pending state.
  import { api } from '$lib/api';
  import type { GmailStatus, MailboxStatus, InboxSource } from '$lib/api';
  import { Badge, Button, ConfirmDialog } from '$lib/ui';
  import { Mail, AtSign, Copy } from '@lucide/svelte';
  import { errorMessage } from '$lib/utils';

  interface Props {
    gmail: GmailStatus | null;
    mailbox: MailboxStatus | null;
    /** True while the parent's sync poll is running. */
    syncing: boolean;
    /** Run the Gmail sync + the listing poll that follows it. */
    onSync: () => void;
    /** Open the first-time connect explainer. */
    onConnect: () => void;
    /** Send the browser to Google's consent screen (re-consent path). */
    onReconnect: () => void;
    /**
     * A source was added or removed. `removed` names the account that went away, so
     * the parent can drop a filter pointing at it before reloading.
     */
    onSourceChanged: (removed: InboxSource | null) => void;
    onError: (message: string) => void;
  }

  let {
    gmail = $bindable(),
    mailbox = $bindable(),
    syncing,
    onSync,
    onConnect,
    onReconnect,
    onSourceChanged,
    onError,
  }: Props = $props();

  let claiming = $state(false);
  let confirmDisconnectGmailOpen = $state(false);
  let confirmReleaseMailboxOpen = $state(false);

  const hasGmail = $derived(!!gmail?.connected);
  const hasMailbox = $derived(!!mailbox?.address);

  async function disconnectGmail() {
    try {
      await api.disconnectGmail();
      gmail = { connected: false, available: gmail?.available };
      onSourceChanged('gmail');
    } catch (e) {
      onError(errorMessage(e, 'Failed to disconnect.'));
    }
  }

  async function claimMailbox() {
    if (claiming) return;
    claiming = true;
    try {
      mailbox = await api.claimMailbox();
      onSourceChanged(null);
    } catch (e) {
      onError(errorMessage(e, 'Failed to create a mailbox.'));
    } finally {
      claiming = false;
    }
  }

  async function releaseMailbox() {
    try {
      mailbox = await api.releaseMailbox();
      onSourceChanged('hosted');
    } catch (e) {
      onError(errorMessage(e, 'Failed to release the mailbox.'));
    }
  }

  function copyAddress() {
    if (mailbox?.address) navigator.clipboard?.writeText(mailbox.address);
  }
</script>

<!-- Sources: the two ways to get mail in — connect Gmail and/or claim a mailbox. -->
<div class="grid gap-3 sm:grid-cols-2">
  <!-- Gmail -->
  <div class="rounded-xl border border-border bg-card p-4">
    <div class="flex items-center gap-2 text-sm font-medium">
      <Mail class="h-4 w-4 text-muted-foreground" /> Gmail
    </div>
    {#if hasGmail}
      <p class="mt-1 truncate text-xs text-muted-foreground">{gmail?.email}</p>
      {#if gmail?.status === 'needs_reconsent'}
        <Badge variant="outline" class="mt-2 border-destructive/40 text-destructive">Reconnect needed</Badge>
      {/if}
      <div class="mt-3 flex flex-wrap gap-2">
        {#if gmail?.status === 'needs_reconsent'}
          <Button variant="secondary" size="sm" onclick={onReconnect}>Reconnect</Button>
        {/if}
        <Button variant="secondary" size="sm" disabled={syncing} onclick={onSync}>
          {syncing ? 'Syncing…' : 'Sync'}
        </Button>
        <Button variant="outline" size="sm" onclick={() => (confirmDisconnectGmailOpen = true)}>
          Disconnect
        </Button>
      </div>
    {:else if gmail?.available}
      <p class="mt-1 text-xs text-muted-foreground">Pull replies from your own Gmail (needs Google sign-in).</p>
      <Button variant="primary" size="sm" class="mt-3" onclick={onConnect}>
        Connect Gmail <Mail class="h-4 w-4" />
      </Button>
    {:else}
      <p class="mt-1 text-xs text-muted-foreground">Not available yet.</p>
    {/if}
  </div>

  <!-- Hosted mailbox -->
  <div class="rounded-xl border border-border bg-card p-4">
    <div class="flex items-center gap-2 text-sm font-medium">
      <AtSign class="h-4 w-4 text-muted-foreground" /> freehire mailbox
    </div>
    {#if hasMailbox}
      <div class="mt-1 flex items-center gap-1">
        <code class="truncate rounded bg-muted px-1.5 py-0.5 text-xs">{mailbox?.address}</code>
        <button type="button" onclick={copyAddress} title="Copy address" class="shrink-0 text-muted-foreground hover:text-foreground">
          <Copy class="h-3.5 w-3.5" />
        </button>
      </div>
      <p class="mt-2 text-xs text-muted-foreground">Use this address when you apply — replies land here.</p>
      <Button variant="outline" size="sm" class="mt-3" onclick={() => (confirmReleaseMailboxOpen = true)}>
        Release
      </Button>
    {:else if mailbox?.available}
      <p class="mt-1 text-xs text-muted-foreground">Get an address on our domain — no Google needed.</p>
      <Button variant="primary" size="sm" class="mt-3" disabled={claiming} onclick={claimMailbox}>
        {claiming ? 'Creating…' : 'Get a freehire mailbox'} <AtSign class="h-4 w-4" />
      </Button>
    {:else}
      <p class="mt-1 text-xs text-muted-foreground">Not available yet.</p>
    {/if}
  </div>
</div>

<ConfirmDialog
  bind:open={confirmDisconnectGmailOpen}
  title="Disconnect Gmail?"
  description="Its synced mail is removed."
  confirmLabel="Disconnect"
  onConfirm={disconnectGmail}
/>

<ConfirmDialog
  bind:open={confirmReleaseMailboxOpen}
  title="Release your freehire mailbox?"
  description="Its received mail is deleted."
  confirmLabel="Release"
  variant="destructive"
  onConfirm={releaseMailbox}
/>
