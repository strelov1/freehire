<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '$lib/api';
  import type {
    GmailStatus,
    MailboxStatus,
    InboxSource,
    InboxMessage,
    EmailBody,
  } from '$lib/api';
  import { Badge, Button } from '$lib/ui';
  import { Mail, AtSign, Copy } from '@lucide/svelte';
  import { timeAgo } from '$lib/utils';

  const PAGE_SIZE = 20;

  let gmail = $state<GmailStatus | null>(null);
  let mailbox = $state<MailboxStatus | null>(null);
  let messages = $state<InboxMessage[]>([]);
  let total = $state(0);
  let loading = $state(true);
  let error = $state<string | null>(null);

  // Account switcher: '' = all sources, 'gmail' | 'hosted' = one account.
  let source = $state<InboxSource>('');

  // Search: filters by subject/sender/body server-side, debounced.
  let search = $state('');
  let searchTimer: ReturnType<typeof setTimeout> | undefined;

  let syncing = $state(false);
  let claiming = $state(false);

  // Which pane: the mail list ('inbox') or the account setup ('settings').
  let tab = $state<'inbox' | 'settings'>('inbox');

  // The selected message and its loaded body (reading pane).
  let selectedId = $state<number | null>(null);
  let selected = $state<EmailBody | null>(null);
  let bodyLoading = $state(false);

  const hasGmail = $derived(!!gmail?.connected);
  const hasMailbox = $derived(!!mailbox?.address);
  const hasAnySource = $derived(hasGmail || hasMailbox);
  const bothConnected = $derived(hasGmail && hasMailbox);

  onMount(load);

  async function load() {
    loading = true;
    error = null;
    try {
      [gmail, mailbox] = await Promise.all([api.gmailStatus(), api.mailboxStatus()]);
      if (hasAnySource) await fetchFirstPage();
      else tab = 'settings'; // nothing to read yet — land on setup
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load the inbox.';
    } finally {
      loading = false;
    }
  }

  // Load the first page for the current search term + source filter.
  async function fetchFirstPage() {
    const res = await api.getInbox(search, PAGE_SIZE, 0, source);
    messages = res.messages;
    total = res.total;
  }

  // Reload the first page; clears the reading pane.
  async function reloadList() {
    selectedId = null;
    selected = null;
    try {
      await fetchFirstPage();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load the inbox.';
    }
  }

  async function loadMore() {
    try {
      const res = await api.getInbox(search, PAGE_SIZE, messages.length, source);
      messages = [...messages, ...res.messages];
      total = res.total;
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load more.';
    }
  }

  function onSearchInput() {
    clearTimeout(searchTimer);
    searchTimer = setTimeout(reloadList, 250);
  }

  async function setSource(s: InboxSource) {
    if (source === s) return;
    source = s;
    await reloadList();
  }

  async function openMessage(id: number) {
    selectedId = id;
    selected = null;
    bodyLoading = true;
    try {
      selected = await api.getEmail(id);
      // Reflect the just-opened message as read in the list without a refetch.
      messages = messages.map((m) => (m.id === id ? { ...m, read: true } : m));
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load the message.';
    } finally {
      bodyLoading = false;
    }
  }

  // --- Gmail source ---

  function connectGmail() {
    window.location.href = '/api/v1/me/gmail/connect';
  }

  async function sync() {
    if (syncing) return;
    syncing = true;
    error = null;
    try {
      await api.syncGmail();
      for (let i = 0; i < 8; i++) {
        await new Promise((r) => setTimeout(r, 2500));
        await fetchFirstPage();
      }
    } catch (e) {
      error = e instanceof Error ? e.message : 'Sync failed.';
    } finally {
      syncing = false;
    }
  }

  async function disconnectGmail() {
    if (!confirm('Disconnect Gmail and remove its synced mail?')) return;
    try {
      await api.disconnectGmail();
      gmail = { connected: false, available: gmail?.available };
      if (source === 'gmail') source = '';
      await refresh();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to disconnect.';
    }
  }

  // Deep link to a Gmail message in Gmail's web UI (the Gmail API id is the URL id).
  const gmailUrl = (externalId: string) =>
    `https://mail.google.com/mail/?authuser=${encodeURIComponent(gmail?.email ?? '')}#all/${externalId}`;

  // --- Hosted mailbox source ---

  async function claimMailbox() {
    if (claiming) return;
    claiming = true;
    error = null;
    try {
      mailbox = await api.claimMailbox();
      await refresh();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to create a mailbox.';
    } finally {
      claiming = false;
    }
  }

  async function releaseMailbox() {
    if (!confirm('Release your freehire mailbox and delete its received mail?')) return;
    try {
      mailbox = await api.releaseMailbox();
      if (source === 'hosted') source = '';
      await refresh();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to release the mailbox.';
    }
  }

  function copyAddress() {
    if (mailbox?.address) navigator.clipboard?.writeText(mailbox.address);
  }

  // Refresh the listing after a source is added/removed; empties it when none left.
  async function refresh() {
    if (!hasAnySource) {
      messages = [];
      total = 0;
      selectedId = null;
      selected = null;
      return;
    }
    await reloadList();
  }
