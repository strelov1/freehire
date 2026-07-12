<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '$lib/api';
  import type {
    GmailStatus,
    MailboxStatus,
    InboxSource,
    InboxGroup,
    InboxMessage,
    EmailBody,
  } from '$lib/api';
  import { Badge, Button } from '$lib/ui';
  import { Mail, AtSign, Copy } from '@lucide/svelte';
  import { timeAgo } from '$lib/utils';

  const PAGE_SIZE = 20;

  let gmail = $state<GmailStatus | null>(null);
  let mailbox = $state<MailboxStatus | null>(null);
  let groups = $state<InboxGroup[]>([]);
  let total = $state(0);
  let loading = $state(true);
  let error = $state<string | null>(null);

  // Account switcher: '' = all sources, 'gmail' | 'hosted' = one account.
  let source = $state<InboxSource>('');

  // Search: filters groups by message subject/sender/body server-side, debounced.
  let search = $state('');
  let searchTimer: ReturnType<typeof setTimeout> | undefined;

  let syncing = $state(false);
  let claiming = $state(false);

  // Expanded group's messages, cached by key; and the opened message body.
  let expanded = $state<string | null>(null);
  let threads = $state<Record<string, InboxMessage[]>>({});
  let openMsgId = $state<number | null>(null);
  let bodies = $state<Record<number, EmailBody>>({});

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
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load the inbox.';
    } finally {
      loading = false;
    }
  }

  // Load the first page of groups for the current search term + source filter.
  async function fetchFirstPage() {
    const res = await api.getInbox(search, PAGE_SIZE, 0, source);
    groups = res.groups;
    total = res.total;
  }

  // Reload the first page for the current filters; collapses any open group.
  async function reloadGroups() {
    expanded = null;
    openMsgId = null;
    try {
      await fetchFirstPage();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load the inbox.';
    }
  }

  async function loadMore() {
    try {
      const res = await api.getInbox(search, PAGE_SIZE, groups.length, source);
      groups = [...groups, ...res.groups];
      total = res.total;
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load more.';
    }
  }

  function onSearchInput() {
    clearTimeout(searchTimer);
    searchTimer = setTimeout(reloadGroups, 250);
  }

  async function setSource(s: InboxSource) {
    if (source === s) return;
    source = s;
    await reloadGroups();
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
      groups = [];
      total = 0;
      return;
    }
    await reloadGroups();
  }

  // --- Reading ---

  async function toggleGroup(key: string) {
    if (expanded === key) {
      expanded = null;
      return;
    }
    expanded = key;
    openMsgId = null;
    if (!threads[key]) {
      try {
        threads = { ...threads, [key]: await api.getInboxGroup(key) };
      } catch (e) {
        error = e instanceof Error ? e.message : 'Failed to load the thread.';
      }
    }
  }

  async function openMessage(id: number) {
    if (openMsgId === id) {
      openMsgId = null;
      return;
    }
    openMsgId = id;
    if (!bodies[id]) {
      try {
        const body = await api.getEmail(id);
        bodies = { ...bodies, [id]: body };
        // Reflect the just-opened message as read in the current thread + group counts.
        markReadLocally(id);
      } catch (e) {
        error = e instanceof Error ? e.message : 'Failed to load the message.';
      }
    }
  }

  // Optimistically flip a message to read in the cached thread and decrement its
  // group's unread count, so the list matches the server without a refetch.
  function markReadLocally(id: number) {
    for (const [key, msgs] of Object.entries(threads)) {
      const idx = msgs.findIndex((m) => m.id === id && !m.read);
      if (idx === -1) continue;
      threads[key] = msgs.map((m) => (m.id === id ? { ...m, read: true } : m));
      groups = groups.map((g) =>
        g.key === key ? { ...g, unread_count: Math.max(0, g.unread_count - 1) } : g,
      );
      break;
    }
  }
</script>

