<script lang="ts">
  // The inbox's Settings pane: the freehire mailbox, the one source it still owns
  // (claim/release). It reports the outcome upward — the mail list, its filters and
  // its pager live in InboxView and stay there.
  //
  // Gmail connect/disconnect/sync lives on the Integrations tab (/my/integrations),
  // not here — this pane only shows a short status line pointing there, so a caller
  // wondering "where did the Gmail card go" finds it in one click.
  import { resolve } from '$app/paths';
  import { api } from '$lib/api';
  import type { GmailStatus, MailboxStatus, InboxSource } from '$lib/api';
  import { Badge, Button, ConfirmDialog } from '$lib/ui';
  import { Mail, AtSign, Copy } from '@lucide/svelte';
  import { errorMessage } from '$lib/utils';
  import { messages } from './InboxSettings.messages';
  import { locale } from '$lib/i18n/currentLocale.svelte';
  import { t } from '$lib/i18n/t';

  interface Props {
    gmail: GmailStatus | null;
    mailbox: MailboxStatus | null;
    /**
     * A source was added or removed. `removed` names the account that went away, so
     * the parent can drop a filter pointing at it before reloading.
     */
    onSourceChanged: (removed: InboxSource | null) => void;
    onError: (message: string) => void;
  }

  let { gmail, mailbox = $bindable(), onSourceChanged, onError }: Props = $props();

  let claiming = $state(false);
  let confirmReleaseMailboxOpen = $state(false);

  const hasMailbox = $derived(!!mailbox?.address);
  const s = $derived(t(messages, locale()));

  async function claimMailbox() {
    if (claiming) return;
    claiming = true;
    try {
      mailbox = await api.claimMailbox();
      onSourceChanged(null);
    } catch (e) {
      onError(errorMessage(e, s.claimFailed));
    } finally {
      claiming = false;
    }
  }

  async function releaseMailbox() {
    try {
      mailbox = await api.releaseMailbox();
      onSourceChanged('hosted');
    } catch (e) {
      onError(errorMessage(e, s.releaseFailed));
    }
  }

  function copyAddress() {
    if (mailbox?.address) navigator.clipboard?.writeText(mailbox.address);
  }
</script>

<!-- Sources: the two ways to get mail in — Gmail (a status line, connected on
     Integrations) and the freehire mailbox (owned here). -->
<div class="grid gap-3 sm:grid-cols-2">
  <!-- Gmail: status only — the connect/disconnect/sync UI lives on Integrations. -->
  <div class="rounded-xl border border-border bg-card p-4">
    <div class="flex items-center gap-2 text-sm font-medium">
      <Mail class="h-4 w-4 text-muted-foreground" /> Gmail
      {#if gmail?.connected}
        <Badge variant="outline" class="border-brand-ring/40 text-brand-strong">{s.connected}</Badge>
      {/if}
    </div>
    {#if gmail?.connected}
      <p class="mt-1 truncate text-xs text-muted-foreground">{gmail?.email}</p>
    {:else if gmail?.available}
      <p class="mt-1 text-xs text-muted-foreground">{s.gmailHint}</p>
    {:else}
      <p class="mt-1 text-xs text-muted-foreground">{s.notAvailable}</p>
    {/if}
    <Button variant="secondary" size="sm" class="mt-3" href={resolve('/my/integrations')}>
      {gmail?.connected ? s.manageIntegrations : s.connectIntegrations}
    </Button>
  </div>

  <!-- Hosted mailbox -->
  <div class="rounded-xl border border-border bg-card p-4">
    <div class="flex items-center gap-2 text-sm font-medium">
      <AtSign class="h-4 w-4 text-muted-foreground" /> {s.freehireMailboxLabel}
    </div>
    {#if hasMailbox}
      <div class="mt-1 flex items-center gap-1">
        <code class="truncate rounded bg-muted px-1.5 py-0.5 text-xs">{mailbox?.address}</code>
        <button type="button" onclick={copyAddress} title={s.copyAddressTitle} class="shrink-0 text-muted-foreground hover:text-foreground">
          <Copy class="h-3.5 w-3.5" />
        </button>
      </div>
      <p class="mt-2 text-xs text-muted-foreground">{s.mailboxHint}</p>
      <Button variant="outline" size="sm" class="mt-3" onclick={() => (confirmReleaseMailboxOpen = true)}>
        {s.release}
      </Button>
    {:else if mailbox?.available}
      <p class="mt-1 text-xs text-muted-foreground">{s.getMailboxHint}</p>
      <Button variant="primary" size="sm" class="mt-3" disabled={claiming} onclick={claimMailbox}>
        {claiming ? s.creating : s.getMailbox} <AtSign class="h-4 w-4" />
      </Button>
    {:else}
      <p class="mt-1 text-xs text-muted-foreground">{s.notAvailable}</p>
    {/if}
  </div>
</div>

<ConfirmDialog
  bind:open={confirmReleaseMailboxOpen}
  title={s.releaseDialogTitle}
  description={s.releaseDialogDescription}
  confirmLabel={s.release}
  variant="destructive"
  onConfirm={releaseMailbox}
/>