</script>

{#if loading}
  <p class="py-12 text-center text-sm text-muted-foreground">Loading…</p>
{:else if error}
  <p class="text-sm text-destructive">{error}</p>
{:else}
  <div class="flex flex-col gap-4">
    <!-- Tabs: keep the mail list and the account setup on separate panes. -->
    <div class="flex gap-4 border-b border-border text-sm">
      {#each [{ id: 'inbox', label: 'Inbox' }, { id: 'settings', label: 'Settings' }] as t (t.id)}
        <button
          type="button"
          onclick={() => (tab = t.id as 'inbox' | 'settings')}
          class="-mb-px border-b-2 px-1 py-2 transition-colors {tab === t.id
            ? 'border-brand font-medium text-foreground'
            : 'border-transparent text-muted-foreground hover:text-foreground'}"
        >
          {t.label}
        </button>
      {/each}
    </div>

    {#if tab === 'settings'}
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
                <Button variant="secondary" size="sm" onclick={connectGmail}>Reconnect</Button>
              {/if}
              <Button variant="secondary" size="sm" disabled={syncing} onclick={sync}>
                {syncing ? 'Syncing…' : 'Sync'}
              </Button>
              <Button variant="outline" size="sm" onclick={disconnectGmail}>Disconnect</Button>
            </div>
          {:else if gmail?.available}
            <p class="mt-1 text-xs text-muted-foreground">Pull replies from your own Gmail (needs Google sign-in).</p>
            <Button variant="primary" size="sm" class="mt-3" onclick={connectGmail}>
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
            <Button variant="outline" size="sm" class="mt-3" onclick={releaseMailbox}>Release</Button>
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
    {:else if !hasAnySource}
      <p class="py-8 text-center text-sm text-muted-foreground">
        No mail source yet —
        <button type="button" class="font-medium text-primary hover:underline" onclick={() => (tab = 'settings')}>set one up in Settings</button>.
      </p>
    {:else}
      <!-- Account switcher (only when both sources feed the inbox). -->
      {#if bothConnected}
        <div class="flex gap-1 self-start rounded-lg border border-border p-1 text-sm">
          {#each [{ value: '', label: 'All' }, { value: 'gmail', label: 'Gmail' }, { value: 'hosted', label: 'Mailbox' }] as opt (opt.value)}
            <button
              type="button"
              onclick={() => setSource(opt.value as InboxSource)}
              class="rounded px-3 py-1 transition-colors {source === opt.value
                ? 'bg-secondary font-medium text-foreground'
                : 'text-muted-foreground hover:text-foreground'}"
            >
              {opt.label}
            </button>
          {/each}
        </div>
      {/if}

      <input
        type="search"
        placeholder="Search subject, sender, or body…"
        bind:value={search}
        oninput={onSearchInput}
        class="w-full rounded-md border border-border bg-background px-3 py-2 text-sm"
      />

      {#if messages.length === 0}
        <p class="py-12 text-center text-sm text-muted-foreground">
          {search ? 'No mail matches your search.' : 'No mail yet — it appears here as it arrives.'}
        </p>
      {:else}
        <!-- Two-pane mail client: the flat message list, then the reading pane. -->
        <div class="grid gap-4 md:grid-cols-[minmax(0,22rem)_1fr]">
          <div class="flex flex-col gap-2">
            <ul class="flex flex-col gap-1">
              {#each messages as m (m.id)}
                <li>
                  <button
                    type="button"
                    onclick={() => openMessage(m.id)}
                    aria-current={selectedId === m.id}
                    class="w-full rounded-lg border p-3 text-left transition-colors {selectedId === m.id
                      ? 'border-brand bg-secondary'
                      : 'border-border bg-card hover:bg-accent'}"
                  >
                    <div class="flex items-center gap-2">
                      {#if !m.read}
                        <span class="h-1.5 w-1.5 shrink-0 rounded-full bg-brand" aria-label="unread"></span>
                      {/if}
                      <span class="min-w-0 truncate text-sm {m.read ? 'font-normal' : 'font-semibold'}">
                        {m.from_name || m.from_addr}
                      </span>
                      <span class="ml-auto shrink-0 text-xs text-muted-foreground">{timeAgo(m.received_at)}</span>
                    </div>
                    <div class="mt-0.5 truncate text-sm {m.read ? 'text-muted-foreground' : 'text-foreground'}">
                      {m.subject || '(no subject)'}
                    </div>
                    {#if m.snippet}
                      <div class="mt-0.5 truncate text-xs text-muted-foreground">{m.snippet}</div>
                    {/if}
                  </button>
                </li>
              {/each}
            </ul>

            {#if messages.length < total}
              <div class="flex justify-center pt-1">
                <Button variant="outline" size="sm" onclick={loadMore}>
                  Load more ({messages.length} of {total})
                </Button>
              </div>
            {/if}
          </div>

          <!-- Reading pane. -->
          <div class="rounded-xl border border-border bg-card p-4">
            {#if bodyLoading}
              <p class="py-12 text-center text-sm text-muted-foreground">Loading…</p>
            {:else if !selected}
              <p class="py-12 text-center text-sm text-muted-foreground">Select a message to read it.</p>
            {:else}
              <div class="flex flex-col gap-1">
                <h2 class="text-lg font-semibold tracking-tight">{selected.subject || '(no subject)'}</h2>
                <div class="flex flex-wrap items-center gap-2 text-sm text-muted-foreground">
                  <span>{selected.from_name || selected.from_addr}</span>
                  {#if selected.from_name}
                    <Badge variant="outline">{selected.from_addr}</Badge>
                  {/if}
                  <span class="ml-auto text-xs">{timeAgo(selected.received_at)}</span>
                </div>
              </div>
              {#if selected.source === 'gmail'}
                <div class="mt-2 flex justify-end">
                  <a
                    href={gmailUrl(selected.external_id)}
                    target="_blank"
                    rel="noopener noreferrer"
                    class="text-xs font-medium text-primary hover:underline"
                  >
                    Open in Gmail ↗
                  </a>
                </div>
              {/if}
              <hr class="my-3 border-border" />
              {#if selected.body_html}
                <!-- Untrusted sender HTML isolated in a sandboxed iframe (no scripts/forms/navigation). -->
                <iframe
                  title="Message body"
                  sandbox=""
                  srcdoc={selected.body_html}
                  class="h-[28rem] w-full rounded-md border border-border bg-white"
                ></iframe>
              {:else}
                <pre class="whitespace-pre-wrap font-sans text-sm">{selected.body_text}</pre>
              {/if}
            {/if}
          </div>
        </div>
      {/if}
    {/if}
  </div>
{/if}