{#if loading}
  <p class="py-12 text-center text-sm text-muted-foreground">Loading…</p>
{:else if error}
  <p class="text-sm text-destructive">{error}</p>
{:else}
  <div class="flex flex-col gap-4">
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

    {#if !hasAnySource}
      <p class="py-8 text-center text-sm text-muted-foreground">
        Connect Gmail or get a freehire mailbox above to start seeing your ATS mail here.
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

      {#if groups.length === 0}
        <p class="py-12 text-center text-sm text-muted-foreground">
          {search ? 'No mail matches your search.' : 'No mail yet — it appears here as it arrives.'}
        </p>
      {:else}
        <ul class="flex flex-col gap-2">
          {#each groups as g (g.key)}
            <li class="overflow-hidden rounded-xl border border-border bg-card transition hover:border-brand">
              <button
                type="button"
                onclick={() => toggleGroup(g.key)}
                class="flex w-full items-center gap-3 p-4 text-left hover:bg-accent"
              >
                {#if g.unread_count > 0}
                  <span class="h-2 w-2 shrink-0 rounded-full bg-brand" aria-label="unread"></span>
                {/if}
                <div class="min-w-0 flex-1">
                  <div class="truncate {g.unread_count > 0 ? 'font-semibold' : 'font-medium'}">
                    {g.subject || '(no subject)'}
                  </div>
                  <div class="truncate text-xs text-muted-foreground">{g.senders.join(', ')}</div>
                </div>
                <span class="shrink-0 text-xs text-muted-foreground">{timeAgo(g.latest_received)}</span>
                <Badge variant="secondary">{g.message_count}</Badge>
              </button>

              {#if expanded === g.key}
                <div class="border-t border-border p-3">
                  {#if !threads[g.key]}
                    <p class="text-sm text-muted-foreground">Loading…</p>
                  {:else}
                    <ul class="flex flex-col gap-2">
                      {#each threads[g.key] as m (m.id)}
                        <li class="overflow-hidden rounded-lg border border-border">
                          <button
                            type="button"
                            onclick={() => openMessage(m.id)}
                            class="flex w-full flex-col gap-0.5 p-3 text-left hover:bg-accent"
                          >
                            <div class="flex items-center gap-2">
                              <span class="min-w-0 truncate text-sm {m.read ? 'font-normal' : 'font-semibold'}">{m.subject}</span>
                              <span class="ml-auto shrink-0 text-xs text-muted-foreground">{timeAgo(m.received_at)}</span>
                            </div>
                            <div class="truncate text-xs text-muted-foreground">{m.from_name || m.from_addr}</div>
                          </button>
                          {#if openMsgId === m.id}
                            <div class="border-t border-border p-3">
                              {#if !bodies[m.id]}
                                <p class="text-sm text-muted-foreground">Loading…</p>
                              {:else}
                                {@const body = bodies[m.id]}
                                {#if body?.source === 'gmail'}
                                  <div class="mb-2 flex justify-end">
                                    <a
                                      href={gmailUrl(body?.external_id ?? '')}
                                      target="_blank"
                                      rel="noopener noreferrer"
                                      class="text-xs font-medium text-primary hover:underline"
                                    >
                                      Open in Gmail ↗
                                    </a>
                                  </div>
                                {/if}
                                {#if body?.body_html}
                                  <!-- Untrusted sender HTML isolated in a sandboxed iframe (no scripts/forms/navigation). -->
                                  <iframe
                                    title="Message body"
                                    sandbox=""
                                    srcdoc={body.body_html}
                                    class="h-[24rem] w-full rounded-md border border-border bg-white"
                                  ></iframe>
                                {:else}
                                  <pre class="whitespace-pre-wrap font-sans text-sm">{body?.body_text}</pre>
                                {/if}
                              {/if}
                            </div>
                          {/if}
                        </li>
                      {/each}
                    </ul>
                  {/if}
                </div>
              {/if}
            </li>
          {/each}
        </ul>

        {#if groups.length < total}
          <div class="flex justify-center pt-1">
            <Button variant="outline" onclick={loadMore}>
              Load more ({groups.length} of {total})
            </Button>
          </div>
        {/if}
      {/if}
    {/if}
  </div>
{/if}
