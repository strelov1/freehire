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
  import { Mail, AtSign, Copy, Search, RefreshCw, ChevronLeft } from '@lucide/svelte';
  import { timeAgo } from '$lib/utils';
  import { avatarInitials, avatarColor } from '$lib/avatar';

  const PAGE_SIZE = 5;

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
  let refreshing = $state(false);

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

  // Toolbar refresh — re-fetch the first page, keeping the open message.
  async function refreshInbox() {
    if (refreshing) return;
    refreshing = true;
    error = null;
    try {
      await fetchFirstPage();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Refresh failed.';
    } finally {
      refreshing = false;
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

  // Mobile master-detail: clear the selection to return from the reading pane to
  // the list (on md+ both panes are always visible, so this only matters below md).
  function backToList() {
    selectedId = null;
    selected = null;
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
      <!-- Toolbar: account switcher, search, refresh. -->
      <div class="flex flex-wrap items-center gap-2">
        {#if bothConnected}
          <div class="flex gap-1 rounded-lg border border-border p-1 text-sm">
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

        <div class="relative min-w-0 flex-1">
          <Search class="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
          <input
            type="search"
            placeholder="Search subject, sender, or body…"
            bind:value={search}
            oninput={onSearchInput}
            class="w-full rounded-lg border border-border bg-background py-2 pl-9 pr-3 text-sm outline-none transition focus:border-brand focus:ring-2 focus:ring-brand-ring/40"
          />
        </div>

        <Button variant="outline" size="sm" disabled={refreshing} onclick={refreshInbox} title="Refresh">
          <RefreshCw class="h-4 w-4 {refreshing ? 'animate-spin' : ''}" />
          Refresh
        </Button>
      </div>

      {#if messages.length === 0}
        <p class="py-12 text-center text-sm text-muted-foreground">
          {search ? 'No mail matches your search.' : 'No mail yet — it appears here as it arrives.'}
        </p>
      {:else}
        <!-- Two-pane on md+; a mobile master-detail below md (open a message → the
             reading pane replaces the list, with a Back control). -->
        <div class="grid gap-5 md:grid-cols-[minmax(0,19rem)_1fr]">
          <div class="flex-col gap-2 {selectedId === null ? 'flex' : 'hidden md:flex'}">
            <ul class="flex flex-col gap-1">
              {#each messages as m, i (m.id)}
                <li class="row-in" style="animation-delay: {Math.min(i, 14) * 15}ms">
                  <button
                    type="button"
                    onclick={() => openMessage(m.id)}
                    aria-current={selectedId === m.id}
                    class="flex w-full items-start gap-3 rounded-xl border p-3 text-left transition-colors {selectedId === m.id
                      ? 'border-brand-ring bg-brand-muted/60'
                      : 'border-transparent hover:border-border hover:bg-accent'}"
                  >
                    <div
                      class="mt-0.5 flex h-9 w-9 shrink-0 select-none items-center justify-center rounded-full text-xs font-semibold text-white"
                      style="background-color: {avatarColor(m.from_addr || m.from_name)}"
                    >
                      {avatarInitials(m.from_name, m.from_addr)}
                    </div>
                    <div class="min-w-0 flex-1">
                      <div class="flex items-baseline gap-2">
                        {#if !m.read}
                          <span class="h-1.5 w-1.5 shrink-0 rounded-full bg-brand" aria-label="unread"></span>
                        {/if}
                        <span class="min-w-0 flex-1 truncate text-sm {m.read ? 'font-medium text-foreground/90' : 'font-semibold text-foreground'}">
                          {m.from_name || m.from_addr}
                        </span>
                        <span class="shrink-0 text-[11px] text-muted-foreground">{timeAgo(m.received_at)}</span>
                      </div>
                      <div class="mt-0.5 truncate text-sm {m.read ? 'text-muted-foreground' : 'text-foreground'}">
                        {m.subject || '(no subject)'}
                      </div>
                      {#if m.snippet}
                        <div class="mt-0.5 truncate text-xs text-muted-foreground/80">{m.snippet}</div>
                      {/if}
                    </div>
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

          <!-- Reading pane — borderless, flush, to give the content the room. On
               mobile it replaces the list once a message is open. -->
          <div class="min-h-[20rem] {selectedId === null ? 'hidden md:block' : 'block'}">
            <button
              type="button"
              onclick={backToList}
              class="mb-3 -ml-1 flex items-center gap-1 rounded-md px-1 py-1 text-sm text-muted-foreground hover:text-foreground md:hidden"
            >
              <ChevronLeft class="h-4 w-4" /> Inbox
            </button>
            {#if bodyLoading}
              <p class="py-16 text-center text-sm text-muted-foreground">Loading…</p>
            {:else if !selected}
              <div class="flex h-full flex-col items-center justify-center gap-2 py-16 text-center">
                <Mail class="h-7 w-7 text-muted-foreground/50" />
                <p class="text-sm text-muted-foreground">Select a message to read it.</p>
              </div>
            {:else}
              {@const s = selected}
              <div class="flex items-start gap-3">
                <div
                  class="flex h-10 w-10 shrink-0 select-none items-center justify-center rounded-full text-sm font-semibold text-white"
                  style="background-color: {avatarColor(s.from_addr || s.from_name)}"
                >
                  {avatarInitials(s.from_name, s.from_addr)}
                </div>
                <div class="min-w-0 flex-1">
                  <h2 class="text-base font-semibold leading-snug tracking-tight">{s.subject || '(no subject)'}</h2>
                  <div class="mt-1 flex flex-wrap items-center gap-x-2 gap-y-0.5 text-xs text-muted-foreground">
                    <span class="font-medium text-foreground/80">{s.from_name || s.from_addr}</span>
                    {#if s.from_name}
                      <span aria-hidden="true">·</span>
                      <span class="truncate">{s.from_addr}</span>
                    {/if}
                    <span class="ml-auto shrink-0">{timeAgo(s.received_at)}</span>
                  </div>
                </div>
              </div>

              {#if s.source === 'gmail'}
                <div class="mt-2 flex justify-end">
                  <a
                    href={gmailUrl(s.external_id)}
                    target="_blank"
                    rel="noopener noreferrer"
                    class="text-xs font-medium text-brand-strong hover:underline"
                  >
                    Open in Gmail ↗
                  </a>
                </div>
              {/if}

              <hr class="my-4 border-border" />

              {#if s.body_html}
                <!-- Untrusted sender HTML isolated in a sandboxed iframe (no scripts/forms/navigation). -->
                <iframe
                  title="Message body"
                  sandbox=""
                  srcdoc={s.body_html}
                  class="h-[30rem] w-full rounded-md border border-border bg-white"
                ></iframe>
              {:else}
                <pre class="whitespace-pre-wrap font-sans text-sm leading-relaxed">{s.body_text}</pre>
              {/if}
            {/if}
          </div>
        </div>
      {/if}
    {/if}
  </div>
{/if}

<style>
  @keyframes row-in {
    from {
      opacity: 0;
      transform: translateY(3px);
    }
    to {
      opacity: 1;
      transform: none;
    }
  }
  .row-in {
    animation: row-in 0.22s ease both;
  }
  @media (prefers-reduced-motion: reduce) {
    .row-in {
      animation: none;
    }
  }
</style>
